package page

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// sample is a 200 × 150 px white page with a grey print at (40,30)-(120,100).
func sample() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 200, 150))
	for y := range 150 {
		for x := range 200 {
			c := color.RGBA{255, 255, 255, 255}
			if x >= 40 && x < 120 && y >= 30 && y < 100 {
				c = color.RGBA{100, 100, 100, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// jfif is the JPEG with a JFIF segment declaring density in dots per inch.
func jfif(t *testing.T, img image.Image, dpi int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	app0 := []byte{0xff, 0xe0, 0, 16, 'J', 'F', 'I', 'F', 0, 1, 1, 1, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(app0[12:], uint16(dpi))
	binary.BigEndian.PutUint16(app0[14:], uint16(dpi))
	return append(append(append([]byte{}, raw[:2]...), app0...), raw[2:]...)
}

// phys is the PNG with a pHYs chunk declaring dpi.
func phys(t *testing.T, img image.Image, dpi int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	ppm := uint32(float64(dpi)/0.0254 + 0.5)
	data := make([]byte, 9)
	binary.BigEndian.PutUint32(data, ppm)
	binary.BigEndian.PutUint32(data[4:], ppm)
	data[8] = 1
	chunk := append([]byte("pHYs"), data...)
	var out []byte
	out = append(out, raw[:33]...) // signature and IHDR
	out = binary.BigEndian.AppendUint32(out, 9)
	out = append(out, chunk...)
	out = binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(chunk))
	return append(out, raw[33:]...)
}

func TestDecodeReadsWhatTheFileDeclares(t *testing.T) {
	var plain bytes.Buffer
	if err := jpeg.Encode(&plain, sample(), nil); err != nil {
		t.Fatal(err)
	}
	m := master.NewImage(200, 150)
	for i := range m.Pix {
		m.Pix[i] = 65535
	}
	var tif bytes.Buffer
	if err := master.Encode(&tif, m, master.Options{PPI: 600}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name          string
		raw           []byte
		format        string
		ppi, depth, w int
	}{
		{"copy.jpg", jfif(t, sample(), 300), "jpeg", 300, 8, 200},
		{"copy.png", phys(t, sample(), 300), "png", 300, 8, 200},
		{"scan.tif", tif.Bytes(), "tiff", 600, 16, 200},
		{"plain.jpg", plain.Bytes(), "jpeg", 0, 8, 200},
	} {
		p, err := Decode(c.name, bytes.NewReader(c.raw))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if p.Name != c.name || p.Format != c.format || p.PPI != c.ppi || p.Depth != c.depth || p.Image.Bounds().Dx() != c.w {
			t.Errorf("%s: %+v, want format %s, %d ppi, %d bits, %d px wide", c.name, p, c.format, c.ppi, c.depth, c.w)
		}
	}
	if _, err := Decode("notes.txt", strings.NewReader("not a picture")); err == nil || !strings.Contains(err.Error(), "notes.txt") {
		t.Errorf("a text file: %v, want refused naming the file", err)
	}
}

func TestThePreviewIsThePageAtThePreviewPPI(t *testing.T) {
	p, err := Decode("copy.jpg", bytes.NewReader(jfif(t, sample(), 300)))
	if err != nil {
		t.Fatal(err)
	}
	s := &Scanner{Page: p, PPI: 300, PreviewPPI: 150, Device: "office copier"}
	img, err := s.Preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got != image.Pt(100, 75) {
		t.Errorf("preview is %v, want 100 × 75 (200 × 150 at 300 ppi shown at 150)", got)
	}
	// the print, grey, lies at (20,15)-(60,50) on the preview
	if r, _, _, _ := img.At(40, 30).RGBA(); r>>8 > 110 || r>>8 < 90 {
		t.Errorf("preview at the print's centre = %d, want grey about 100", r>>8)
	}
	if r, _, _, _ := img.At(5, 5).RGBA(); r>>8 < 250 {
		t.Errorf("preview in the margin = %d, want white", r>>8)
	}
	if s.Name() != "office copier" || len(s.Settings()) != 0 {
		t.Errorf("name %q settings %v, want the device and none", s.Name(), s.Settings())
	}
}

func TestDetailCutsThePageAtItsOwnPPI(t *testing.T) {
	p, err := Decode("copy.png", bytes.NewReader(phys(t, sample(), 300)))
	if err != nil {
		t.Fatal(err)
	}
	s := &Scanner{Page: p, PPI: 300, PreviewPPI: 150}
	mm := func(px int) float64 { return float64(px) * 25.4 / 300 }
	// the print with 10 px round it, reaching past the page's top edge
	area := find.Rect{X0: mm(30), Y0: mm(-10), X1: mm(130), Y1: mm(110)}
	rc, err := s.Detail(context.Background(), area, 300)
	if err != nil {
		t.Fatal(err)
	}
	imgs, err := detailpass.Read(rc, []detailpass.Region{{Rect: image.Rect(0, 0, 100, 120), Factor: 1}})
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	m := imgs[0]
	if m.W != 100 || m.H != 120 {
		t.Fatalf("cut is %d × %d, want 100 × 120", m.W, m.H)
	}
	if v := m.At(5, 5, 0); v != 65535 {
		t.Errorf("above the page = %d, want white", v)
	}
	if v := m.At(5, 15, 0); v != 65535 {
		t.Errorf("page margin = %d, want white", v)
	}
	if v := m.At(50, 60, 0); v != 100<<8|100 {
		t.Errorf("print = %d, want grey 100 widened to 16 bits", v)
	}

	if _, err := s.Detail(context.Background(), area, 600); err == nil || !strings.Contains(err.Error(), "300") {
		t.Errorf("another ppi: %v, want refused naming the page's", err)
	}
	// the stream is a 16-bit P6 of the area's size
	rc, _ = s.Detail(context.Background(), area, 300)
	head, _ := io.ReadAll(io.LimitReader(rc, 20))
	rc.Close()
	if !strings.HasPrefix(string(head), "P6\n100 120\n65535\n") {
		t.Errorf("header %q", head)
	}
}
