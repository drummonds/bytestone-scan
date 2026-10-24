package quality

import (
	"image"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

func ramp(w, h int) *master.Image {
	m := master.NewImage(w, h)
	for y := range h {
		for x := range w {
			m.Set(x, y, 0, uint16(x*100))
			m.Set(x, y, 1, uint16(y*100))
			m.Set(x, y, 2, 30000)
		}
	}
	return m
}

func TestHistogramAndClipping(t *testing.T) {
	m := master.NewImage(4, 1)
	for c := range 3 {
		m.Set(0, 0, c, 0)
		m.Set(1, 0, c, 65535)
		m.Set(2, 0, c, 65535)
		m.Set(3, 0, c, 300)
	}
	m.Set(3, 0, 2, 65535) // blue clips three times in four
	h := Analyse(m, image.Rect(0, 0, 4, 1))
	if h.Bins[0][0] != 1 || h.Bins[0][1] != 1 || h.Bins[0][255] != 2 {
		t.Errorf("red bins 0, 1, 255 = %d, %d, %d, want 1, 1, 2", h.Bins[0][0], h.Bins[0][1], h.Bins[0][255])
	}
	if h.Shadows[0] != 0.25 || h.Highlights[0] != 0.5 || h.Highlights[2] != 0.75 {
		t.Errorf("clipped shadows %v highlights %v, want red 0.25 and 0.5, blue highlights 0.75", h.Shadows, h.Highlights)
	}
}

func TestAnalyseOnlyWithinThePrint(t *testing.T) {
	// the white lid in the margin clips; the print inside it does not
	m := master.NewImage(10, 10)
	for i := range m.Pix {
		m.Pix[i] = 65535
	}
	for y := 2; y < 8; y++ {
		for x := 2; x < 8; x++ {
			for c := range 3 {
				m.Set(x, y, c, 30000)
			}
		}
	}
	h := Analyse(m, image.Rect(2, 2, 8, 8))
	if h.Highlights[0] != 0 || h.Bins[0][30000>>8] != 36 {
		t.Errorf("within the print: highlights %v, bin %d; want none clipped, 36 in the print's bin", h.Highlights, h.Bins[0][30000>>8])
	}
	if h := Analyse(m, image.Rect(-5, -5, 50, 50)); h.Highlights[0] != 0.64 {
		t.Errorf("a rectangle past the master is cut to it: highlights %v, want 0.64", h.Highlights[0])
	}
}

func TestFitKeepsTheShape(t *testing.T) {
	img := Fit(ramp(400, 300), 100)
	if img.Bounds() != image.Rect(0, 0, 100, 75) {
		t.Errorf("fit is %v, want 100 × 75", img.Bounds())
	}
	if small := Fit(ramp(40, 30), 100); small.Bounds() != image.Rect(0, 0, 40, 30) {
		t.Errorf("a small master is enlarged to %v", small.Bounds())
	}
}

func TestCropIsOneToOne(t *testing.T) {
	m := ramp(300, 200)
	c := Crop(m, image.Pt(150, 100), 64)
	if c.Bounds() != image.Rect(0, 0, 64, 64) {
		t.Fatalf("crop is %v", c.Bounds())
	}
	// the crop's top left is master pixel (118, 68)
	r, g, b, _ := c.At(0, 0).RGBA()
	if r>>8 != (118*100)>>8 || g>>8 != (68*100)>>8 || b>>8 != 30000>>8 {
		t.Errorf("crop (0,0) = %d,%d,%d, want master (118,68)", r>>8, g>>8, b>>8)
	}
}

func TestCropStaysInsideTheMaster(t *testing.T) {
	m := ramp(300, 200)
	c := Crop(m, image.Pt(0, 199), 64)
	r, g, _, _ := c.At(0, 0).RGBA()
	if r>>8 != 0 || g>>8 != (136*100)>>8 {
		t.Errorf("crop at the corner starts at %d,%d, want master (0,136)", r>>8, g>>8)
	}
	if c := Crop(ramp(20, 10), image.Pt(5, 5), 64); c.Bounds() != image.Rect(0, 0, 20, 10) {
		t.Errorf("crop of a small master is %v, want the whole 20 × 10", c.Bounds())
	}
}
