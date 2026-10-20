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
	fmt.Fprintf(&b, "P6\n# scanimage\n%d %d\n65535\n", w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for c := 0; c < 3; c++ {
				binary.Write(&b, binary.BigEndian, f(x, y, c))
			}
		}
	}
	return b.Bytes()
}

func coord(x, y, c int) uint16 { return uint16(x*1000 + y*10 + c) }

func TestFactorOneCutsRegionsExactly(t *testing.T) {
	src := pnm(10, 8, coord)
	regions := []image.Rectangle{image.Rect(0, 0, 3, 2), image.Rect(5, 4, 10, 8)}
	got, err := Read(bytes.NewReader(src), 1, regions)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range regions {
		img := got[i]
		if img.W != r.Dx() || img.H != r.Dy() {
			t.Fatalf("region %d is %d × %d, want %v", i, img.W, img.H, r.Size())
		}
		for y := 0; y < img.H; y++ {
			for x := 0; x < img.W; x++ {
				for c := 0; c < 3; c++ {
					if v, want := img.At(x, y, c), coord(r.Min.X+x, r.Min.Y+y, c); v != want {
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
		got, err := Read(bytes.NewReader(src), 2, []image.Rectangle{image.Rect(0, 0, 1, 1)})
		if err != nil {
			t.Fatal(err)
		}
		if v := got[0].At(0, 0, 1); v != tc.want {
			t.Errorf("%s: %v averages to %d, want %d", tc.name, tc.block, v, tc.want)
		}
	}
}

func TestRegionsAreInMasterPixels(t *testing.T) {
	// A 4 × 4 average over a 16 × 8 pass gives a 4 × 2 master grid.
	// Region (1,1)-(3,2) is scan pixels x 4..11, y 4..7.
	src := pnm(16, 8, func(x, y, c int) uint16 { return uint16(x/4*100 + y/4*10 + c) })
	got, err := Read(bytes.NewReader(src), 4, []image.Rectangle{image.Rect(1, 1, 3, 2)})
	if err != nil {
		t.Fatal(err)
	}
	img := got[0]
	if img.W != 2 || img.H != 1 {
		t.Fatalf("size %d × %d, want 2 × 1", img.W, img.H)
	}
	if a, b := img.At(0, 0, 2), img.At(1, 0, 2); a != 112 || b != 212 {
		t.Errorf("blue samples %d, %d, want 112, 212", a, b)
	}
}

func TestPartialBlocksAtTheEdgesAreDropped(t *testing.T) {
	// 9 × 5 at factor 2 is 4 × 2 whole blocks; the last column and row are
	// dropped, so a region reaching x = 5 lies outside the pass.
	src := pnm(9, 5, coord)
	if _, err := Read(bytes.NewReader(src), 2, []image.Rectangle{image.Rect(0, 0, 4, 2)}); err != nil {
		t.Errorf("region within the whole blocks: %v", err)
	}
	_, err := Read(bytes.NewReader(src), 2, []image.Rectangle{image.Rect(0, 0, 5, 2)})
	if err == nil {
		t.Error("region beyond the whole blocks accepted")
	}
}

func TestReadRefuses(t *testing.T) {
	good := pnm(4, 4, coord)
	cases := []struct {
		name    string
		src     []byte
		factor  int
		regions []image.Rectangle
		want    string
	}{
		{"truncated scan", good[:len(good)-7], 1, []image.Rectangle{image.Rect(0, 0, 4, 4)}, "short"},
		{"8-bit scan", []byte("P6\n4 4\n255\n" + strings.Repeat("\x00", 48)), 1, []image.Rectangle{image.Rect(0, 0, 1, 1)}, "16-bit"},
		{"not a colour scan", []byte("P5\n4 4\n65535\n"), 1, []image.Rectangle{image.Rect(0, 0, 1, 1)}, "P6"},
		{"region outside the pass", good, 1, []image.Rectangle{image.Rect(2, 2, 5, 3)}, "outside"},
		{"empty region", good, 1, []image.Rectangle{image.Rect(1, 1, 1, 2)}, "empty"},
		{"factor 3", good, 3, []image.Rectangle{image.Rect(0, 0, 1, 1)}, "factor"},
	}
	for _, tc := range cases {
		_, err := Read(bytes.NewReader(tc.src), tc.factor, tc.regions)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want one mentioning %q", tc.name, err, tc.want)
		}
	}
}
