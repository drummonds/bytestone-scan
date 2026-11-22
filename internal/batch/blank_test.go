package batch

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// names are the files in dir, sorted; none for a directory that is not there.
func names(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// Story 181: a side the operator marks blank at arrange is left out of
// the letter and recorded as blank; a sheet blank on both sides is left
// out whole and the sheets after it close the gap. An envelope's side is
// left out as a sheet's is.
func TestBlankSidesAreLeftOutOfALetter(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	b := sitting(t, root)
	l, err := b.NewFeed("", now)
	if err != nil {
		t.Fatal(err)
	}
	// sheet 1 written on the front, sheet 2 on the back, sheet 3 on
	// neither, and the envelope on its front
	f := &batchtest.Feeder{Sides: []uint8{40, 255, 250, 90, 251, 252, 60, 253}}
	runFeed(t, l, store, f)
	choices := []SheetChoice{
		{Letter: 1, Role: "sheet", BlankBack: true},
		{Letter: 1, Role: "sheet", BlankFront: true},
		{Letter: 1, Role: "sheet", BlankFront: true, BlankBack: true},
		{Letter: 1, Role: "envelope", BlankBack: true},
	}
	if err := l.Arrange(choices); err != nil {
		t.Fatal(err)
	}
	if offered, err := l.Sheets(); err != nil || len(offered) != 4 || offered[0] != choices[0] || offered[1] != choices[1] || offered[2] != choices[2] || offered[3] != choices[3] {
		t.Errorf("sheets offered again %+v, %v: want the marks as made", offered, err)
	}
	if fed, out, err := l.SidesLeftOut(); err != nil || fed != 8 || out != 5 {
		t.Errorf("sides fed %d, left out %d, %v: want 8 and 5", fed, out, err)
	}
	if rec := runFeed(t, l, store, f); rec.Status != wf.StatusWaiting {
		t.Fatalf("after arrange: %s %s", rec.Status, rec.Error)
	}
	letters, _ := l.Letters()
	if len(letters) != 1 || len(letters[0].Sheets) != 3 || letters[0].Sheets[2].N != 4 {
		t.Fatalf("letters %+v: want one of three sheets, the third the feed's fourth", letters)
	}
	dir := filepath.Join(l.Dir(), "masters", "0")
	if got, want := names(t, dir), "letter.yaml sheet-01-front.tif sheet-02-back.tif sheet-03-front.tif"; got != want {
		t.Errorf("the letter's files: %s\nwant: %s", got, want)
	}
	rec := readLetter(t, filepath.Join(dir, "letter.yaml"))
	want := []Sheet{
		{Role: "sheet", Front: SheetSide{File: "sheet-01-front.tif"}, Back: SheetSide{Blank: true}},
		{Role: "sheet", Front: SheetSide{Blank: true}, Back: SheetSide{File: "sheet-02-back.tif"}},
		{Role: "envelope", Front: SheetSide{File: "sheet-03-front.tif"}, Back: SheetSide{Blank: true}},
	}
	if len(rec.Sheets) != 3 || rec.Sheets[0] != want[0] || rec.Sheets[1] != want[1] || rec.Sheets[2] != want[2] {
		t.Errorf("letter.yaml sheets %+v\nwant %+v", rec.Sheets, want)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "letter.yaml"))
	if n := strings.Count(string(raw), "file:"); n != 3 {
		t.Errorf("letter.yaml names %d files, want 3: a side left out has none\n%s", n, raw)
	}
	for file, value := range map[string]uint16{"sheet-01-front.tif": 40, "sheet-02-back.tif": 90, "sheet-03-front.tif": 60} {
		tif, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		if img, err := master.Decode(bytes.NewReader(tif)); err != nil || img.At(1, 1, 0)>>8 != value {
			t.Errorf("%s: %v, value %d, want %d", file, err, img.At(1, 1, 0)>>8, value)
		}
	}
	if got, want := names(t, l.Renditions(0)), "sheet-01-front sheet-02-back sheet-03-front"; got != want {
		t.Errorf("renditions: %s\nwant: %s", got, want)
	}
	var captions []string
	for _, s := range rec.EverySide() {
		c := s.Caption
		if s.Blank {
			c += " (blank)"
		}
		captions = append(captions, c)
	}
	if got, want := strings.Join(captions, "; "), "Sheet 1, front; Sheet 1, back (blank); Sheet 2, front (blank); Sheet 2, back; Envelope, front; Envelope, back (blank)"; got != want {
		t.Errorf("every side: %s\nwant: %s", got, want)
	}
	if err := l.TurnSide(0, "", 90); err == nil {
		t.Error("a side left out was turned")
	}

	if err := l.Accept(); err != nil {
		t.Fatal(err)
	}
	if rec := runFeed(t, l, store, f); rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s %s", rec.Status, rec.Error)
	}
	if got, want := names(t, filepath.Join(root, "bags", "0", "data")), "letter.yaml sheet-01-front.tif sheet-02-back.tif sheet-03-front.tif"; got != want {
		t.Errorf("the bag's payload: %s\nwant: %s", got, want)
	}
	if d, err := DepositOf(root, 0); err != nil || d == nil || d.Status != "held" {
		t.Errorf("deposit %+v, %v, want held", d, err)
	}
	// the feed's work files still hold every side as fed
	for n := 1; n <= 8; n++ {
		if _, err := os.Stat(l.sidePath(n)); err != nil {
			t.Errorf("side %d as fed: %v", n, err)
		}
	}
}

