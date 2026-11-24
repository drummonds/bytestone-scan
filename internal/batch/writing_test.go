package batch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"
	lw "git.bytestone.uk/hum3/life-weaver/writing"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/writing"
	"git.bytestone.uk/hum3/bytestone-scan/internal/writing/writingtest"
)

func importWriting(t *testing.T, b *Batch, name string, data []byte) *Scan {
	t.Helper()
	l, err := b.NewWriting(WritingImport{File: name, Data: bytes.NewReader(data)}, now)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func runWriting(t *testing.T, l *Scan, store wf.Store) *wf.RunRecord {
	t.Helper()
	rec, err := l.Run(t.Context(), Env{Store: store, Archive: service(l.root), Now: clock}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return rec
}

// describedWriting is a writing imported, read and described with nothing
// known, its workflow not yet ticked past describe.
func describedWriting(t *testing.T, b *Batch, store wf.Store, name string, data []byte) *Scan {
	t.Helper()
	l := importWriting(t, b, name, data)
	runWriting(t, l, store)
	if err := l.ConfirmWriting(l.Writing.Title); err != nil {
		t.Fatal(err)
	}
	runWriting(t, l, store)
	if err := l.DescribeWriting(service(l.root), WritingDescription{}, now); err != nil {
		t.Fatal(err)
	}
	return l
}

// files are the regular files under dir, relative to it.
func files(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			found = append(found, rel)
		}
		return nil
	})
	return found
}

