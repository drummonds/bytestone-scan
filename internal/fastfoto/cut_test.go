package fastfoto

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// The sides here are the feed path at 100 ppi: 850 px across, so that
// 3 mm is 12 px and the 1 mm left out at each edge is 3 px.
const (
	benchPPI = 100
	pathW    = 850
)

type rgb [3]uint8

var (
	pathGrey = rgb{203, 213, 222} // the FF-680W's path, as its first real feed gave it
	blueCard = rgb{167, 215, 220}
	white    = rgb{250, 250, 247}
	shadow   = rgb{181, 191, 198}
)

// path is h rows of the feed path, its grey a level or so uneven as the
// real one is.
func path(h int) *master.Image {
	img := master.NewImage(pathW, h)
	for y := range h {
		for x := range pathW {
			n := (x*7+y*13)%3 - 1
			for c := range 3 {
				img.Set(x, y, c, wide(uint8(int(pathGrey[c])+n)))
			}
		}
	}
	return img
}

func wide(v uint8) uint16 { return uint16(v)<<8 | uint16(v) }

// paint fills columns x0 to x1 of rows y0 to y1.
func paint(img *master.Image, x0, x1, y0, y1 int, v rgb) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			for c := range 3 {
				img.Set(x, y, c, wide(v[c]))
			}
		}
	}
}

// fill is the driver's pure white below the paper's end, from row y0.
func fill(img *master.Image, y0 int) { paint(img, 0, img.W, y0, img.H, rgb{255, 255, 255}) }

// #177: each side is cut across to the sheet and 3 mm of path either
// side, and the driver's white fill is left off.
func TestASideIsCutToTheSheet(t *testing.T) {
	for _, c := range []struct {
		name   string
		side   func() *master.Image
		x0, x1 int // the columns kept
		rows   int
	}{
		{"a coloured sheet mid-path, fill below", func() *master.Image {
			img := path(120)
			paint(img, 300, 600, 0, 70, blueCard)
			fill(img, 80)
			return img
		}, 288, 612, 80},
		{"a white sheet", func() *master.Image {
			img := path(120)
			paint(img, 300, 600, 0, 70, white)
			fill(img, 80)
			return img
		}, 288, 612, 80},
		{"a shadow along the sheet's edges", func() *master.Image {
			img := path(120)
			paint(img, 296, 604, 0, 70, shadow)
			paint(img, 300, 600, 0, 70, blueCard)
			fill(img, 80)
			return img
		}, 284, 616, 80},
		{"a sheet nearer than 3 mm to the path's edge", func() *master.Image {
			img := path(120)
			paint(img, 5, 400, 0, 70, blueCard)
			fill(img, 80)
			return img
		}, 0, 412, 80},
		{"a sheet the full width of the path", func() *master.Image {
			img := path(120)
			paint(img, 0, pathW, 0, 70, blueCard)
			fill(img, 80)
			return img
		}, 0, pathW, 80},
		{"a sheet fed askew", func() *master.Image {
			img := path(120)
			for y := range 70 {
				paint(img, 300+y/2, 600+y/2, y, y+1, blueCard)
			}
			fill(img, 80)
			return img
		}, 288, 612 + 34, 80},
		{"a sheet as long as the path", func() *master.Image {
			img := path(120)
			paint(img, 300, 600, 0, 120, blueCard)
			return img
		}, 288, 612, 120},
		{"a blank side the path's own grey, its edges in shadow", func() *master.Image {
			img := path(120)
			paint(img, 296, 300, 0, 70, shadow)
			paint(img, 600, 604, 0, 70, shadow)
			fill(img, 80)
			return img
		}, 284, 616, 80},
		{"a streak down the path outside the sheet", func() *master.Image {
			img := path(120)
			paint(img, 100, 102, 0, 80, rgb{120, 120, 120})
			paint(img, 300, 600, 0, 70, blueCard)
			fill(img, 80)
			return img
		}, 88, 612, 80},
		{"white columns within 1 mm of the path's edge", func() *master.Image {
			img := path(120)
			paint(img, 0, 3, 0, 80, rgb{255, 255, 252})
			paint(img, pathW-3, pathW, 0, 80, rgb{255, 255, 252})
			paint(img, 300, 600, 0, 70, blueCard)
			fill(img, 80)
			return img
		}, 288, 612, 80},
		{"nothing but path", func() *master.Image {
			img := path(120)
			fill(img, 80)
			return img
		}, 0, pathW, 80},
	} {
		whole := c.side()
		got := cut(whole, benchPPI)
		if got.W != c.x1-c.x0 || got.H != c.rows {
			t.Errorf("%s: cut to %d × %d, want %d × %d (columns %d to %d)", c.name, got.W, got.H, c.x1-c.x0, c.rows, c.x0, c.x1)
			continue
		}
		// the pixels kept are the side's own, from column x0
		for _, at := range [][2]int{{0, 0}, {got.W - 1, got.H - 1}, {got.W / 2, got.H / 2}} {
			for ch := range 3 {
				if g, w := got.At(at[0], at[1], ch), whole.At(c.x0+at[0], at[1], ch); g != w {
					t.Errorf("%s: sample %v channel %d is %#x, want the side's %#x", c.name, at, ch, g, w)
				}
			}
		}
	}
}

