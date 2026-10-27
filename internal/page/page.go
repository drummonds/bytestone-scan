// Package page reads a received file, an image of prints captured by
// someone else's device, and serves it as a scanner whose bed is the
// page: its preview is the page at the preview ppi, and its detail pass
// cuts the page's own pixels at the page's own ppi.
package page

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"math"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff" // registered for image.Decode
	_ "image/jpeg"
	_ "image/png"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// Page is a received file decoded: what it holds and what it declares.
type Page struct {
	Name   string // the file's name as sent
	Format string // jpeg, png or tiff
	PPI    int    // the resolution the file declares; 0 when it says nothing
	Depth  int    // bits per sample as received: 8 or 16
	Image  image.Image
}

// Decode reads the file named name from r.
func Decode(name string, r io.Reader) (*Page, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: not a JPEG, PNG or TIFF image: %w", name, err)
	}
	p := &Page{Name: name, Format: format, Depth: 8, Image: img}
	switch img.(type) {
	case *image.RGBA64, *image.NRGBA64, *image.Gray16:
		p.Depth = 16
	}
	switch format {
	case "jpeg":
		p.PPI = jfifPPI(raw)
	case "png":
		p.PPI = physPPI(raw)
	case "tiff":
		p.PPI = tiffPPI(raw)
	default:
		return nil, fmt.Errorf("%s: a %s image, want JPEG, PNG or TIFF", name, format)
	}
	return p, nil
}

// jfifPPI is the density a JPEG's JFIF segment declares, in dots per
// inch; 0 without one, or with one in no unit.
func jfifPPI(raw []byte) int {
	for i := 2; i+4 <= len(raw) && raw[i] == 0xff; {
		marker, n := raw[i+1], int(binary.BigEndian.Uint16(raw[i+2:]))
		if marker == 0xda { // start of scan: no more headers
			break
		}
		if marker == 0xe0 && i+4+n-2 <= len(raw) && bytes.HasPrefix(raw[i+4:], []byte("JFIF\x00")) && n >= 14 {
			seg := raw[i+4:]
			x := int(binary.BigEndian.Uint16(seg[8:]))
			switch seg[7] {
			case 1:
				return x
			case 2:
				return int(math.Round(float64(x) * 2.54))
			}
			return 0
		}
		i += 2 + n
	}
	return 0
}

// physPPI is the resolution a PNG's pHYs chunk declares, in pixels per
// inch; 0 without one, or with one in no unit.
func physPPI(raw []byte) int {
	for i := 8; i+12 <= len(raw); {
		n, typ := int(binary.BigEndian.Uint32(raw[i:])), string(raw[i+4:i+8])
		if typ == "pHYs" && n == 9 && i+8+9 <= len(raw) {
			if raw[i+16] != 1 {
				return 0
			}
			return int(math.Round(float64(binary.BigEndian.Uint32(raw[i+8:])) * 0.0254))
		}
		if typ == "IDAT" || typ == "IEND" {
			break
		}
		i += 12 + n
	}
	return 0
}

// tiffPPI is the resolution a TIFF's first directory declares, in pixels
// per inch; 0 without XResolution, or with it in no unit.
func tiffPPI(raw []byte) int {
	if len(raw) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(raw[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	ifd := int(bo.Uint32(raw[4:]))
	if ifd+2 > len(raw) {
		return 0
	}
	n := int(bo.Uint16(raw[ifd:]))
	unit, res := 2, 0.0 // inches unless said otherwise
	for k := range n {
		e := ifd + 2 + k*12
		if e+12 > len(raw) {
			return 0
		}
		tag, typ := bo.Uint16(raw[e:]), bo.Uint16(raw[e+2:])
		switch {
		case tag == 282 && typ == 5: // XResolution, RATIONAL
			off := int(bo.Uint32(raw[e+8:]))
			if off+8 > len(raw) {
				return 0
			}
			num, den := bo.Uint32(raw[off:]), bo.Uint32(raw[off+4:])
			if den != 0 {
				res = float64(num) / float64(den)
			}
		case tag == 296 && typ == 3: // ResolutionUnit, SHORT
			unit = int(bo.Uint16(raw[e+8:]))
		}
	}
	switch unit {
	case 2:
		return int(math.Round(res))
	case 3:
		return int(math.Round(res * 2.54))
	}
	return 0
}

// Scanner serves one page as a scanner: the page is the bed.
type Scanner struct {
	Page       *Page
	PPI        int    // the page's resolution, declared by the file or given by the operator
	PreviewPPI int    // the resolution the preview is served at
	Device     string // what captured the page, as the operator knows it
}

// Name is the device the page was captured on.
func (s *Scanner) Name() string {
	if s.Device == "" {
		return "unknown device"
	}
	return s.Device
}

// Settings: a received page has none on record.
func (s *Scanner) Settings() []string { return nil }

// Preview is the page scaled to PreviewPPI.
func (s *Scanner) Preview(context.Context) (image.Image, error) {
	if s.PPI <= 0 || s.PreviewPPI <= 0 {
		return nil, errors.New("page: no resolution")
	}
	b := s.Page.Image.Bounds()
	w := max(1, int(math.Round(float64(b.Dx())*float64(s.PreviewPPI)/float64(s.PPI))))
	h := max(1, int(math.Round(float64(b.Dy())*float64(s.PreviewPPI)/float64(s.PPI))))
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(out, out.Bounds(), s.Page.Image, b, draw.Src, nil)
	return out, nil
}

// Detail streams area, in mm on the page, as a 16-bit colour PNM at ppi,
// which must be the page's own: no averaging is done. What lies beyond
// the page is white, as the lid is beyond the bed.
func (s *Scanner) Detail(_ context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	if ppi != s.PPI {
		return nil, fmt.Errorf("page is %d ppi and is cut at that, not %d", s.PPI, ppi)
	}
	px := func(mm float64) int { return int(math.Round(mm / 25.4 * float64(ppi))) }
	x0, y0 := px(area.X0), px(area.Y0)
	w, h := px(area.X1-area.X0), px(area.Y1-area.Y0)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("page: area %v is empty", area)
	}
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(s.stream(pw, x0, y0, w, h))
	}()
	return pr, nil
}

func (s *Scanner) stream(w io.Writer, x0, y0, outW, outH int) error {
	if _, err := fmt.Fprintf(w, "P6\n%d %d\n65535\n", outW, outH); err != nil {
		return err
	}
	img, b := s.Page.Image, s.Page.Image.Bounds()
	row := make([]byte, outW*6)
	for y := range outH {
		sy := b.Min.Y + y0 + y
		for x := range outW {
			sx := b.Min.X + x0 + x
			var r, g, bl uint32 = 65535, 65535, 65535
			if sx >= b.Min.X && sx < b.Max.X && sy >= b.Min.Y && sy < b.Max.Y {
				r, g, bl, _ = img.At(sx, sy).RGBA()
			}
			binary.BigEndian.PutUint16(row[x*6:], uint16(r))
			binary.BigEndian.PutUint16(row[x*6+2:], uint16(g))
			binary.BigEndian.PutUint16(row[x*6+4:], uint16(bl))
		}
		if _, err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}
