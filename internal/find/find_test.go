package find

import (
	"image"
	"image/color"
	"math"
	"testing"
)

const ppi = 75

// bed makes a preview of an empty bed as the 4990 sees it: the white lid
// at about 252, with a little noise, and the shadow of the bed's frame darkening the
// first 3.5 mm on the left and 5 mm at the top.
func bed() *image.RGBA { return shadedBed(nil) }

// shadedBed is bed() with the lid darkened by dark(x, y) levels on every
// channel at each point, in mm from the bed's origin: the lid's shading.
func shadedBed(dark func(xmm, ymm float64) int) *image.RGBA {
	w, h := px(215.9), px(297.2)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			n := (x*7 + y*13) % 3
			shade := max(0, 50-5*x) + max(0, 30-2*y)
			if dark != nil {
				shade += dark((float64(x)+0.5)*25.4/ppi, (float64(y)+0.5)*25.4/ppi)
			}
			img.Set(x, y, color.RGBA{uint8(252 + n - min(shade, 252)), uint8(253 + n - min(shade, 253)), uint8(253 + n - min(shade, 253)), 255})
		}
	}
	return img
}

// lidShading is the bed as measured on 2026-10-24 (#65), with a little
// more shading than measured, which is what the rule guards against: the
// lid shades the bed by 10 levels from top to bottom, and a band 50 mm
// wide down the right shades again, to 24 levels darker at the bed's edge.
func lidShading(xmm, ymm float64) int {
	dark := int(math.Round(10 * ymm / 297.2))
	if from := 215.9 - 50; xmm > from {
		dark += int(math.Round(24 * (xmm - from) / 50))
	}
	return dark
}

func px(mm float64) int { return int(math.Round(mm / 25.4 * ppi)) }

// print draws a print with a cream border and dark content, rotated by deg
// about its centre. inside reports whether a point (in the print's own mm
// coordinates) is part of it, for prints that are not rectangles.
func drawPrint(img *image.RGBA, x0, y0, w, h, deg float64, inside func(u, v float64) bool) {
	cx, cy := x0+w/2, y0+h/2
	s, c := math.Sincos(deg * math.Pi / 180)
	b := img.Bounds()
	for py := 0; py < b.Dy(); py++ {
		for pxl := 0; pxl < b.Dx(); pxl++ {
			mx, my := (float64(pxl)+0.5)*25.4/ppi, (float64(py)+0.5)*25.4/ppi
			// into the print's frame
			dx, dy := mx-cx, my-cy
			u, v := dx*c+dy*s+w/2, -dx*s+dy*c+h/2
			if u < 0 || u >= w || v < 0 || v >= h {
				continue
			}
			if inside != nil && !inside(u, v) {
				continue
			}
			col := color.RGBA{222, 210, 185, 255} // cream border
			if u > 4 && u < w-4 && v > 4 && v < h-4 {
				col = color.RGBA{90, 80, 70, 255}
				if u < w/3 && v < h/3 {
					col = color.RGBA{246, 246, 244, 255} // a white sky, like the lid
				}
			}
			img.Set(pxl, py, col)
		}
	}
}

func near(a, b float64) bool { return math.Abs(a-b) <= 0.4 }

func checkRect(t *testing.T, what string, got, want Rect) {
	t.Helper()
	if !near(got.X0, want.X0) || !near(got.Y0, want.Y0) || !near(got.X1, want.X1) || !near(got.Y1, want.Y1) {
		t.Errorf("%s = %+v, want %+v (±0.4 mm)", what, got, want)
	}
}

func TestOnePrintGetsATightMargin(t *testing.T) {
	img := bed()
	drawPrint(img, 50, 60, 100, 70, 0, nil)
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints, want 1", len(got))
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{50, 60, 150, 130})
	checkRect(t, "region", got[0].Region.Bounds(), Rect{49.5, 59.5, 150.5, 130.5})
	if got[0].Review {
		t.Errorf("square, rectangular print flagged for review: skew %.2f°, fill %.3f", got[0].Skew, got[0].Fill)
	}
}

func TestPrintAgainstTheBedEdgeHasNoMarginThere(t *testing.T) {
	img := bed()
	drawPrint(img, 0, 0, 120, 80, 0, nil)
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints, want 1", len(got))
	}
	checkRect(t, "region", got[0].Region.Bounds(), Rect{0, 0, 120.5, 80.5})
}

