package rendition

import (
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

const ppi = 600

// synth is a master of a print lying at angle, in a 0.5 mm margin of
// clipped lid, textured so it has a histogram: a 1 mm frame of lo round
// the edge, a band of lo down the left tenth, a band of hi down the right
// tenth, a gradient between. torn cuts a triangle off its top left
// corner, tornMM along each edge.
func synth(t *testing.T, wmm, hmm, angle float64, lo, hi uint16, tornMM float64) (*master.Image, find.Box) {
	t.Helper()
	px := func(mm float64) float64 { return mm / 25.4 * ppi }
	print := find.Box{W: px(wmm), H: px(hmm), Angle: angle}
	region := find.Box{W: print.W + px(1), H: print.H + px(1), Angle: angle}
	b := region.Bounds()
	w, h := int(math.Ceil(b.X1-b.X0)), int(math.Ceil(b.Y1-b.Y0))
	print.CX, print.CY = float64(w)/2, float64(h)/2
	m := master.NewImage(w, h)
	torn := px(tornMM)
	for y := range h {
		for x := range w {
			u, v := print.In(float64(x)+0.5, float64(y)+0.5)
			val := uint16(65535)
			if u >= 0 && u < print.W && v >= 0 && v < print.H && u+v >= torn {
				val = lo + uint16(float64(hi-lo)*min(1, max(0, (u/print.W-0.1)/0.8)))
				if u < px(1) || v < px(1) || u > print.W-px(1) || v > print.H-px(1) {
					val = lo
				}
			}
			for c := range 3 {
				m.Set(x, y, c, val)
			}
		}
	}
	return m, print
}

// Story 003, "Deskew": the print's angle measured on the master to 0.1°.
func TestMeasureFindsTheSkew(t *testing.T) {
	for _, angle := range []float64{0, 0.7, 2.3, -4, 9.5} {
		m, truth := synth(t, 60, 45, angle, 8000, 56000, 0)
		got := Measure(m, truth, ppi)
		// the edge is found at whole pixels, so the print measures up to a pixel short each side
		if math.Abs(got.Print.Angle-angle) > 0.05 || math.Abs(got.Print.W-truth.W) > 3 || math.Abs(got.Print.H-truth.H) > 3 ||
			math.Abs(got.Print.CX-truth.CX) > 1.5 || math.Abs(got.Print.CY-truth.CY) > 1.5 {
			t.Errorf("at %.1f°: measured %+v, truth %+v", angle, got.Print, truth)
		}
		if len(got.Flags) != 0 {
			t.Errorf("at %.1f°: flags %v", angle, got.Flags)
		}
	}
}

func TestMeasureStartsFromTheRegionsAngle(t *testing.T) {
	m, truth := synth(t, 60, 45, 2.3, 8000, 56000, 0)
	hint := truth
	hint.Angle = 1.1 // the preview's guess, 1.2° off
	got := Measure(m, hint, ppi)
	if math.Abs(got.Print.Angle-2.3) > 0.05 {
		t.Errorf("measured %.2f°, want 2.3", got.Print.Angle)
	}
	if len(got.Flags) != 1 || !strings.Contains(got.Flags[0], "1.1°") {
		t.Errorf("flags %v, want the region's angle questioned", got.Flags)
	}
	hint.Angle = 2.0 // within 0.5°: nothing to say
	if got := Measure(m, hint, ppi); len(got.Flags) != 0 {
		t.Errorf("0.3° off: flags %v", got.Flags)
	}
}

func TestATornCornerIsFlaggedAndStillMeasured(t *testing.T) {
	m, truth := synth(t, 60, 45, 2.3, 8000, 56000, 8)
	got := Measure(m, truth, ppi)
	if math.Abs(got.Print.Angle-2.3) > 0.1 {
		t.Errorf("measured %.2f°, want 2.3 from the straight edges", got.Print.Angle)
	}
	if len(got.Flags) != 1 || !strings.Contains(got.Flags[0], "not rectangular") {
		t.Errorf("flags %v", got.Flags)
	}
}

// Story 003, "Levels": black and white points 0.1% in from the print's
// extremes, the lid excluded.
func TestLevels(t *testing.T) {
	m, print := synth(t, 60, 45, 0, 8000, 56000, 0)
	black, white := Levels(m, print, ppi)
	if math.Abs(float64(black)-8000) > 300 || math.Abs(float64(white)-56000) > 300 {
		t.Errorf("levels %d–%d, want about 8000–56000", black, white)
	}
	m, print = synth(t, 60, 45, 0, 0, 65535, 0)
	if black, white := Levels(m, print, ppi); black != 0 || white != 65535 {
		t.Errorf("full range: levels %d–%d, want unchanged", black, white)
	}
}

// Story 003, "Renditions": the display image fits within 1920 × 1080,
// never scaled up; thumbnails by their long edge.
func TestDevelopSizes(t *testing.T) {
	cases := []struct {
		wmm, hmm float64
		w, h     int
	}{
		{120, 90, 1440, 1080}, // landscape: 1080 tall
		{90, 120, 810, 1080},  // portrait: 1080 tall
		{100, 45, 1920, 864},  // panoramic: 1920 wide
		{20, 15, 466, 348},    // small: master pixels (472 × 354, less the edge the kernel would blur), not scaled up
	}
	for _, tc := range cases {
		m, print := synth(t, tc.wmm, tc.hmm, 0, 8000, 56000, 0)
		r, img := Develop(m, print, ppi)
		b := img.Display.Bounds()
		if math.Abs(float64(b.Dx()-tc.w)) > 3 || math.Abs(float64(b.Dy()-tc.h)) > 3 || r.Display.W != b.Dx() || r.Display.H != b.Dy() {
			t.Errorf("%v × %v mm: display %v, recipe %+v, want %d × %d", tc.wmm, tc.hmm, b, r.Display, tc.w, tc.h)
		}
		if long := max(img.Thumb.Bounds().Dx(), img.Thumb.Bounds().Dy()); long != ThumbEdge {
			t.Errorf("thumb long edge %d", long)
		}
		if long := max(img.Tiny.Bounds().Dx(), img.Tiny.Bounds().Dy()); long != TinyEdge {
			t.Errorf("tiny long edge %d", long)
		}
		if r.Version != Version {
			t.Errorf("recipe version %d", r.Version)
		}
	}
}

// Story 003, "Crop": no margin or lid left, and the levels applied.
func TestDevelopedPixelsAreThePrintStretched(t *testing.T) {
	m, print := synth(t, 60, 45, 2.3, 8000, 56000, 0)
	_, img := Develop(m, print, ppi)
	d := img.Display
	b := d.Bounds()
	at := func(x, y int) uint8 { r, _, _, _ := d.At(x, y).RGBA(); return uint8(r >> 8) }
	// the corners are the print's dark frame, not lid
	for _, p := range [][2]int{{2, 2}, {b.Dx() - 3, 2}, {2, b.Dy() - 3}, {b.Dx() - 3, b.Dy() - 3}} {
		if v := at(p[0], p[1]); v > 8 {
			t.Errorf("display corner %v is %d, want the print's dark frame", p, v)
		}
	}
	// the gradient runs the full range after levels: 1.5 mm in, past the frame
	in := int(math.Round(1.5 / 25.4 * ppi))
	if l, r := at(in, b.Dy()/2), at(b.Dx()-1-in, b.Dy()/2); l > 8 || r < 247 {
		t.Errorf("levels: left %d, right %d; want near 0 and 255", l, r)
	}
}

func TestOverTenDegreesIsNotRotated(t *testing.T) {
	m, print := synth(t, 60, 45, 12, 8000, 56000, 0)
	r, img := Develop(m, print, ppi)
	if r.Skew != 0 || len(r.Flags) == 0 || !strings.Contains(strings.Join(r.Flags, ";"), "over 10°") {
		t.Errorf("recipe %+v", r)
	}
	// the crop is square to the bed, within the print
	inside := r.Print.Inside()
	if b := img.Display.Bounds(); math.Abs(float64(b.Dx())/float64(b.Dy())-(inside.X1-inside.X0)/(inside.Y1-inside.Y0)) > 0.01 {
		t.Errorf("display %v for a print whose inside is %v", b, inside)
	}
}

func TestSaveWritesTheRenditions(t *testing.T) {
	m, print := synth(t, 20, 15, 0, 8000, 56000, 0)
	r, img := Develop(m, print, ppi)
	r.Master = "abc123"
	dir := filepath.Join(t.TempDir(), "renditions", "7", "1")
	if err := Save(dir, r, img); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"display.jpg", "thumb.jpg", "tiny.jpg"} {
		fh, err := os.Open(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		_, err = jpeg.Decode(fh)
		fh.Close()
		if err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "recipe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version: 1", "master: abc123", "black:", "white:", "skew:", "print:"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("recipe.yaml lacks %q:\n%s", want, raw)
		}
	}
	again, err := Load(dir)
	if err != nil || again.Master != "abc123" || again.Display != r.Display {
		t.Errorf("loaded %+v, %v", again, err)
	}
}