// Story 190: a Word document is imported as a scan of its batch, numbered
// among them, converted to djot and held with the file received; read
// and described, it is packaged as a writing, its djot and its record
// alone, the received file let go; deposited and receipted.
func TestImportAWritingToTheArchive(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b := sitting(t, root)
	if _, err := b.NewScan("", now); err != nil { // a bed scan first: the writing is numbered after it
		t.Fatal(err)
	}
	sent := writingtest.Sample(t)
	sum := sha256.Sum256(sent)
	l := importWriting(t, b, `C:\Users\jane\Recollections of Megginch.DOCX`, sent)
	if !l.IsWriting() || l.IsFilm() || l.IsFeed() || l.Import() || l.N != 2 {
		t.Fatalf("writing import %+v: want a writing, scan 2 of the batch", l)
	}
	if scans, _ := b.Scans(); len(scans) != 2 || !scans[1].IsWriting() {
		t.Errorf("the batch's scans %v: want the writing among them", scans)
	}
	w := l.Writing
	if w.Title != "Recollections of Megginch" || l.Description != w.Title || w.Read != nil || w.ID != nil ||
		w.Received != (ReceivedFile{Name: "Recollections of Megginch.DOCX", Format: "docx", Size: int64(len(sent)), SHA256: hex.EncodeToString(sum[:]), LeftOut: writing.LeftOut{Pictures: 1}}) {
		t.Errorf("as imported: %+v", w)
	}
	djot, err := l.WritingText()
	if err != nil || lw.Check(djot) != nil || lw.Words(djot) != "Megginch The castle stands in the Carse. We left in 1921. Near Errol." {
		t.Errorf("the djot %q, %v", djot, err)
	}
	if rec := runWriting(t, l, store); rec.Status != wf.StatusWaiting || rec.WorkflowType != WritingImportWorkflow {
		t.Fatalf("first tick: %s %s %s, want waiting at read", rec.WorkflowType, rec.Status, rec.Error)
	}

	// read: the operator goes on, the title theirs to put right; a writing has one
	if err := l.ConfirmWriting(" \t "); err == nil || !strings.Contains(err.Error(), "a writing has a title") {
		t.Errorf("a blank title: %v, want refused: a writing has a title", err)
	}
	if again, _ := b.Scan(l.N); again.Writing.Read != nil {
		t.Fatal("a refused confirm confirmed the writing")
	}
	if err := l.ConfirmWriting("  Recollections  of Megginch "); err != nil {
		t.Fatal(err)
	}
	if rec := runWriting(t, l, store); rec.Status != wf.StatusWaiting {
		t.Fatalf("after read: %s %s, want waiting at describe", rec.Status, rec.Error)
	}
	if ids, _ := l.Items(); len(ids) != 0 {
		t.Errorf("items %v before the writing is described", ids)
	}

	svc := service(root)
	people := []RoleChoice{{Person: "a", Role: "author"}, {Person: "e", Role: "subject"}}
	for _, c := range []struct {
		d    WritingDescription
		want string
	}{
		{WritingDescription{People: []RoleChoice{{Role: "author"}}}, "a part needs its person"},
		{WritingDescription{People: []RoleChoice{{Person: "a", Role: "director"}}}, "role"},
		{WritingDescription{People: []RoleChoice{{Person: "zz", Role: "author"}}}, "register"},
		{WritingDescription{Transmission: []HandChoice{{Part: "typed"}}}, "a hand needs a name"},
		{WritingDescription{Transmission: []HandChoice{{Part: "typed", Person: "a", Name: "Jane Smith"}}}, "one or the other"},
		{WritingDescription{Transmission: []HandChoice{{Part: "printed", Name: "Jane Smith"}}}, "part"},
		{WritingDescription{Transmission: []HandChoice{{Name: "Jane Smith"}}}, "part"},
		{WritingDescription{Transmission: []HandChoice{{Part: "sent", Person: "zz"}}}, "register"},
		{WritingDescription{Transmission: []HandChoice{{Part: "sent", Name: "jean"}}}, "reads as a person's id"},
		{WritingDescription{Transmission: []HandChoice{{Part: "sent", Name: "Jane Smith", Date: "last week"}}}, "ISO 8601"},
		{WritingDescription{Written: MadeClaim{Value: "the seventies"}}, "ISO 8601"},
		{WritingDescription{Written: MadeClaim{Value: "1974", Source: "film"}}, "source"},
	} {
		if err := l.DescribeWriting(svc, c.d, now); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v, want refused, saying %q", c.d, err, c.want)
		}
	}
	if again, _ := b.Scan(l.N); again.Writing.Described != nil {
		t.Fatal("a refused description described the writing")
	}
	err = l.DescribeWriting(svc, WritingDescription{
		People:  append(people, RoleChoice{}),
		Written: MadeClaim{Value: "1974", Approximate: true},
		Transmission: []HandChoice{
			{Part: "typed", Person: "a", Date: "1998", Note: " from the  manuscript "},
			{},
			{Part: "sent", Name: " Jane  Smith ", Date: "2026-11-02", Note: "by email"},
			{Part: "kept", Name: "Jean"}, // a name, no date and no note
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if rec := runWriting(t, l, store); rec.Status != wf.StatusCompleted {
		t.Fatalf("after describe: %s %s, want through", rec.Status, rec.Error)
	}
	ids, _ := l.Items()
	if len(ids) != 1 || ids[0] != 0 {
		t.Fatalf("items %v, want the writing, item 0", ids)
	}
	id := ids[0]
	if run, _ := store.FindRunByKey(t.Context(), WritingWorkflow, id.String()); run == nil || run.Status != wf.StatusCompleted {
		t.Fatalf("the writing's run %+v: steps %v", run, runnertest.Steps(t, store, run.ID))
	}

	// the bag: the djot and the record, and nothing else
	bagDir := filepath.Join(root, "bags", id.String())
	manifest, err := bag.Manifest(bagDir)
	if err != nil {
		t.Fatal(err)
	}
	bagged, _ := os.ReadFile(filepath.Join(bagDir, "data", "writing.dj"))
	if len(manifest) != 2 || manifest["data/writing.dj"] == "" || manifest["data/writing.yaml"] == "" || !bytes.Equal(bagged, djot) {
		t.Errorf("manifest %v: want writing.dj, as converted, and writing.yaml alone", manifest)
	}
	raw, _ := os.ReadFile(filepath.Join(bagDir, "data", "writing.yaml"))
	var rec WritingRecord
	if err := yaml.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	hands := []Hand{
		{Part: "typed", By: By{Person: 10}, Date: "1998", Note: "from the manuscript"},
		{Part: "sent", By: By{Name: "Jane Smith"}, Date: "2026-11-02", Note: "by email"},
		{Part: "kept", By: By{Name: "Jean"}},
	}
	if rec.ID != id || rec.Batch != b.ID || rec.Writing != 2 || rec.Title != "Recollections of Megginch" || rec.Home != (Person{10, "Ada Example"}) ||
		rec.Provenance != b.Provenance || rec.Credit != b.Credit ||
		len(rec.People) != 2 || rec.People[0] != (Role{Person: 10, Role: "author"}) || rec.People[1] != (Role{Person: 14, Role: "subject"}) ||
		rec.Written == nil || *rec.Written != (Made{Value: "1974", Approximate: true, Source: "family"}) ||
		len(rec.Transmission) != 3 || rec.Transmission[0] != hands[0] || rec.Transmission[1] != hands[1] || rec.Transmission[2] != hands[2] ||
		rec.Received != w.Received {
		t.Errorf("writing.yaml %+v", rec)
	}
	// as life-weaver reads it: the keys by name, a hand's person an id and its name a name
	for _, key := range []string{"title: Recollections of Megginch", "writing: 2", "people:", "role: author", "written:", "value: \"1974\"", "approximate: true",
		"transmission:", "part: typed", "by: a\n", "by: Jane Smith\n", "date: \"1998\"", "note: from the manuscript", "received:", "name: Recollections of Megginch.DOCX",
		"format: docx", "sha256: " + w.Received.SHA256, "left_out:", "pictures: 1", "tables: 0", "objects: 0", "credit: "} {
		if !bytes.Contains(raw, []byte(key)) {
			t.Errorf("writing.yaml lacks %q:\n%s", key, raw)
		}
	}
	// no renditions, and the received file gone: no copy of it anywhere in staging
	if _, err := os.Stat(l.Renditions(id)); err == nil {
		t.Error("renditions were made of a writing")
	}
	for _, f := range files(t, root) {
		data, _ := os.ReadFile(filepath.Join(root, f))
		if strings.Contains(strings.ToLower(f), ".docx") || bytes.Equal(data, sent) || bytes.HasPrefix(data, []byte("PK")) {
			t.Errorf("%s: the received file is still in staging", f)
		}
	}
	// the archive's answer: held, a writing, checksum for checksum
	d, err := DepositOf(root, id)
	if err != nil || d == nil || d.Status != "held" || d.Kind != "writing" || d.Home != 10 || d.Payload["data/writing.dj"] != manifest["data/writing.dj"] {
		t.Errorf("deposit %+v, %v", d, err)
	}
	if c := check(t, l, svc, id); c.Verdict != Verified || c.Changed {
		t.Errorf("verify %+v", c)
	}
	// its record another checksum at the archive since: no fault (story 191)
	held, _ := svc.Receipt(id)
	svc.Hold(id, map[string]string{"data/writing.dj": held.Payload["data/writing.dj"], "data/writing.yaml": strings.Repeat("0", 64)})
	if c := check(t, l, svc, id); c.Verdict != Verified || !c.Changed {
		t.Errorf("verify, the record changed since: %+v, want verified and changed", c)
	}
	svc.Hold(id, map[string]string{"data/writing.dj": strings.Repeat("0", 64), "data/writing.yaml": held.Payload["data/writing.yaml"]})
	if c := check(t, l, svc, id); c.Verdict != Differs {
		t.Errorf("verify, the djot another checksum: %+v, want differs", c)
	}
	// found again by its id, from its bag, with its text
	if got, text, err := FindWriting(root, id); err != nil || got.Title != rec.Title || !bytes.Equal(text, djot) {
		t.Errorf("FindWriting %+v, %v", got, err)
	}
	// described and packaged, the writing is no longer the import's to change
	if err := l.DescribeWriting(svc, WritingDescription{}, now); err == nil {
		t.Error("a packaged writing was described again")
	}
	if err := l.ConfirmWriting("Another title"); err == nil {
		t.Error("a packaged writing was given another title")
	}
	// counted among what staging holds, among the photos as a film is
	s, err := Statistics(root)
	if err != nil || s.Photos.Held != 1 || s.Photos.Pending != 0 || s.Scans != 2 {
		t.Errorf("statistics %+v, %v: want the writing counted held", s, err)
	}
}

// Nothing is required of a description; and the same file imported twice
// converts to the same bytes, so the archive refuses the second as a
// duplicate, naming the first, and it ends as a rejected photo does.
func TestTheSameWritingTwiceIsADuplicate(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b := sitting(t, root)
	text := []byte("Dear all,\r\nthe reunion was a success.\r\n\r\nJean\r\n")
	service(root).Next = 5 // 0 is no id to be a duplicate of (story 191)
	first := describedWriting(t, b, store, "reunion.txt", text)
	if rec := runWriting(t, first, store); rec.Status != wf.StatusCompleted {
		t.Fatalf("the first: %s %s", rec.Status, rec.Error)
	}
	second := describedWriting(t, b, store, "reunion again.txt", text)
	if rec := runWriting(t, second, store); rec.Status != wf.StatusCompleted {
		t.Fatalf("the second: %s %s", rec.Status, rec.Error)
	}
	one, err := os.ReadFile(filepath.Join(root, "bags", "5", "data", "writing.dj"))
	two, err2 := os.ReadFile(filepath.Join(root, "bags", "6", "data", "writing.dj"))
	if err != nil || err2 != nil || !bytes.Equal(one, two) {
		t.Errorf("the two writing.dj differ, or are not there: %v %v\n%s\n%s", err, err2, one, two)
	}
	rec, _, err := FindWriting(root, 5)
	if err != nil || rec.Title != "reunion" || len(rec.People) != 0 || rec.Written != nil || len(rec.Transmission) != 0 || rec.Received.Format != "txt" {
		t.Errorf("a writing of which nothing is known: %+v, %v", rec, err)
	}
	if d, _ := DepositOf(root, 5); d == nil || d.Status != "held" {
		t.Errorf("the first: %+v, want held", d)
	}
	if d, _ := DepositOf(root, 6); d == nil || d.Status != "duplicate" || d.DuplicateOf != 5 || d.File != "data/writing.dj" {
		t.Errorf("the second: %+v, want a duplicate of item 5 by its djot", d)
	}
	if c := check(t, second, service(root), 6); c.Verdict != Rejected || !strings.Contains(c.Detail, "a duplicate of item 5") {
		t.Errorf("verify of the second: %+v, want rejected, naming item 5", c)
	}
	if o, err := b.Outcome(); err != nil || o.Held != 1 || len(o.Rejected) != 1 || o.Rejected[0] != 6 || o.Pending != 0 {
		t.Errorf("outcome %+v, %v: want one held and one rejected", o, err)
	}
}

// What cannot be converted is refused, and nothing of it kept in staging.
func TestImportRefusesWhatIsNotAWriting(t *testing.T) {
	root := t.TempDir()
	b := sitting(t, root)
	macros := writingtest.Docx(t, writingtest.P("Words."), writingtest.Part{Name: "word/vbaProject.bin", XML: "macros"})
	for _, c := range []struct {
		name string
		data []byte
		want error
	}{
		{"memoir.docx", []byte("not a zip"), writing.ErrNotWord},
		{"memoir.docx", macros, writing.ErrMacros},
		{"memoir.docm", writingtest.Sample(t), writing.ErrMacros},
		{"memoir.txt", []byte("\n \n"), writing.ErrNothingToRead},
		{"memoir.odt", []byte("words"), writing.ErrFormat},
		{"memoir.pdf", []byte("%PDF-1.4"), writing.ErrFormat},
		{".docx", writingtest.Sample(t), writing.ErrFormat},
	} {
		if _, err := b.NewWriting(WritingImport{File: c.name, Data: bytes.NewReader(c.data)}, now); !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%s: %v, want refused, naming it: %v", c.name, err, c.want)
		}
	}
	if _, err := b.NewWriting(WritingImport{}, now); err == nil {
		t.Error("nothing was imported as a writing")
	}
	if scans, _ := b.Scans(); len(scans) != 0 {
		t.Errorf("%d scans after refusals, want none", len(scans))
	}
	if kept := files(t, b.Dir()); len(kept) != 1 || kept[0] != "batch.yaml" {
		t.Errorf("kept in staging after refusals: %v", kept)
	}
}

func TestAFinishedBatchTakesNoWriting(t *testing.T) {
	b := sitting(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(b.Dir(), "finished"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := b.NewWriting(WritingImport{File: "memoir.txt", Data: strings.NewReader("Words.")}, now)
	if err == nil || !strings.Contains(err.Error(), "finished") {
		t.Errorf("a finished batch: %v, want refused", err)
	}
}

// Story 191: a writing packaged and not deposited, its batch's person
// gone from the register, has its record written again with the home
// chosen; its djot is as it was, byte for byte.
func TestAPackagedWritingIsRehomed(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b := sitting(t, root)
	svc := service(root)
	l := describedWriting(t, b, store, "memoir.docx", writingtest.Sample(t))
	// the person is renumbered as the bag is written: after the id is
	// reserved, before the deposit
	renumbered := func() time.Time { svc.Renumber(10, 11); return now }
	_, err := l.Run(t.Context(), Env{Store: store, Archive: svc, Now: renumbered}, nil)
	if err == nil || !strings.Contains(err.Error(), "the batch names a, the archive b") {
		t.Fatalf("a deposit under a home the archive does not give the id: %v, want refused naming both", err)
	}
	dj := filepath.Join(root, "bags", "0", "data", "writing.dj")
	was, err := os.ReadFile(dj)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Rehome(svc, 11); err != nil {
		t.Fatal(err)
	}
	rec, text, err := FindWriting(root, 0)
	if err != nil || rec.Home != (Person{11, "Ada Example"}) || rec.ID != 0 || rec.Title != "memoir" || !bytes.Equal(text, was) {
		t.Errorf("after the re-home: %+v, %v: want home b, the djot as it was", rec, err)
	}
	if now, _ := os.ReadFile(dj); !bytes.Equal(now, was) {
		t.Error("the djot was rewritten by the re-home")
	}
	again, _ := Open(root, b.ID)
	if l, err = again.Scan(l.N); err != nil {
		t.Fatal(err)
	}
	runWriting(t, l, store) // the import is through already: its writing is ticked on
	if d, _ := DepositOf(root, 0); d == nil || d.Status != "held" || d.Home != 11 || d.Kind != "writing" {
		t.Errorf("deposit after the re-home: %+v, want held, home b", d)
	}
}

// A closed batch's writing goes with its scans when the batch is purged.
func TestAWritingIsPurgedWithItsBatch(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	b := sitting(t, root)
	l := describedWriting(t, b, store, "memoir.docx", writingtest.Sample(t))
	if rec := runWriting(t, l, store); rec.Status != wf.StatusCompleted {
		t.Fatalf("writing: %s %s", rec.Status, rec.Error)
	}
	rec, err := b.Finish(t.Context(), store)
	if err != nil || rec.Status != wf.StatusCompleted {
		t.Fatalf("finish: %+v, %v", rec, err)
	}
	closedAt, ok := b.Closed()
	if !ok {
		t.Fatal("the batch did not close with its writing held")
	}
	p := &Purger{Root: root, Now: func() time.Time { return closedAt.Add(31 * 24 * time.Hour) }, Grace: DefaultGrace, Threshold: 0}
	if purged, err := p.Sweep(); err != nil || len(purged) != 1 {
		t.Fatalf("purged %v, %v", purged, err)
	}
	for _, gone := range []string{filepath.Join(b.Dir(), "scans"), filepath.Join(root, "bags", "0")} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s survives the purge", gone)
		}
	}
	if checks, err := l.Verify(service(root)); err != nil || len(checks) != 1 || checks[0].Verdict != Verified {
		t.Errorf("verify after the purge %+v, %v", checks, err)
	}
}

// A hand's by is a person's id or a name as written, and reads back as
// it was written.
func TestAHandIsAPersonOrAName(t *testing.T) {
	for _, by := range []By{{Person: 10}, {Person: pid.ID(1027)}, {Name: "Jane Smith"}, {Name: "Jean"}, {Name: "3rd Battalion clerk"}} {
		raw, err := yaml.Marshal(Hand{Part: "kept", By: by})
		if err != nil {
			t.Fatal(err)
		}
		var back Hand
		if err := yaml.Unmarshal(raw, &back); err != nil || back.By != by {
			t.Errorf("%+v written %q, read back %+v, %v", by, raw, back.By, err)
		}
	}
}
