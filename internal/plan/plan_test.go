package plan

import (
	"image"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

const in = 25.4

func print(wIn, hIn float64) find.Box {
	return find.BoxOf(find.Rect{X0: 0, Y0: 0, X1: wIn * in, Y1: hIn * in})
}

var bed = find.Rect{X1: 215.9, Y1: 297.2}

// Story 001, "Master ppi and averaging".
func TestMasterPPIAndAveraging(t *testing.T) {
	cases := []struct {
		print     find.Box
		scanPPI   int
		masterPPI int
		factor    int
		warn      bool
	}{
		{print(7, 5), 600, 600, 1, false},
		{print(5, 7), 1200, 600, 2, false},
		{print(6, 4), 2400, 600, 4, false},
		{find.Box{W: 152, H: 99.5}, 2400, 600, 4, false},            // a 6 × 4 cut a little small
		{find.Box{W: 152, H: 98}, 2400, 1200, 2, false},             // more than 3 mm short
		{find.Box{W: 152, H: 101.6, Angle: 9}, 2400, 600, 4, false}, // turned: its own size, not its bounds
		{print(3.5, 2.5), 600, 600, 1, true},
		{print(3.5, 2.5), 1200, 1200, 1, false},
		{print(3.5, 2.5), 2400, 1200, 2, false},
		{print(5, 4), 2400, 1200, 2, false},  // 4 × 5 is smaller than 4 × 6
		{print(10, 3), 2400, 1200, 2, false}, // a panorama strip, short side under 4
	}
	for _, tc := range cases {
		m, err := MasterFor(tc.print, tc.scanPPI)
		if err != nil {
			t.Fatalf("%v at %d: %v", tc.print, tc.scanPPI, err)
		}
		if m.PPI != tc.masterPPI || m.Factor != tc.factor || (m.Warning != "") != tc.warn {
			t.Errorf("%.1f × %.1f in at %d ppi: master %d ppi, factor %d, warning %q; want %d, %d, warning %v",
				tc.print.W/in, tc.print.H/in, tc.scanPPI, m.PPI, m.Factor, m.Warning, tc.masterPPI, tc.factor, tc.warn)
		}
	}
}

func TestMasterForRefusesOtherScanPPI(t *testing.T) {
	for _, ppi := range []int{0, 300, 1600, 4800} {
		if _, err := MasterFor(print(7, 5), ppi); err == nil {
			t.Errorf("scan ppi %d accepted", ppi)
		}
	}
}

func TestPassCoversEveryRegionOnce(t *testing.T) {
	prints := []find.Print{
		{Print: find.BoxOf(find.Rect{X0: 13, Y0: 23, X1: 113, Y1: 93}), Region: find.BoxOf(find.Rect{X0: 10, Y0: 20, X1: 116, Y1: 96})},     // ≥ 4 × 6? no: 100 × 70 mm
		{Print: find.BoxOf(find.Rect{X0: 13, Y0: 103, X1: 190, Y1: 230}), Region: find.BoxOf(find.Rect{X0: 10, Y0: 100, X1: 193, Y1: 233})}, // 177 × 127 mm, 7 × 5
	}
	p, err := New(prints, bed, 2400)
	if err != nil {
		t.Fatal(err)
	}
	if want := (find.Rect{X0: 10, Y0: 20, X1: 193, Y1: 233}); p.Area != want {
		t.Errorf("area %+v, want %+v", p.Area, want)
	}
	if p.ScanPPI != 2400 {
		t.Errorf("scan ppi %d", p.ScanPPI)
	}
	px := func(mm float64) int { return int(mm / in * 2400) }
	want := []detailpass.Region{
		{Rect: image.Rect(0, 0, px(106), px(76)), Factor: 2},       // small: master 1200
		{Rect: image.Rect(0, px(80), px(183), px(213)), Factor: 4}, // master 600
	}
	for i, w := range want {
		got := p.Regions[i]
		if got.Factor != w.Factor || !near(got.Rect, w.Rect) {
			t.Errorf("region %d %v factor %d, want %v factor %d (±1 px)", i+1, got.Rect, got.Factor, w.Rect, w.Factor)
		}
	}
	if p.Masters[0].PPI != 1200 || p.Masters[1].PPI != 600 {
		t.Errorf("master ppi %d, %d, want 1200, 600", p.Masters[0].PPI, p.Masters[1].PPI)
	}
}

// Story 004: a turned region is scanned as its bounds, within the bed.
func TestPassScansATurnedRegionsBoundsOnTheBed(t *testing.T) {
	region := find.Box{CX: 100, CY: 10, W: 120, H: 30, Angle: 5}
	p, err := New([]find.Print{{Print: region, Region: region}}, bed, 1200)
	if err != nil {
		t.Fatal(err)
	}
	b := region.Bounds()
	if want := (find.Rect{X0: b.X0, Y0: 0, X1: b.X1, Y1: b.Y1}); p.Area != want || b.Y0 >= 0 {
		t.Errorf("area %+v, want the bounds %+v cut at the bed's top", p.Area, b)
	}
}

func TestPassNeedsPrints(t *testing.T) {
	if _, err := New(nil, bed, 1200); err == nil {
		t.Error("a pass with no prints was planned")
	}
}

func near(a, b image.Rectangle) bool {
	d := func(x, y int) bool { return x-y <= 1 && y-x <= 1 }
	return d(a.Min.X, b.Min.X) && d(a.Min.Y, b.Min.Y) && d(a.Max.X, b.Max.X) && d(a.Max.Y, b.Max.Y)
}

// Story 039: the backs pass covers the fronts' regions at 300 ppi, each
// back's master at the scan's own ppi.
func TestBacksPassScansTheFrontsRegionsAt300(t *testing.T) {
	prints := []find.Print{
		{Print: find.BoxOf(find.Rect{X0: 13, Y0: 23, X1: 113, Y1: 93}), Region: find.BoxOf(find.Rect{X0: 10, Y0: 20, X1: 116, Y1: 96})},
		{Print: find.BoxOf(find.Rect{X0: 13, Y0: 103, X1: 190, Y1: 230}), Region: find.BoxOf(find.Rect{X0: 10, Y0: 100, X1: 193, Y1: 233})},
	}
	p, err := Backs(prints, bed)
	if err != nil {
		t.Fatal(err)
	}
	if want := (find.Rect{X0: 10, Y0: 20, X1: 193, Y1: 233}); p.Area != want || p.ScanPPI != BackPPI || BackPPI != 300 {
		t.Errorf("area %+v at %d ppi, want %+v at 300", p.Area, p.ScanPPI, want)
	}
	px := func(mm float64) int { return int(mm / in * 300) }
	want := []detailpass.Region{
		{Rect: image.Rect(0, 0, px(106), px(76)), Factor: 1},
		{Rect: image.Rect(0, px(80), px(183), px(213)), Factor: 1},
	}
	for i, w := range want {
		got := p.Regions[i]
		if got.Factor != 1 || !near(got.Rect, w.Rect) || p.Masters[i].PPI != 300 || p.Masters[i].Factor != 1 || p.Masters[i].Warning != "" {
			t.Errorf("region %d %v factor %d master %+v, want %v at 300 ppi unaveraged", i+1, got.Rect, got.Factor, p.Masters[i], w.Rect)
		}
	}
	if _, err := Backs(nil, bed); err == nil {
		t.Error("a backs pass over no prints")
	}
}
