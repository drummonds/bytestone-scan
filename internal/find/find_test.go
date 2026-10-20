package find

import (
	"image"
	"image/color"
	"math"
	"testing"
)

const ppi = 75

// bed makes a preview of an empty bed as the 4990 sees it: the white lid,
// with a little noise, and the shadow of the bed's frame darkening the
// first 3.5 mm on the left and 5 mm at the top.
func bed() *image.RGBA {
	w, h := px(215.9), px(297.2)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8((x*7 + y*13) % 5)
			shade := max(0, 50-5*x) + max(0, 30-2*y)
			img.Set(x, y, color.RGBA{uint8(243 + int(n) - min(shade, 243)), uint8(244 + int(n) - min(shade, 244)), uint8(242 + int(n) - min(shade, 242)), 255})
		}
	}
	return img
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

func TestOnePrintGetsA1_5mmMargin(t *testing.T) {
	img := bed()
	drawPrint(img, 50, 60, 100, 70, 0, nil)
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints, want 1", len(got))
	}
	checkRect(t, "print", got[0].Print, Rect{50, 60, 150, 130})
	checkRect(t, "region", got[0].Region, Rect{48.5, 58.5, 151.5, 131.5})
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
	checkRect(t, "region", got[0].Region, Rect{0, 0, 121.5, 81.5})
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
		if !near(got[i].Print.X0, x0) {
			t.Errorf("print %d starts at x %.1f, want %.0f", i+1, got[i].Print.X0, x0)
		}
	}
	if !near(got[2].Print.Y0, 90) {
		t.Errorf("print 3 starts at y %.1f, want 90", got[2].Print.Y0)
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
	checkRect(t, "print", got[0].Print, Rect{10, 10, 133, 70})
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
	checkRect(t, "print", got[0].Print, Rect{100, 100, 125, 125})
}

func TestSkewedPrintIsFlagged(t *testing.T) {
	img := bed()
	drawPrint(img, 40, 60, 120, 80, 3, nil)
	got := Prints(img, ppi)
	if len(got) != 1 {
		t.Fatalf("found %d prints, want 1", len(got))
	}
	if math.Abs(got[0].Skew-3) > 0.5 {
		t.Errorf("skew %.2f°, want about 3°", got[0].Skew)
	}
	if !got[0].Review {
		t.Error("skewed print not flagged for review")
	}
}

func TestSlightSkewIsNotFlagged(t *testing.T) {
	img := bed()
	drawPrint(img, 40, 60, 120, 80, 0.5, nil)
	got := Prints(img, ppi)
	if len(got) != 1 || got[0].Review {
		t.Fatalf("0.5° print: %+v, want one print not flagged", got)
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
	checkRect(t, "print 1", got[0].Print, Rect{0, 0, 120, 80})
	checkRect(t, "print 2", got[1].Print, Rect{30, 100, 90, 160})
}

func TestEmptyBedHasNoPrints(t *testing.T) {
	if got := Prints(bed(), ppi); len(got) != 0 {
		t.Errorf("found %d prints on an empty bed", len(got))
	}
}
