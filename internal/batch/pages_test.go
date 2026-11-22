package batch

import (
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// fedLetter feeds one side per value, arranges the sheets as choices say
// and runs the feed on to its quality check, the letters written and
// developed.
func fedLetter(t *testing.T, sides []uint8, choices ...SheetChoice) (*Scan, wf.Store, *batchtest.Feeder) {
	t.Helper()
	store := wf.NewMemStore()
	l, err := sitting(t, t.TempDir()).NewFeed("", now)
	if err != nil {
		t.Fatal(err)
	}
	f := &batchtest.Feeder{Sides: sides}
	runFeed(t, l, store, f)
	if err := l.Arrange(choices); err != nil {
		t.Fatal(err)
	}
	if rec := runFeed(t, l, store, f); rec.Status != wf.StatusWaiting {
		t.Fatalf("after arrange: %s %s, want waiting at the quality check", rec.Status, rec.Error)
	}
	return l, store, f
}

var sheet = SheetChoice{Letter: 1, Role: "sheet"}

// laidOut is letter 0's layout as its letter.yaml has it, one sheet
// after another: each side's turn and its pages left to right, "-" for
// a blank page; "blank" for a side left out.
func laidOut(t *testing.T, l *Scan) string {
	t.Helper()
	rec := readLetter(t, l.letterFileOf(0, "letter.yaml"))
	var sheets []string
	for _, s := range rec.Sheets {
		var sides []string
		for _, sd := range []SheetSide{s.Front, s.Back} {
			if sd.Blank {
				sides = append(sides, "blank")
				continue
			}
			var pages []string
			for _, p := range sd.Pages {
				if p.Blank {
					pages = append(pages, "-")
				} else {
					pages = append(pages, fmt.Sprint(p.N))
				}
			}
			sides = append(sides, fmt.Sprintf("%d[%s]", sd.Turn, strings.Join(pages, " ")))
		}
		sheets = append(sheets, strings.Join(sides, " "))
	}
	return strings.Join(sheets, " | ")
}

// reading is letter 0's pages in reading order, each as the side it is
// cut from and, on a folded side, which half.
func reading(t *testing.T, l *Scan) string {
	t.Helper()
	var parts []string
	for i, p := range readLetter(t, l.letterFileOf(0, "letter.yaml")).Pages() {
		if p.N != i+1 {
			t.Errorf("page %d of the reading is numbered %d", i+1, p.N)
		}
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(p.Side.Stem(), "sheet-")+" "+p.Place))
	}
	return strings.Join(parts, ", ")
}

// pictures are the page pictures among letter 0's renditions, each with
// its size: "01-front/page-01.jpg 24x32".
func pictures(t *testing.T, l *Scan) string {
	t.Helper()
	found, _ := filepath.Glob(filepath.Join(l.Renditions(0), "*", "page-*.jpg"))
	sort.Strings(found)
	var parts []string
	for _, path := range found {
		fh, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(fh)
		fh.Close()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		rel, _ := filepath.Rel(l.Renditions(0), path)
		parts = append(parts, fmt.Sprintf("%s %dx%d", strings.TrimPrefix(filepath.ToSlash(rel), "sheet-"), cfg.Width, cfg.Height))
	}
	return strings.Join(parts, ", ")
}

func want(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s:\n got %s\nwant %s", what, got, want)
	}
}

// Story 202: the operator does nothing: each side is one page, numbered
// in sheet order, front before back; an envelope's sides as a sheet's.
func TestALetterNotLaidOutIsOnePageASideInSheetOrder(t *testing.T) {
	l, _, _ := fedLetter(t, []uint8{40, 50, 90, 120}, sheet, SheetChoice{Letter: 1, Role: "envelope"})
	want(t, "letter.yaml", laidOut(t, l), "0[1] 0[2] | 0[3] 0[4]")
	want(t, "reading order", reading(t, l), "01-front, 01-back, 02-front, 02-back")
	want(t, "page pictures", pictures(t, l), "01-back/page-02.jpg 24x32, 01-front/page-01.jpg 24x32, 02-back/page-04.jpg 24x32, 02-front/page-03.jpg 24x32")
	// the envelope folds and is numbered as a sheet is
	if err := l.FoldSide(0, "sheet-02-back", true); err != nil {
		t.Fatal(err)
	}
	want(t, "the envelope folded", laidOut(t, l), "0[1] 0[2] | 0[3 4] 0[5 6]")
}

