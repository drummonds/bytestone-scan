package batch

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// runFeed ticks a feed with a fake feeder serving sides.
func runFeed(t *testing.T, l *Scan, store wf.Store, f *batchtest.Feeder) *wf.RunRecord {
	t.Helper()
	rec, err := l.Run(t.Context(), Env{Store: store, Feeder: f, Archive: service(l.root), Now: clock}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return rec
}

func readLetter(t *testing.T, path string) Letter {
	t.Helper()
	var rec Letter
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// Story 117: a feed of two sheets, both sides, arranged as one letter
// whose second sheet is the envelope; every side kept, each a master of
// the letter, the letter archived as one item.
func TestFeedOneLetterWithItsEnvelope(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	b := sitting(t, root)
	l, err := b.NewFeed("", now)
	if err != nil {
		t.Fatal(err)
	}
	if !l.IsFeed() || l.Import() || l.N != 1 || l.Description != "SS Bonita and crew" || l.Feed.PPI != FeedPPI {
		t.Errorf("feed %+v %+v", l, l.Feed)
	}
	// sheet 1 front and back (blank), the envelope's front and back
	f := &batchtest.Feeder{Sides: []uint8{40, 255, 90, 120}}
	rec := runFeed(t, l, store, f)
	if rec.Status != wf.StatusWaiting {
		t.Fatalf("first tick: %s %s, want waiting at arrange", rec.Status, rec.Error)
	}
	steps := runnertest.Steps(t, store, rec.ID)
	if steps["feed"] != wf.StatusCompleted || f.Feeds != 1 || f.LastPPI != FeedPPI {
		t.Errorf("steps %v, feeds %d at %d ppi", steps, f.Feeds, f.LastPPI)
	}
	for n, want := range []uint8{40, 255, 90, 120} {
		raw, err := os.ReadFile(filepath.Join(l.Dir(), "sheets", "side-"+string(rune('1'+n))+".tif"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := master.Decode(bytes.NewReader(raw))
		if err != nil || img.At(1, 1, 0)>>8 != uint16(want) {
			t.Errorf("side %d: %v, value %d, want %d", n+1, err, img.At(1, 1, 0)>>8, want)
		}
	}
	fed, err := l.Fed()
	if err != nil || fed.Sides != 4 || fed.PPI != FeedPPI || fed.Depth != 8 || fed.Scanner != "Fake Feeder" || len(fed.Settings) == 0 || fed.Captured.IsZero() {
		t.Errorf("fed.yaml %+v, %v", fed, err)
	}
	sheets, err := l.Sheets()
	if err != nil || len(sheets) != 2 || sheets[0].Letter != 1 || sheets[1].Letter != 1 || sheets[0].Role != "sheet" {
		t.Errorf("sheets offered %+v, %v: want two, each in letter 1, a sheet", sheets, err)
	}

	if err := l.Arrange([]SheetChoice{{Letter: 1, Role: "sheet"}, {Letter: 1, Role: "envelope"}}); err != nil {
		t.Fatal(err)
	}
	rec = runFeed(t, l, store, f)
	if rec.Status != wf.StatusWaiting {
		t.Fatalf("after arrange: %s %s, want waiting at the quality check", rec.Status, rec.Error)
	}
	letters, err := l.Letters()
	if err != nil || len(letters) != 1 || letters[0].ID == nil || *letters[0].ID != 0 || len(letters[0].Sheets) != 2 || letters[0].Sheets[1].Role != "envelope" || letters[0].Sheets[1].N != 2 {
		t.Fatalf("letters %+v, %v: want one, id 0, two sheets, the second the envelope", letters, err)
	}
	if ids, _ := l.Items(); len(ids) != 1 || ids[0] != 0 {
		t.Errorf("items %v, want the letter", ids)
	}
	rec0 := readLetter(t, filepath.Join(l.Dir(), "masters", "0", "letter.yaml"))
	if rec0.ID != 0 || rec0.Batch != b.ID || rec0.Feed != 1 || rec0.Description != "SS Bonita and crew" || rec0.Home.ID != 10 || rec0.Credit != b.Credit ||
		rec0.Scanner != "Fake Feeder" || rec0.PPI != FeedPPI || rec0.Depth != 8 || rec0.Scanned.IsZero() || len(rec0.Sheets) != 2 ||
		rec0.Sheets[0].Role != "sheet" || rec0.Sheets[0].Front.File != "sheet-01-front.tif" || rec0.Sheets[0].Back.File != "sheet-01-back.tif" ||
		rec0.Sheets[1].Role != "envelope" || rec0.Sheets[1].Front.File != "sheet-02-front.tif" || rec0.Sheets[1].Back.File != "sheet-02-back.tif" {
		t.Errorf("letter.yaml %+v", rec0)
	}
	tif, err := os.ReadFile(filepath.Join(l.Dir(), "masters", "0", "sheet-02-front.tif"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<dc:identifier>0</dc:identifier>", "<bs:side>sheet-02-front</bs:side>", "<bs:batch>" + b.ID + "</bs:batch>", "<bs:feed>1</bs:feed>", "<bs:scanner>Fake Feeder</bs:scanner>", "<bs:scanPPI>600</bs:scanPPI>", "<bs:role>envelope</bs:role>"} {
		if !bytes.Contains(tif, []byte(want)) {
			t.Errorf("master's XMP lacks %s", want)
		}
	}
	if img, err := master.Decode(bytes.NewReader(tif)); err != nil || img.At(1, 1, 0)>>8 != 90 {
		t.Errorf("the envelope's front master: %v, value %d, want 90", err, img.At(1, 1, 0)>>8)
	}
	var front, back rendition.Recipe
	for _, side := range []string{"sheet-01-front", "sheet-01-back", "sheet-02-front", "sheet-02-back"} {
		r, err := rendition.Load(filepath.Join(l.Renditions(0), side))
		if err != nil || r.Display.W == 0 {
			t.Errorf("%s developed: %+v, %v", side, r, err)
		}
		switch side {
		case "sheet-01-front":
			front = r
		case "sheet-01-back":
			back = r
		}
	}

	// the operator turns sheet 1's back a quarter turn clockwise: its
	// renditions are made again, turned, and the front's the other way
	// (story 202)
	if err := l.TurnSide(0, "sheet-01-back", 90, false); err != nil {
		t.Fatal(err)
	}
	rec0 = readLetter(t, filepath.Join(l.Dir(), "masters", "0", "letter.yaml"))
	if rec0.Sheets[0].Back.Turn != 90 || rec0.Sheets[0].Front.Turn != 270 {
		t.Errorf("turns %+v", rec0.Sheets[0])
	}
	if turned, err := rendition.Load(filepath.Join(l.Renditions(0), "sheet-01-back")); err != nil || turned.Turn != 90 || turned.Display.W != back.Display.H || turned.Display.H != back.Display.W {
		t.Errorf("turned back %+v (was %+v), %v: want its sides swapped", turned, back, err)
	}
	if front.Display.W == front.Display.H {
		t.Error("the fake sides are square: the turn cannot be seen")
	}

	if err := l.Accept(); err != nil {
		t.Fatal(err)
	}
	if rec = runFeed(t, l, store, f); rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s %s, want completed", rec.Status, rec.Error)
	}
	arc := filepath.Join(root, "bags")
	if !bag.Complete(arc, 0) {
		t.Fatal("letter 0 not bagged")
	}
	for _, name := range []string{"sheet-01-front.tif", "sheet-01-back.tif", "sheet-02-front.tif", "sheet-02-back.tif", "letter.yaml"} {
		if _, err := os.Stat(filepath.Join(arc, "0", "data", name)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(l.Dir(), "masters", "0")); err == nil {
		t.Error("the letter's masters are still in the feed after packaging")
	}
	d, err := DepositOf(root, 0)
	if err != nil || d == nil || d.Status != "held" {
		t.Errorf("deposit %+v, %v, want held", d, err)
	}
	if o, _ := b.Outcome(); o.Held != 1 || o.Pending != 0 {
		t.Errorf("outcome %+v", o)
	}
	if s, err := l.Standing(t.Context(), store); err != nil || !s.Through() {
		t.Errorf("standing %+v, %v, want through", s, err)
	}
	if err := l.TurnSide(0, "sheet-01-back", 90, false); err == nil {
		t.Error("a turn after accepting was allowed")
	}
	if st, err := Statistics(root); err != nil || st.Scans != 1 || st.Photos.Held != 1 {
		t.Errorf("statistics %+v, %v: want the feed among the scans and the letter held", st, err)
	}
}

// Story 117: a load that holds two letters is split at arrange; the
// letters take their ids in letter order and number their sheets afresh.
func TestFeedSplitIntoTwoLetters(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	l, err := sitting(t, root).NewFeed("Two letters from the same envelope", now)
	if err != nil {
		t.Fatal(err)
	}
	f := &batchtest.Feeder{Sides: []uint8{40, 255, 90, 120, 60, 70}}
	runFeed(t, l, store, f)
	// sheet 1 is the second letter, sheets 2 and 3 the first
	if err := l.Arrange([]SheetChoice{{Letter: 2, Role: "sheet"}, {Letter: 1, Role: "sheet"}, {Letter: 1, Role: "envelope"}}); err != nil {
		t.Fatal(err)
	}
	if rec := runFeed(t, l, store, f); rec.Status != wf.StatusWaiting {
		t.Fatalf("after arrange: %s %s", rec.Status, rec.Error)
	}
	letters, _ := l.Letters()
	if len(letters) != 2 || *letters[0].ID != 0 || *letters[1].ID != 1 || len(letters[0].Sheets) != 2 || letters[0].Sheets[0].N != 2 || len(letters[1].Sheets) != 1 || letters[1].Sheets[0].N != 1 {
		t.Fatalf("letters %+v", letters)
	}
	second := readLetter(t, filepath.Join(l.Dir(), "masters", "1", "letter.yaml"))
	if len(second.Sheets) != 1 || second.Sheets[0].Front.File != "sheet-01-front.tif" {
		t.Errorf("the second letter's one sheet is numbered %+v, want sheet 1 of its own", second.Sheets)
	}
	tif, _ := os.ReadFile(filepath.Join(l.Dir(), "masters", "1", "sheet-01-front.tif"))
	if img, err := master.Decode(bytes.NewReader(tif)); err != nil || img.At(1, 1, 0)>>8 != 40 {
		t.Errorf("the second letter's front is the feed's side 1: %v, value %d, want 40", err, img.At(1, 1, 0)>>8)
	}
	if ids, _ := l.Items(); len(ids) != 2 || ids[0] != 0 || ids[1] != 1 {
		t.Errorf("items %v", ids)
	}
	if err := l.Accept(); err != nil {
		t.Fatal(err)
	}
	if rec := runFeed(t, l, store, f); rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s %s", rec.Status, rec.Error)
	}
	if o, _ := l.Outcome(); o.Held != 2 {
		t.Errorf("outcome %+v, want both letters held", o)
	}
}

func TestFeedRefusals(t *testing.T) {
	// a feed that cannot go on fails its feed step, the error saying why
	fails := func(t *testing.T, f Feeder, want string) {
		t.Helper()
		root := t.TempDir()
		l, _ := sitting(t, root).NewFeed("", now)
		rec, err := l.Run(t.Context(), Env{Store: wf.NewMemStore(), Feeder: f, Archive: service(root), Now: clock}, nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v %v, want the feed step to fail saying %q", rec, err, want)
		}
	}
	t.Run("an odd number of sides", func(t *testing.T) { fails(t, &batchtest.Feeder{Sides: []uint8{1, 2, 3}}, "pairs") })
	t.Run("the feeder empty", func(t *testing.T) { fails(t, &batchtest.Feeder{}, "load the sheets") })
	t.Run("no feeder", func(t *testing.T) { fails(t, nil, "feeder") })
	t.Run("arranging", func(t *testing.T) {
		root := t.TempDir()
		store := wf.NewMemStore()
		l, _ := sitting(t, root).NewFeed("", now)
		if err := l.Arrange([]SheetChoice{{Letter: 1, Role: "sheet"}}); err == nil {
			t.Error("arranged before the sheets were fed")
		}
		runFeed(t, l, store, &batchtest.Feeder{Sides: []uint8{1, 2, 3, 4}})
		for name, choices := range map[string][]SheetChoice{
			"too few":       {{Letter: 1, Role: "sheet"}},
			"a bad role":    {{Letter: 1, Role: "sheet"}, {Letter: 1, Role: "postcard"}},
			"letter 0":      {{Letter: 0, Role: "sheet"}, {Letter: 1, Role: "sheet"}},
			"only letter 2": {{Letter: 2, Role: "sheet"}, {Letter: 2, Role: "sheet"}},
		} {
			if err := l.Arrange(choices); err == nil {
				t.Errorf("%s: arranged", name)
			}
		}
		if _, err := l.Letters(); err == nil {
			t.Error("letters before arranging")
		}
	})
	t.Run("a bed scan is not a feed", func(t *testing.T) {
		l := describe(t, t.TempDir())
		if l.IsFeed() {
			t.Error("a bed scan says it is a feed")
		}
		if _, err := l.Fed(); err == nil {
			t.Error("a bed scan has fed.yaml")
		}
	})
}

// A turn is a quarter turn of a side the letter has, before acceptance.
func TestTurnSideRefusals(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	l, _ := sitting(t, root).NewFeed("", now)
	f := &batchtest.Feeder{Sides: []uint8{1, 2}}
	runFeed(t, l, store, f)
	if err := l.Arrange([]SheetChoice{{Letter: 1, Role: "sheet"}}); err != nil {
		t.Fatal(err)
	}
	runFeed(t, l, store, f)
	for name, c := range map[string]struct {
		id   pid.ID
		side string
		by   int
	}{"not a quarter": {0, "sheet-01-front", 45}, "no such side": {0, "sheet-02-front", 90}, "no such letter": {7, "sheet-01-front", 90}} {
		if err := l.TurnSide(c.id, c.side, c.by, false); err == nil {
			t.Errorf("%s: turned", name)
		}
	}
	if err := l.TurnSide(0, "sheet-01-front", -90, false); err != nil {
		t.Errorf("anticlockwise: %v", err)
	}
	if rec := readLetter(t, filepath.Join(l.Dir(), "masters", "0", "letter.yaml")); rec.Sheets[0].Front.Turn != 270 {
		t.Errorf("turn %d, want 270", rec.Sheets[0].Front.Turn)
	}
}
