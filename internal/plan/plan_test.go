package plan

import (
	"image"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

const in = 25.4

func print(wIn, hIn float64) find.Rect { return find.Rect{X0: 0, Y0: 0, X1: wIn * in, Y1: hIn * in} }

// Story 001, "Master ppi and averaging".
func TestMasterPPIAndAveraging(t *testing.T) {
	cases := []struct {
		print     find.Rect
		scanPPI   int
		masterPPI int
		factor    int
		warn      bool
	}{
		{print(7, 5), 600, 600, 1, false},
		{print(5, 7), 1200, 600, 2, false},
		{print(6, 4), 2400, 600, 4, false},
		{find.Rect{X1: 152, Y1: 99.5}, 2400, 600, 4, false}, // a 6 × 4 cut a little small
		{find.Rect{X1: 152, Y1: 98}, 2400, 1200, 2, false},  // more than 3 mm short
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
				(tc.print.X1-tc.print.X0)/in, (tc.print.Y1-tc.print.Y0)/in, tc.scanPPI, m.PPI, m.Factor, m.Warning, tc.masterPPI, tc.factor, tc.warn)
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
		{Print: find.Rect{X0: 13, Y0: 23, X1: 113, Y1: 93}, Region: find.Rect{X0: 10, Y0: 20, X1: 116, Y1: 96}},     // ≥ 4 × 6? no: 100 × 70 mm
		{Print: find.Rect{X0: 13, Y0: 103, X1: 190, Y1: 230}, Region: find.Rect{X0: 10, Y0: 100, X1: 193, Y1: 233}}, // 177 × 127 mm, 7 × 5
	}
	p, err := New(prints, 2400)
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

func TestPassNeedsPrints(t *testing.T) {
	if _, err := New(nil, 1200); err == nil {
		t.Error("a pass with no prints was planned")
	}
}

func near(a, b image.Rectangle) bool {
	d := func(x, y int) bool { return x-y <= 1 && y-x <= 1 }
	return d(a.Min.X, b.Min.X) && d(a.Min.Y, b.Min.Y) && d(a.Max.X, b.Max.X) && d(a.Max.Y, b.Max.Y)
}