// Story 202: a sheet's sides are coupled in turn, since the feeder sees
// the back flipped about the way the sheet travels: a turn of one turns
// the other the opposite way by as much, unless said of this side only
// or the other side was left out.
func TestASheetsSidesTurnTogether(t *testing.T) {
	l, _, _ := fedLetter(t, []uint8{40, 50, 90, 120, 60, 255}, sheet, sheet, SheetChoice{Letter: 1, Role: "sheet", BlankBack: true})
	turn := func(side string, by int, alone bool) {
		t.Helper()
		if err := l.TurnSide(0, side, by, alone); err != nil {
			t.Fatal(err)
		}
	}
	turn("sheet-01-front", 90, false)
	want(t, "the front a quarter turn clockwise", laidOut(t, l), "90[1] 270[2] | 0[3] 0[4] | 0[5] blank")
	want(t, "both sides drawn again, turned", pictures(t, l), "01-back/page-02.jpg 32x24, 01-front/page-01.jpg 32x24, 02-back/page-04.jpg 24x32, 02-front/page-03.jpg 24x32, 03-front/page-05.jpg 24x32")
	turn("sheet-02-back", 90, false)
	turn("sheet-02-back", 90, false)
	want(t, "the back half a turn", laidOut(t, l), "90[1] 270[2] | 180[3] 180[4] | 0[5] blank")
	turn("sheet-01-back", 90, true)
	want(t, "this side only", laidOut(t, l), "90[1] 0[2] | 180[3] 180[4] | 0[5] blank")
	turn("sheet-01-front", -90, false)
	want(t, "a later turn still moves both by as much", laidOut(t, l), "0[1] 90[2] | 180[3] 180[4] | 0[5] blank")
	turn("sheet-03-front", 90, false)
	want(t, "a side whose other side was left out", laidOut(t, l), "0[1] 90[2] | 180[3] 180[4] | 90[5] blank")
}