// Story 181: nothing marked, every side kept, as before.
func TestNothingMarkedBlankKeepsEverySide(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	l, _ := sitting(t, root).NewFeed("", now)
	f := &batchtest.Feeder{Sides: []uint8{40, 255}}
	runFeed(t, l, store, f)
	if err := l.Arrange([]SheetChoice{{Letter: 1, Role: "sheet"}}); err != nil {
		t.Fatal(err)
	}
	if fed, out, _ := l.SidesLeftOut(); fed != 2 || out != 0 {
		t.Errorf("sides fed %d, left out %d", fed, out)
	}
	runFeed(t, l, store, f)
	dir := filepath.Join(l.Dir(), "masters", "0")
	if got, want := names(t, dir), "letter.yaml sheet-01-back.tif sheet-01-front.tif"; got != want {
		t.Errorf("the letter's files: %s", got)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "letter.yaml")); strings.Contains(string(raw), "blank") {
		t.Errorf("letter.yaml speaks of blank sides when none was marked:\n%s", raw)
	}
}

// Story 181: a letter needs a side. A letter whose every side is marked
// blank refuses the arrangement, and nothing is reserved.
func TestALetterOfBlankSidesIsRefused(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	l, _ := sitting(t, root).NewFeed("", now)
	f := &batchtest.Feeder{Sides: []uint8{40, 255, 250, 251}}
	runFeed(t, l, store, f)
	both := SheetChoice{Letter: 2, Role: "sheet", BlankFront: true, BlankBack: true}
	err := l.Arrange([]SheetChoice{{Letter: 1, Role: "sheet"}, both})
	if err == nil || !strings.Contains(err.Error(), "letter 2") || !strings.Contains(err.Error(), "needs a side") {
		t.Fatalf("a letter of blank sides: %v, want it refused saying letter 2 needs a side", err)
	}
	if l.Arranged() {
		t.Error("the refused arrangement was kept")
	}
	if rec := runFeed(t, l, store, f); rec.Status != wf.StatusWaiting {
		t.Fatalf("still waiting at arrange: %s", rec.Status)
	}
	if ids, _ := l.Items(); len(ids) != 0 {
		t.Errorf("ids reserved for a refused arrangement: %v", ids)
	}
}

// Story 181: arranged again before the letters have their ids, the mark
// taken off, the side is back in the letter.
func TestABlankMarkTakenOffPutsTheSideBack(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	l, _ := sitting(t, root).NewFeed("", now)
	f := &batchtest.Feeder{Sides: []uint8{40, 255}}
	runFeed(t, l, store, f)
	if err := l.Arrange([]SheetChoice{{Letter: 1, Role: "sheet", BlankBack: true}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Arrange([]SheetChoice{{Letter: 1, Role: "sheet"}}); err != nil {
		t.Fatal(err)
	}
	if offered, _ := l.Sheets(); len(offered) != 1 || offered[0].BlankBack {
		t.Errorf("sheets offered %+v: the mark was taken off", offered)
	}
	runFeed(t, l, store, f)
	if got, want := names(t, filepath.Join(l.Dir(), "masters", "0")), "letter.yaml sheet-01-back.tif sheet-01-front.tif"; got != want {
		t.Errorf("the letter's files: %s", got)
	}
}
