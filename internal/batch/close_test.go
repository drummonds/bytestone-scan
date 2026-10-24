package batch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// through takes a fresh batch's one scan to the archive: every photo held,
// or rejected for the ids in reject.
func through(t *testing.T, root string, store wf.Store, reject ...pid.ID) (*Batch, *Scan) {
	t.Helper()
	scanner := twoSided()
	l := describe(t, root)
	run(t, l, store, scanner)
	l.Confirm()
	scanned(t, l, store, scanner)
	_, ids, _ := l.Found()
	for i, id := range ids {
		if i < len(reject) {
			service(root).RejectAs[id] = reject[i]
		}
	}
	described(t, l)
	l.Accept()
	if rec := run(t, l, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("scan: %s %s", rec.Status, rec.Error)
	}
	return l.Sitting(), l
}

// Story 042: a batch closes when scanning is finished and every photo is
// held by the archive or rejected by it. Its work files are purged 30
// days after it closes, or sooner, oldest closed first, when staging is
// short of space; an open batch's never. A rejected photo's bag is kept,
// since the archive does not hold its record.
func TestABatchClosesAndIsPurged(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	a, al := through(t, root, store, 7)
	_, ids, _ := al.Found()
	held, rejected := ids[1], ids[0]
	rec, err := a.Finish(ctx, store)
	if err != nil || rec.Status != wf.StatusCompleted || runnertest.Steps(t, store, rec.ID)["close"] != wf.StatusCompleted {
		t.Fatalf("finish: %v %+v %v", err, rec, runnertest.Steps(t, store, rec.ID))
	}
	closedAt, ok := a.Closed()
	if !ok || closedAt.IsZero() {
		t.Fatal("finished batch not closed")
	}
	if _, ok := a.Purged(); ok {
		t.Error("purged on closing")
	}
	open := describe(t, root) // a batch still open, at confirm
	run(t, open, store, twoSided())

	free := uint64(100 << 30)
	p := &Purger{Root: root, Now: func() time.Time { return closedAt.Add(24 * time.Hour) }, Grace: 30 * 24 * time.Hour,
		Threshold: 10 << 30, Free: func(string) (uint64, error) { return free, nil }}
	if purged, err := p.Sweep(); err != nil || len(purged) != 0 {
		t.Fatalf("a day after closing: purged %v, %v", purged, err)
	}
	p.Now = func() time.Time { return closedAt.Add(31 * 24 * time.Hour) }
	purged, err := p.Sweep()
	if err != nil || len(purged) != 1 || purged[0] != a.ID {
		t.Fatalf("31 days after closing: purged %v, %v", purged, err)
	}
	for _, gone := range []string{filepath.Join(a.Dir(), "scans"), filepath.Join(root, "bags", held.String()), filepath.Join(root, "renditions", held.String())} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s survives the purge", gone)
		}
	}
	for _, kept := range []string{filepath.Join(a.Dir(), "batch.yaml"), filepath.Join(a.Dir(), "closed"), filepath.Join(root, "bags", held.String()+".deposit.yaml"),
		filepath.Join(root, "bags", rejected.String()), filepath.Join(open.Dir(), "prints.yaml")} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s purged: %v", kept, err)
		}
	}
	if at, ok := a.Purged(); !ok || !at.Equal(p.Now()) {
		t.Errorf("purged at %v %v, want %v", at, ok, p.Now())
	}
	if scans, _ := a.Scans(); len(scans) != 0 {
		t.Errorf("%d scans after the purge", len(scans))
	}
	if purged, _ := p.Sweep(); len(purged) != 0 {
		t.Errorf("purged again: %v", purged)
	}

	// short of space, closed batches go oldest first until there is room
	c, _ := through(t, root, store)
	c.Finish(ctx, store)
	time.Sleep(10 * time.Millisecond)
	d, _ := through(t, root, store)
	d.Finish(ctx, store)
	cClosed, _ := c.Closed()
	p.Now = func() time.Time { return cClosed.Add(time.Hour) }
	p.Free = func(string) (uint64, error) {
		n := uint64(0)
		for _, b := range []*Batch{c, d} {
			if _, ok := b.Purged(); ok {
				n++
			}
		}
		return 5<<30 + n*20<<30, nil
	}
	purged, err = p.Sweep()
	if err != nil || len(purged) != 1 || purged[0] != c.ID {
		t.Errorf("short of space: purged %v, %v; want the oldest closed, %s", purged, err, c.ID)
	}
	if _, ok := d.Purged(); ok {
		t.Error("the newer closed batch purged though space was back")
	}
	if _, ok := open.Sitting().Purged(); ok {
		t.Error("an open batch purged")
	}
	if dur, err := Purge(root); err != nil || dur == nil {
		t.Errorf("the default purger: %v %v", dur, err)
	}
}

// Story 057: a batch's outcome is how its photos stand with the archive:
// how many it holds, which it rejected as duplicates, how many are not
// there yet.
func TestABatchsOutcomeCountsItsPhotos(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	a, al := through(t, root, store, 7)
	_, ids, _ := al.Found()
	a.Finish(ctx, store)
	o, err := a.Outcome()
	if err != nil || o.Held != 1 || o.Pending != 0 || len(o.Rejected) != 1 || o.Rejected[0] != ids[0] {
		t.Errorf("outcome %+v, %v; want 1 held, %s rejected", o, err, ids[0])
	}
	open := describe(t, root) // an open batch, its prints confirmed and ids reserved
	scanner := twoSided()
	run(t, open, store, scanner)
	open.Confirm()
	run(t, open, store, scanner)
	if o, err := open.Sitting().Outcome(); err != nil || o.Held != 0 || len(o.Rejected) != 0 || o.Pending != 2 {
		t.Errorf("an open batch's outcome %+v, %v; want 2 pending", o, err)
	}
}