// Story 202: a side marked folded in half is two pages, and so is its
// sheet's other side; the pages are numbered in the order they are read,
// a page may be blank, and the fold comes off again.
func TestASideFoldedInHalfIsTwoPages(t *testing.T) {
	l, _, _ := fedLetter(t, []uint8{40, 50, 90, 120}, sheet, sheet)
	if err := l.FoldSide(0, "sheet-01-front", true); err != nil {
		t.Fatal(err)
	}
	want(t, "folded: numbered again in sheet order, left before right", laidOut(t, l), "0[1 2] 0[3 4] | 0[5] 0[6]")
	want(t, "each page half its side", pictures(t, l), "01-back/page-03.jpg 12x32, 01-back/page-04.jpg 12x32, 01-front/page-01.jpg 12x32, 01-front/page-02.jpg 12x32, 02-back/page-06.jpg 24x32, 02-front/page-05.jpg 24x32")

	// one folded sheet read as a letter is: the outside carries pages 4
	// and 1, the inside 2 and 3
	one, _, _ := fedLetter(t, []uint8{40, 50}, sheet)
	if err := one.FoldSide(0, "sheet-01-back", true); err != nil {
		t.Fatal(err)
	}
	number := func(front, back []LetterPage) error {
		return one.NumberPages(0, map[string][]LetterPage{"sheet-01-front": front, "sheet-01-back": back})
	}
	if err := number([]LetterPage{{N: 4}, {N: 1}}, []LetterPage{{N: 2}, {N: 3}}); err != nil {
		t.Fatal(err)
	}
	want(t, "numbered", laidOut(t, one), "0[4 1] 0[2 3]")
	want(t, "reading order", reading(t, one), "01-front right, 01-back left, 01-back right, 01-front left")
	want(t, "pictures by number", pictures(t, one), "01-back/page-02.jpg 12x32, 01-back/page-03.jpg 12x32, 01-front/page-01.jpg 12x32, 01-front/page-04.jpg 12x32")

	// page 4 is blank: recorded so, and no picture is made of it
	if err := number([]LetterPage{{Blank: true}, {N: 1}}, []LetterPage{{N: 2}, {N: 3}}); err != nil {
		t.Fatal(err)
	}
	want(t, "page 4 blank", laidOut(t, one), "0[- 1] 0[2 3]")
	want(t, "reading order", reading(t, one), "01-front right, 01-back left, 01-back right")
	want(t, "no picture of the blank page", pictures(t, one), "01-back/page-02.jpg 12x32, 01-back/page-03.jpg 12x32, 01-front/page-01.jpg 12x32")
	raw, _ := os.ReadFile(one.letterFileOf(0, "letter.yaml"))
	if !strings.Contains(string(raw), "blank: true") {
		t.Errorf("letter.yaml does not say the page is blank:\n%s", raw)
	}

	// what is refused changes nothing
	for name, c := range map[string]struct {
		front, back []LetterPage
		say         string
	}{
		"a number twice":   {[]LetterPage{{Blank: true}, {N: 1}}, []LetterPage{{N: 2}, {N: 2}}, "2 is given twice"},
		"a gap":            {[]LetterPage{{Blank: true}, {N: 1}}, []LetterPage{{N: 2}, {N: 4}}, "3 is missing"},
		"no number":        {[]LetterPage{{}, {N: 1}}, []LetterPage{{N: 2}, {N: 3}}, "neither a number nor"},
		"every page blank": {[]LetterPage{{Blank: true}, {Blank: true}}, []LetterPage{{Blank: true}, {Blank: true}}, "a letter needs a page"},
		"too many pages":   {[]LetterPage{{N: 1}}, []LetterPage{{N: 2}, {N: 3}}, "a side of 2 pages"},
	} {
		if err := number(c.front, c.back); err == nil || !strings.Contains(err.Error(), c.say) {
			t.Errorf("%s: %v, want it refused saying %q", name, err, c.say)
		}
		want(t, name+": nothing changed", laidOut(t, one), "0[- 1] 0[2 3]")
	}
	if err := one.NumberPages(0, map[string][]LetterPage{"sheet-09-front": {{N: 1}}}); err == nil {
		t.Error("a side the letter has not was numbered")
	}

	// turned after folding: cut left and right of the side as now turned
	if err := one.TurnSide(0, "sheet-01-front", 90, false); err != nil {
		t.Fatal(err)
	}
	want(t, "turned", laidOut(t, one), "90[- 1] 270[2 3]")
	want(t, "cut as now turned", pictures(t, one), "01-back/page-02.jpg 16x24, 01-back/page-03.jpg 16x24, 01-front/page-01.jpg 16x24")

	// the fold taken off: one page a side, numbered again in sheet order
	if err := one.FoldSide(0, "sheet-01-front", false); err != nil {
		t.Fatal(err)
	}
	want(t, "unfolded", laidOut(t, one), "90[1] 270[2]")
	want(t, "unfolded pictures", pictures(t, one), "01-back/page-02.jpg 32x24, 01-front/page-01.jpg 32x24")
}

// Story 202: a side whose other side was left out as blank folds alone;
// the side left out has no pages.
func TestFoldingASideWhoseOtherSideWasLeftOut(t *testing.T) {
	l, _, _ := fedLetter(t, []uint8{40, 255}, SheetChoice{Letter: 1, Role: "sheet", BlankBack: true})
	if err := l.FoldSide(0, "sheet-01-front", true); err != nil {
		t.Fatal(err)
	}
	want(t, "folded", laidOut(t, l), "0[1 2] blank")
	if err := l.FoldSide(0, "sheet-01-back", true); err == nil {
		t.Error("a side left out was folded")
	}
}