func TestSeparatePrintsAreNumberedInRowsLeftToRight(t *testing.T) {
	img := bed()
	drawPrint(img, 110, 10, 90, 60, 0, nil) // top right, its top 3 mm above
	drawPrint(img, 10, 13, 90, 60, 0, nil)  // top left
	drawPrint(img, 10, 90, 90, 60, 0, nil)  // second row
	got := Prints(img, ppi)
	if len(got) != 3 {
		t.Fatalf("found %d prints, want 3", len(got))
	}
	for i, x0 := range []float64{10, 110, 10} {
		if !near(got[i].Print.Bounds().X0, x0) {
			t.Errorf("print %d starts at x %.1f, want %.0f", i+1, got[i].Print.Bounds().X0, x0)
		}
	}
	if !near(got[2].Print.Bounds().Y0, 90) {
		t.Errorf("print 3 starts at y %.1f, want 90", got[2].Print.Bounds().Y0)
	}
}

func TestPrintsCloserThan5mmAreOneRegion(t *testing.T) {
	img := bed()
	drawPrint(img, 10, 10, 60, 60, 0, nil)
	drawPrint(img, 73, 10, 60, 60, 0, nil) // 3 mm gap
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d regions, want 1", len(got))
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{10, 10, 133, 70})
	if !got[0].Review {
		t.Error("two prints in one region not flagged for review")
	}
}

func TestPrintsAt5mmOrMoreAreSeparate(t *testing.T) {
	img := bed()
	drawPrint(img, 10, 10, 60, 60, 0, nil)
	drawPrint(img, 76, 10, 60, 60, 0, nil) // 6 mm gap
	if got := Prints(img, ppi); len(got) != 2 {
		t.Fatalf("found %d regions, want 2", len(got))
	}
}

func TestMarksSmallerThan20mmAreIgnored(t *testing.T) {
	img := bed()
	drawPrint(img, 10, 10, 15, 15, 0, nil)   // a speck
	drawPrint(img, 10, 50, 150, 5, 0, nil)   // a hair or a strip
	drawPrint(img, 100, 100, 25, 25, 0, nil) // the smallest print
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints, want only the 25 mm one", len(got))
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{100, 100, 125, 125})
}

// Story 004: a skewed print's region is square to the print.
func TestASkewedPrintsRegionIsSquareToIt(t *testing.T) {
	for _, deg := range []float64{3, -3, 9} {
		img := bed()
		drawPrint(img, 40, 60, 120, 80, deg, nil)
		got := Prints(img, ppi)
		if len(got) != 1 {
			t.Fatalf("%v°: found %d prints, want 1", deg, len(got))
		}
		p := got[0]
		if math.Abs(p.Skew-deg) > 0.3 || p.Region.Angle != p.Skew || p.Print.Angle != p.Skew {
			t.Errorf("%v°: skew %.2f°, region at %.2f°, print at %.2f°; want all about %v°", deg, p.Skew, p.Region.Angle, p.Print.Angle, deg)
		}
		if !near(p.Print.CX, 100) || !near(p.Print.CY, 100) || !near(p.Print.W, 120) || !near(p.Print.H, 80) {
			t.Errorf("%v°: print %+v, want 120 × 80 about (100, 100)", deg, p.Print)
		}
		if math.Abs(p.Region.W-p.Print.W-1) > 1e-9 || math.Abs(p.Region.H-p.Print.H-1) > 1e-9 || p.Region.CX != p.Print.CX || p.Region.CY != p.Print.CY {
			t.Errorf("%v°: region %+v is not the print %+v with 0.5 mm round it", deg, p.Region, p.Print)
		}
		if p.Review || p.Straighten() {
			t.Errorf("%v°: a skew the region takes up is not for review", deg)
		}
	}
}

func TestUnder0Point1DegreeIsSquareToTheBed(t *testing.T) {
	img := bed()
	drawPrint(img, 40, 60, 120, 80, 0.05, nil)
	got := Prints(img, ppi)
	if len(got) != 1 || got[0].Region.Angle != 0 || got[0].Print.Angle != 0 {
		t.Fatalf("0.05° print: %+v, want one square to the bed", got)
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{40, 60, 160, 140})
}

func TestOver10DegreesIsStraightenedByHand(t *testing.T) {
	img := bed()
	drawPrint(img, 40, 60, 120, 80, 12, nil)
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints, want 1", len(got))
	}
	p := got[0]
	if math.Abs(p.Skew-12) > 0.3 || p.Region.Angle != 0 || !p.Review || !p.Straighten() {
		t.Errorf("12° print: skew %.2f°, region at %.2f°, review %v; want measured, square to the bed, to straighten", p.Skew, p.Region.Angle, p.Review)
	}
}

func TestNonRectangularPrintIsFlagged(t *testing.T) {
	img := bed()
	cornerTorn := func(u, v float64) bool { return u+v > 30 }
	drawPrint(img, 40, 60, 120, 80, 0, cornerTorn)
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints, want 1", len(got))
	}
	if !got[0].Review {
		t.Errorf("torn print not flagged: fill %.3f", got[0].Fill)
	}
}

