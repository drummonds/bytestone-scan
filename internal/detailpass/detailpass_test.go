package detailpass

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"strings"
	"testing"
)

// pnm makes a 16-bit P6 stream whose sample at (x, y, c) is f(x, y, c).
func pnm(w, h int, f func(x, y, c int) uint16) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "P6\n# SANE data follows\n%d %d\n65535\n", w, h)
	for y := range h {
		for x := range w {
			for c := range 3 {
				binary.Write(&b, binary.BigEndian, f(x, y, c))
			}
		}
	}
	return b.Bytes()
}

func coord(x, y, c int) uint16 { return uint16(x*1000 + y*10 + c) }

func TestFactorOneCutsRegionsExactly(t *testing.T) {
	src := pnm(10, 8, coord)
	regions := []Region{{image.Rect(0, 0, 3, 2), 1}, {image.Rect(5, 4, 10, 8), 1}}
	got, err := Read(bytes.NewReader(src), regions)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range regions {
		img := got[i]
		if img.W != r.Rect.Dx() || img.H != r.Rect.Dy() {
			t.Fatalf("region %d is %d × %d, want %v", i, img.W, img.H, r.Rect.Size())
		}
		for y := 0; y < img.H; y++ {
			for x := 0; x < img.W; x++ {
				for c := range 3 {
					if v, want := img.At(x, y, c), coord(r.Rect.Min.X+x, r.Rect.Min.Y+y, c); v != want {
						t.Fatalf("region %d (%d,%d,%d) = %d, want %d", i, x, y, c, v, want)
					}
				}
			}
		}
	}
}

func TestAveragingIsTheBlockMeanRoundedToNearest(t *testing.T) {
	cases := []struct {
		name  string
		block [4]uint16 // (0,0) (1,0) (0,1) (1,1)
		want  uint16
	}{
		{"exact", [4]uint16{4, 4, 4, 4}, 4},
		{"rounds down below a half", [4]uint16{1, 1, 1, 2}, 1},
		{"rounds a half up", [4]uint16{1, 1, 2, 2}, 2},
		{"rounds up above a half", [4]uint16{1, 2, 2, 2}, 2},
		{"full scale stays full scale", [4]uint16{65535, 65535, 65535, 65535}, 65535},
	}
	for _, tc := range cases {
		src := pnm(2, 2, func(x, y, c int) uint16 { return tc.block[y*2+x] })
		got, err := Read(bytes.NewReader(src), []Region{{image.Rect(0, 0, 2, 2), 2}})
		if err != nil {
			t.Fatal(err)
		}
		if v := got[0].At(0, 0, 1); v != tc.want {
			t.Errorf("%s: %v averages to %d, want %d", tc.name, tc.block, v, tc.want)
		}
	}
}

func TestBlocksStartAtTheRegionsCorner(t *testing.T) {
	// Each 4 × 4 block from (3,1) has a constant value, so a block that
	// started anywhere else would mix values.
	src := pnm(16, 10, func(x, y, c int) uint16 { return uint16((x-3+40)/4*100 + (y-1+40)/4*10 + c) })
	got, err := Read(bytes.NewReader(src), []Region{{image.Rect(3, 1, 11, 9), 4}})
	if err != nil {
		t.Fatal(err)
	}
	img := got[0]
	if img.W != 2 || img.H != 2 {
		t.Fatalf("size %d × %d, want 2 × 2", img.W, img.H)
	}
	for _, p := range []struct{ x, y int }{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		want := uint16((p.x+10)*100 + (p.y+10)*10 + 2)
		if v := img.At(p.x, p.y, 2); v != want {
			t.Errorf("block (%d,%d) blue = %d, want %d", p.x, p.y, v, want)
		}
	}
}

func TestEachRegionHasItsOwnFactor(t *testing.T) {
	// One pass at 2400 ppi: a large print averaged 4 × 4, a small one 2 × 2.
	src := pnm(16, 8, coord)
	got, err := Read(bytes.NewReader(src), []Region{{image.Rect(0, 0, 8, 8), 4}, {image.Rect(8, 0, 16, 8), 2}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].W != 2 || got[0].H != 2 || got[1].W != 4 || got[1].H != 4 {
		t.Errorf("sizes %d × %d and %d × %d, want 2 × 2 and 4 × 4", got[0].W, got[0].H, got[1].W, got[1].H)
	}
}

func TestPartialBlocksAtARegionsEdgesAreDropped(t *testing.T) {
	got, err := Read(bytes.NewReader(pnm(9, 5, coord)), []Region{{image.Rect(0, 0, 9, 5), 2}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].W != 4 || got[0].H != 2 {
		t.Errorf("size %d × %d, want 4 × 2", got[0].W, got[0].H)
	}
}

func TestRegionRunningPastTheScanIsCutAtItsEdge(t *testing.T) {
	// The scanner rounds the area asked for to its own grid, so the scan
	// can be a pixel or two short of the regions planned in millimetres.
	got, err := Read(bytes.NewReader(pnm(9, 5, coord)), []Region{{image.Rect(4, 2, 11, 7), 1}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].W != 5 || got[0].H != 3 {
		t.Errorf("size %d × %d, want 5 × 3", got[0].W, got[0].H)
	}
}

func TestReadRefuses(t *testing.T) {
	good := pnm(4, 4, coord)
	cases := []struct {
		name    string
		src     []byte
		regions []Region
		want    string
	}{
		{"truncated scan", good[:len(good)-7], []Region{{image.Rect(0, 0, 4, 4), 1}}, "short"},
		{"8-bit scan", []byte("P6\n4 4\n255\n" + strings.Repeat("\x00", 48)), []Region{{image.Rect(0, 0, 1, 1), 1}}, "16-bit"},
		{"not a colour scan", []byte("P5\n4 4\n65535\n"), []Region{{image.Rect(0, 0, 1, 1), 1}}, "P6"},
		{"region wholly outside the scan", good, []Region{{image.Rect(4, 0, 6, 2), 1}}, "outside"},
		{"region smaller than a block", good, []Region{{image.Rect(0, 0, 3, 4), 4}}, "smaller"},
		{"factor 3", good, []Region{{image.Rect(0, 0, 3, 3), 3}}, "factor"},
	}
	for _, tc := range cases {
		_, err := Read(bytes.NewReader(tc.src), tc.regions)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want one mentioning %q", tc.name, err, tc.want)
		}
	}
}