// Story 202: accepted, the bag's letter.yaml carries each side's pages,
// the masters are as scanned and the bag holds no page picture; the
// layout is no longer the feed's to change.
func TestTheLayoutIsBaggedWithTheLetterAndThePicturesAreNot(t *testing.T) {
	l, store, f := fedLetter(t, []uint8{40, 50}, sheet)
	sum := func(path string) [32]byte {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256(raw)
	}
	asScanned := sum(l.letterFileOf(0, "sheet-01-front.tif"))
	if err := l.FoldSide(0, "sheet-01-front", true); err != nil {
		t.Fatal(err)
	}
	if err := l.TurnSide(0, "sheet-01-front", 90, false); err != nil {
		t.Fatal(err)
	}
	if err := l.NumberPages(0, map[string][]LetterPage{"sheet-01-front": {{Blank: true}, {N: 1}}, "sheet-01-back": {{N: 2}, {N: 3}}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Accept(); err != nil {
		t.Fatal(err)
	}
	if rec := runFeed(t, l, store, f); rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s %s, want completed", rec.Status, rec.Error)
	}
	data := filepath.Join(l.root, "bags", "0", "data")
	want(t, "the bag's payload", names(t, data), "letter.yaml sheet-01-back.tif sheet-01-front.tif")
	want(t, "the bag's letter.yaml", laidOut(t, l), "90[- 1] 270[2 3]")
	if sum(filepath.Join(data, "sheet-01-front.tif")) != asScanned {
		t.Error("the master changed with the layout")
	}
	if d, err := DepositOf(l.root, 0); err != nil || d == nil || d.Status != "held" {
		t.Errorf("deposit %+v, %v, want held", d, err)
	}
	if got := strings.Join(service(l.root).Renditions[0], " "); !strings.Contains(got, "sheet-01-back/page-02.jpg") || strings.Contains(got, "page-04") {
		t.Errorf("renditions deposited: %s", got)
	}
	for name, err := range map[string]error{
		"turn":   l.TurnSide(0, "sheet-01-front", 90, false),
		"fold":   l.FoldSide(0, "sheet-01-front", false),
		"number": l.NumberPages(0, map[string][]LetterPage{"sheet-01-front": {{N: 1}, {N: 4}}, "sheet-01-back": {{N: 2}, {N: 3}}}),
	} {
		if err == nil {
			t.Errorf("%s after accepting was allowed", name)
		}
	}
}

// Story 202: a letter written before this story records no pages: each
// side is one page in sheet order, and its letter.yaml is not rewritten.
func TestALetterWithNoPagesRecordedIsOnePageASide(t *testing.T) {
	l, store, f := fedLetter(t, []uint8{40, 50, 90, 255}, sheet, SheetChoice{Letter: 1, Role: "sheet", BlankBack: true})
	path := l.letterFileOf(0, "letter.yaml")
	rec := readLetter(t, path)
	for i := range rec.Sheets {
		rec.Sheets[i].Front.Pages, rec.Sheets[i].Back.Pages = nil, nil
	}
	if err := writeYAML(path, rec); err != nil {
		t.Fatal(err)
	}
	if err := l.Accept(); err != nil {
		t.Fatal(err)
	}
	if rec := runFeed(t, l, store, f); rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s %s, want completed", rec.Status, rec.Error)
	}
	raw, err := os.ReadFile(l.letterFileOf(0, "letter.yaml"))
	if err != nil || strings.Contains(string(raw), "pages") {
		t.Errorf("the bag's letter.yaml was given pages (%v):\n%s", err, raw)
	}
	want(t, "reading order", reading(t, l), "01-front, 01-back, 02-front")
}