func TestBlackCardBehindThePrints(t *testing.T) {
	img := bed()
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			img.Set(x, y, color.RGBA{20, 20, 22, 255})
		}
	}
	drawPrint(img, 0, 0, 120, 80, 0, nil)
	drawPrint(img, 30, 100, 60, 60, 0, nil)
	got := Prints(img, ppi)
	if len(got) != 2 {
		t.Fatalf("found %d prints on black card, want 2", len(got))
	}
	checkRect(t, "print 1", got[0].Print.Bounds(), Rect{0, 0, 120, 80})
	checkRect(t, "print 2", got[1].Print.Bounds(), Rect{30, 100, 90, 160})
}

func TestEmptyBedHasNoPrints(t *testing.T) {
	if got := Prints(bed(), ppi); len(got) != 0 {
		t.Errorf("found %d prints on an empty bed", len(got))
	}
}

func TestPrintsApartSeparatesWhatMerges(t *testing.T) {
	img := bed()
	drawPrint(img, 10, 10, 60, 60, 0, nil)
	drawPrint(img, 73, 10, 60, 60, 0, nil) // 3 mm gap: one region by Prints
	got := PrintsApart(img, ppi)
	if len(got) != 2 {
		t.Fatalf("found %d prints apart, want 2", len(got))
	}
	checkRect(t, "left print", got[0].Print.Bounds(), Rect{10, 10, 70, 70})
	checkRect(t, "right print", got[1].Print.Bounds(), Rect{73, 10, 133, 70})
	if got[0].Review || got[1].Review {
		t.Error("separated prints still flagged for review")
	}
}

// #65: the background is a field, not a colour. The lid's shading is
// background wherever it falls, a band of bed darker than the rest is
// still bed, and a speck under 1 mm is noise.
func TestTheLidsShadingIsBackground(t *testing.T) {
	img := shadedBed(func(_, ymm float64) int { return int(math.Round(10 * ymm / 297.2)) })
	drawPrint(img, 10, 10, 100, 70, 0, nil)
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints on a shaded bed, want 1", len(got))
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{10, 10, 110, 80})
}

func TestABandOfDarkerBedIsNotAPrint(t *testing.T) {
	img := shadedBed(lidShading)
	drawPrint(img, 10, 10, 100, 70, 0, nil)
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints with a band of darker bed, want 1: %+v", len(got), got)
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{10, 10, 110, 80})
	if got[0].Review {
		t.Errorf("the print on a shaded bed flagged for review: skew %.2f°, fill %.3f, parts %d", got[0].Skew, got[0].Fill, got[0].Parts)
	}
}

func TestSinglePixelsAreNoise(t *testing.T) {
	img := shadedBed(lidShading)
	drawPrint(img, 50, 60, 100, 70, 0, nil)
	// a dozen hot pixels and dust grains, 150 levels out, some of them
	// within the merge radius of the print
	for i, at := range [][2]float64{{48, 70}, {152, 100}, {100, 58}, {100, 132}, {20, 20}, {200, 250}, {30, 200}, {180, 30}, {120, 220}, {60, 280}, {210, 150}, {90, 160}} {
		x, y := px(at[0]), px(at[1])
		v := uint8(255 - 150 + 7*(i%3))
		if i%2 == 1 {
			v = 95 + uint8(i) // dark specks too
		}
		img.Set(x, y, color.RGBA{v, v, v, 255})
	}
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints among specks, want 1", len(got))
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{50, 60, 150, 130})
}

func TestTwoPrintsCloseTogetherOnAShadedBedAreOneRegion(t *testing.T) {
	img := shadedBed(lidShading)
	drawPrint(img, 10, 10, 60, 60, 0, nil)
	drawPrint(img, 74, 10, 60, 60, 0, nil) // 4 mm gap
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d regions, want 1", len(got))
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{10, 10, 134, 70})
	if got[0].Parts != 2 || !got[0].Review {
		t.Errorf("parts %d, review %v; want 2 parts for review", got[0].Parts, got[0].Review)
	}
}

// A print's light content is within the bed's tolerance but is not bed:
// the field is estimated from the bed outside the prints, so a wide,
// pale print does not shade the rows it lies on.
func TestAPalePrintDoesNotShadeTheBedBesideIt(t *testing.T) {
	img := shadedBed(lidShading)
	drawPrint(img, 10, 20, 180, 120, 0, nil)
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if c := img.RGBAAt(x, y); c.R == 90 || c.R == 246 { // the content, dark or sky
				img.Set(x, y, color.RGBA{225, 226, 224, 255}) // an overexposed beach
			}
		}
	}
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints, want 1: %+v", len(got), got)
	}
	checkRect(t, "print", got[0].Print.Bounds(), Rect{10, 20, 190, 140})
}