// The front and the back of a sheet are each cut on their own.
func TestTheSidesOfASheetAreCutEachOnItsOwn(t *testing.T) {
	front, back := path(120), path(120)
	paint(front, 300, 600, 0, 70, blueCard)
	paint(back, 250, 560, 0, 70, blueCard)
	if f, b := cut(front, benchPPI), cut(back, benchPPI); f.W != 324 || b.W != 334 {
		t.Errorf("front %d and back %d across, want 324 and 334", f.W, b.W)
	}
}

// pnm is a side as scanimage writes it, with header between the magic
// and the pixels.
func pnm(t *testing.T, header string, img *master.Image, pixels int) string {
	t.Helper()
	var b bytes.Buffer
	b.WriteString(header)
	for _, v := range img.Pix[:pixels*3] {
		b.WriteByte(byte(v >> 8))
	}
	name := filepath.Join(t.TempDir(), "side.pnm")
	if err := os.WriteFile(name, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return name
}

// #177: scanimage writes a comment line in the header, and the sides a
// feed hands over are cut.
func TestAFeedHandsOverEachSideReadAndCut(t *testing.T) {
	whole := path(120)
	paint(whole, 300, 600, 0, 70, blueCard)
	fill(whole, 80)
	for _, c := range []struct{ name, header, refused string }{
		{"as the FF-680W through scanimage", "P6\n# SANE data follows\n%d %d\n255\n", ""},
		{"no comment", "P6\n%d %d\n255\n", ""},
		{"a comment after the width and height", "P6\n%d %d\n# a comment\n255\n", ""},
		{"a comment, then a maxval other than 255", "P6\n# SANE data follows\n%d %d\n65535\n", "8-bit colour scan"},
	} {
		f, _ := fake(t)
		t.Setenv("FAKE_SIDES", "2")
		t.Setenv("FAKE_SIDE", pnm(t, fmt.Sprintf(c.header, whole.W, whole.H), whole, whole.W*whole.H))
		sides, err := feed(t, f, benchPPI)
		if c.refused != "" {
			if err == nil || !strings.Contains(err.Error(), c.refused) {
				t.Errorf("%s: error %v, want it refused as not an %s", c.name, err, c.refused)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(sides) != 2 {
			t.Errorf("%s: %d sides, want 2", c.name, len(sides))
			continue
		}
		for i, s := range sides {
			if s.W != 324 || s.H != 80 {
				t.Errorf("%s: side %d is %d × %d, want it cut to 324 × 80", c.name, i+1, s.W, s.H)
			} else if s.At(12, 0, 0) != wide(blueCard[0]) {
				t.Errorf("%s: side %d does not begin 3 mm before the sheet", c.name, i+1)
			}
		}
	}
}

func TestASideShortOfItsPixelsIsRefusedNamingTheRow(t *testing.T) {
	whole := path(120)
	f, _ := fake(t)
	t.Setenv("FAKE_SIDES", "1")
	t.Setenv("FAKE_SIDE", pnm(t, fmt.Sprintf("P6\n# SANE data follows\n%d %d\n255\n", whole.W, whole.H), whole, whole.W*50))
	if _, err := feed(t, f, benchPPI); err == nil || !strings.Contains(err.Error(), "row 51 of 120") {
		t.Errorf("error %v, want one naming row 51 of 120", err)
	}
}

func TestTheSettingsRecordedNameTheCutAndTheTransfer(t *testing.T) {
	s := strings.Join((&Feeder{}).Settings(), "; ")
	for _, want := range []string{"cut to the sheet and 3 mm", "JPEG quality 90"} {
		if !strings.Contains(s, want) {
			t.Errorf("settings %q lack %q", s, want)
		}
	}
}
