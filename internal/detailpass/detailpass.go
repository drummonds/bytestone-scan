// Package detailpass turns the detail pass, one high-resolution scan
// covering every print on the bed, into one master image per print.
// Rows are averaged down as they stream from the scanner, so the scan's
// full resolution is never held in memory.
package detailpass

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// Region is where one print lies in the scan, in scan pixels, and the
// factor its master is averaged down by: 1, 2 or 4.
type Region struct {
	Rect   image.Rectangle
	Factor int
}

// Read reads a 16-bit colour PNM scan from r and makes one image per
// region: each factor × factor block of the region's pixels, from its top
// left corner, averaged to one and rounded to the nearest count. Blocks
// cut short by the region's right or bottom edge are dropped, and a region
// running past the scan's edge is cut at it.
func Read(r io.Reader, regions []Region) ([]*master.Image, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	w, h, err := ReadHeader(br)
	if err != nil {
		return nil, err
	}
	scan := image.Rect(0, 0, w, h)
	type state struct {
		Region
		img  *master.Image
		sums []uint32
	}
	states := make([]*state, len(regions))
	out := make([]*master.Image, len(regions))
	for i, rg := range regions {
		f := rg.Factor
		if f != 1 && f != 2 && f != 4 {
			return nil, fmt.Errorf("detail pass: region %d averaging factor %d, want 1, 2 or 4", i+1, f)
		}
		if rg.Rect = rg.Rect.Intersect(scan); rg.Rect.Empty() {
			return nil, fmt.Errorf("detail pass: region %d %v is outside the scan %v", i+1, regions[i].Rect, scan)
		}
		mw, mh := rg.Rect.Dx()/f, rg.Rect.Dy()/f
		if mw == 0 || mh == 0 {
			return nil, fmt.Errorf("detail pass: region %d %v is smaller than one %d × %d block", i+1, rg.Rect, f, f)
		}
		out[i] = master.NewImage(mw, mh)
		states[i] = &state{rg, out[i], make([]uint32, mw*3)}
	}

	row := make([]byte, w*6)
	for y := 0; y < h; y++ {
		if _, err := io.ReadFull(br, row); err != nil {
			return nil, fmt.Errorf("detail pass: scan short at row %d of %d: %w", y+1, h, err)
		}
		for _, st := range states {
			f, rect := st.Factor, st.Rect
			dy := y - rect.Min.Y
			if dy < 0 || dy >= st.img.H*f {
				continue
			}
			for i := range st.sums {
				mx, c := i/3, i%3
				for dx := 0; dx < f; dx++ {
					st.sums[i] += uint32(binary.BigEndian.Uint16(row[((rect.Min.X+mx*f+dx)*3+c)*2:]))
				}
			}
			if dy%f == f-1 {
				n := uint32(f * f)
				dst := st.img.Pix[dy/f*st.img.W*3:]
				for i, s := range st.sums {
					dst[i] = uint16((s + n/2) / n)
				}
				clear(st.sums)
			}
		}
	}
	return out, nil
}

// readHeader reads a P6 header with maxval 65535.
// ReadHeader reads a 16-bit colour PNM header, leaving br at the first
// sample.
func ReadHeader(br *bufio.Reader) (w, h int, err error) {
	var fields [4]string
	for i := range fields {
		tok, err := token(br)
		if err != nil {
			return 0, 0, fmt.Errorf("detail pass: reading PNM header: %w", err)
		}
		fields[i] = tok
	}
	if fields[0] != "P6" {
		return 0, 0, fmt.Errorf("detail pass: PNM type %s, want P6 (colour)", fields[0])
	}
	var maxval int
	if _, err := fmt.Sscan(fields[1]+" "+fields[2]+" "+fields[3], &w, &h, &maxval); err != nil {
		return 0, 0, fmt.Errorf("detail pass: PNM header: %w", err)
	}
	if maxval != 65535 {
		return 0, 0, fmt.Errorf("detail pass: maxval %d, want a 16-bit scan (65535)", maxval)
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("detail pass: scan is %d × %d", w, h)
	}
	return w, h, nil
}

// token reads one whitespace-separated header token, skipping comments,
// and consumes the single whitespace byte that ends it.
func token(br *bufio.Reader) (string, error) {
	var tok []byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			if err == io.EOF {
				err = errors.New("header ends early")
			}
			return "", err
		}
		switch {
		case b == '#' && len(tok) == 0:
			if _, err := br.ReadString('\n'); err != nil {
				return "", err
			}
		case b == ' ' || b == '\t' || b == '\n' || b == '\r':
			if len(tok) > 0 {
				return string(tok), nil
			}
		default:
			tok = append(tok, b)
		}
	}
}
