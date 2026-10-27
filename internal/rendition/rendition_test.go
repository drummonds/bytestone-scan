package rendition

import (
	"image"
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
	return synthLid(t, wmm, hmm, angle, lo, hi, tornMM, 0.5, 65535, 0)
}

// synthLid is synth with the lid as a scanner may give it: marginMM of
// it round the print, at level lidLevel rather than clipped, and darkening
// by 5% over rampMM towards the print's edge, where the print lifts it.
func synthLid(t *testing.T, wmm, hmm, angle float64, lo, hi uint16, tornMM, marginMM float64, lidLevel uint16, rampMM float64) (*master.Image, find.Box) {
	t.Helper()
	px := func(mm float64) float64 { return mm / 25.4 * ppi }
	print := find.Box{W: px(wmm), H: px(hmm), Angle: angle}
	region := find.Box{W: print.W + 2*px(marginMM), H: print.H + 2*px(marginMM), Angle: angle}
	b := region.Bounds()
	w, h := int(math.Ceil(b.X1-b.X0)), int(math.Ceil(b.Y1-b.Y0))
	print.CX, print.CY = float64(w)/2, float64(h)/2
	m := master.NewImage(w, h)
	torn := px(tornMM)
	for y := range h {
		for x := range w {
			u, v := print.In(float64(x)+0.5, float64(y)+0.5)
			val := lidLevel
			if ramp := px(rampMM); ramp > 0 {
				dist := math.Hypot(max(0, -u, u-print.W), max(0, -v, v-print.H))
				if dist < ramp {
					val = lidLevel - uint16(float64(lidLevel)*0.05*(1-dist/ramp))
				}
			}
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
	for _, want := range []string{"version: 2", "master: abc123", "black:", "white:", "skew:", "print:"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("recipe.yaml lacks %q:\n%s", want, raw)
		}
	}
	again, err := Load(dir)
	if err != nil || again.Master != "abc123" || again.Display != r.Display {
		t.Errorf("loaded %+v, %v", again, err)
	}
}

// #75: the lid is not clipped on every scanner (99% white on the 4990
// with colour correction off), and where the print lifts it the lid
// darkens gradually towards the print's edge; the preview's print box
// overshoots by a millimetre or more on that side. The edge is the step
// from the lid into the print, not the first sample short of white, and
// is found from a hint a millimetre and a half too wide.
func TestMeasureFindsTheEdgeUnderARaisedLid(t *testing.T) {
	m, truth := synthLid(t, 60, 45, 0, 8000, 56000, 0, 2.5, 64880, 1.5)
	px := func(mm float64) float64 { return mm / 25.4 * ppi }
	hint := find.Box{CX: truth.CX + px(1.5)/2, CY: truth.CY + px(1.5)/2, W: truth.W + px(1.5), H: truth.H + px(1.5)}
	got := Measure(m, hint, ppi)
	if math.Abs(got.Print.W-truth.W) > 3 || math.Abs(got.Print.H-truth.H) > 3 ||
		math.Abs(got.Print.CX-truth.CX) > 1.5 || math.Abs(got.Print.CY-truth.CY) > 1.5 {
		t.Errorf("measured %+v, truth %+v", got.Print, truth)
	}
	if len(got.Flags) != 0 {
		t.Errorf("flags %v", got.Flags)
	}
}

// #75: a print cut at the bed's edge, or one the preview's box fell short
// of, runs to the master's edge with no lid outside it. A step inside the
// print is its own texture, not an edge: with no lid seen on that side,
// the print is measured to the master's edge.
func TestAPrintAtTheMastersEdgeIsMeasuredToIt(t *testing.T) {
	m, truth := synthLid(t, 60, 45, 0, 8000, 56000, 0, 0, 65535, 0)
	px := func(mm float64) float64 { return mm / 25.4 * ppi }
	// a bright border 1.5 mm wide down the left, then the dark frame: a
	// step of the print's own, 1.5 mm in
	for y := range m.H {
		for x := 0; x < int(px(1.5)); x++ {
			for c := range 3 {
				m.Set(x, y, c, 60000)
			}
		}
	}
	got := Measure(m, truth, ppi)
	if math.Abs(got.Print.W-truth.W) > 3 || math.Abs(got.Print.H-truth.H) > 3 ||
		math.Abs(got.Print.CX-truth.CX) > 1.5 || math.Abs(got.Print.CY-truth.CY) > 1.5 {
		t.Errorf("measured %+v, truth %+v", got.Print, truth)
	}
}

// #75: a print as bright as the lid along one edge gives most profiles
// no step to mark; the few that cross a crease would set the edge there.
// An edge fewer than half the profiles find is no edge: the hint's stands.
func TestAnEdgeFewProfilesFindIsNotAnEdge(t *testing.T) {
	m, truth := synthLid(t, 60, 45, 0, 8000, 56000, 0, 0.5, 65535, 0)
	px := func(mm float64) float64 { return mm / 25.4 * ppi }
	top := int(truth.CY - truth.H/2)
	for y := top; y < top+int(px(4)); y++ {
		for x := range m.W {
			val := uint16(64500) // 98.4%: lid-bright
			if y == top+int(px(2)) && x%7 == 0 {
				val = 20000 // a crease a few profiles cross
			}
			for c := range 3 {
				m.Set(x, y, c, val)
			}
		}
	}
	got := Measure(m, truth, ppi)
	if gotTop := got.Print.CY - got.Print.H/2; math.Abs(gotTop-float64(top)) > 3 {
		t.Errorf("top edge at %.1f px, want the hint's %d: %+v", gotTop, top, got.Print)
	}
}

// #75, the other way a bright edge misleads: every profile crosses a
// crease, each at a depth of its own. Marks that do not line up are no
// edge: the hint's stands.
func TestScatteredMarksAreNotAnEdge(t *testing.T) {
	m, truth := synthLid(t, 60, 45, 0, 8000, 56000, 0, 0.5, 65535, 0)
	px := func(mm float64) float64 { return mm / 25.4 * ppi }
	top := int(truth.CY - truth.H/2)
	for y := top; y < top+int(px(4)); y++ {
		for x := range m.W {
			val := uint16(64500)
			if y == top+int(px(0.3))+(x*13)%int(px(3)) {
				val = 20000
			}
			for c := range 3 {
				m.Set(x, y, c, val)
			}
		}
	}
	got := Measure(m, truth, ppi)
	if gotTop := got.Print.CY - got.Print.H/2; math.Abs(gotTop-float64(top)) > 3 {
		t.Errorf("top edge at %.1f px, want the hint's %d: %+v", gotTop, top, got.Print)
	}
}

// Story 102: a page's renditions show the whole file as sent, fit to
// the screen, levels untouched.
func TestWholeShowsThePageAsSent(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3000, 2000))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	r, imgs := Whole(img)
	if r.Version != Version || r.Black != 0 || r.White != 65535 || r.Skew != 0 || len(r.Flags) != 0 {
		t.Errorf("recipe %+v, want this version, levels untouched, square, no flags", r)
	}
	if r.Print != (find.Box{CX: 1500, CY: 1000, W: 3000, H: 2000}) {
		t.Errorf("print %+v, want the whole page", r.Print)
	}
	if r.Display != (Size{1620, 1080}) || imgs.Display.Bounds().Size() != image.Pt(1620, 1080) {
		t.Errorf("display %+v, want 1620 × 1080: the page fit to the screen", r.Display)
	}
	if imgs.Thumb.Bounds().Dx() != ThumbEdge || imgs.Tiny.Bounds().Dx() != TinyEdge {
		t.Errorf("thumb %v tiny %v", imgs.Thumb.Bounds(), imgs.Tiny.Bounds())
	}
	if c, _, _, _ := imgs.Display.At(800, 500).RGBA(); c>>8 != 200 {
		t.Errorf("display pixel = %d, want 200: not levelled", c>>8)
	}
}
