package batch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Story 054, widened by #98 and #100: any batch can be deleted, open,
// closed or purged: its runs go from the workflow database with their
// steps, and its work files, bags, renditions and the archive's answers
// go from staging. Nothing the archive holds is touched, and the batch's
// id is not given out again.
func TestDeleteABatch(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	l := describe(t, root) // open: its fronts scanned, its photos started, nothing deposited
	run(t, l, store, scanner)
	l.Confirm()
	scanned(t, l, store, scanner)
	_, ids, _ := l.Found()
	b := l.Sitting()
	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(root, "renditions", id.String())); err != nil {
			t.Fatalf("photo %s has no renditions to delete: %v", id, err)
		}
	}
	if err := b.Delete(ctx, store); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(b.Dir()); !os.IsNotExist(err) {
		t.Errorf("batch directory survives: %v", err)
	}
	for _, id := range ids {
		for _, gone := range []string{filepath.Join(root, "bags", id.String()), filepath.Join(root, "renditions", id.String())} {
			if _, err := os.Stat(gone); !os.IsNotExist(err) {
				t.Errorf("%s survives: %v", gone, err)
			}
		}
		if r, _ := store.FindRunByKey(ctx, PhotoWorkflow, id.String()); r != nil {
			t.Errorf("photo %s run survives: %+v", id, r)
		}
	}
	for _, run := range []struct {
		t   wf.WorkflowType
		key string
	}{{Workflow, b.ID}, {ScanWorkflow, l.Key()}} {
		if r, _ := store.FindRunByKey(ctx, run.t, run.key); r != nil {
			t.Errorf("%s %s run survives: %+v", run.t, run.key, r)
		}
	}

	// the deleted batch's id is not given out again, though its runs are gone
	if again, err := Describe(root, service(root), store, Description{Text: "again", Person: 10, ScanPPI: 600, Credit: "c"}, now); err != nil || again.ID != "20261020-2" {
		t.Errorf("the next batch after deleting 20261020-1: %+v %v, want 20261020-2", again, err)
	}

	// a photo held by the archive, the batch closed: deleted all the same,
	// the archive keeping the photo; its bags, renditions and the archive's
	// answers go from staging
	root, store = t.TempDir(), wf.NewMemStore()
	held, hl := through(t, root, store, 7)
	_, hids, _ := hl.Found()
	held.Finish(ctx, store)
	if _, ok := held.Closed(); !ok {
		t.Fatal("not closed")
	}
	if err := held.Delete(ctx, store); err != nil {
		t.Fatalf("deleting a closed batch with a photo held: %v", err)
	}
	for _, gone := range []string{held.Dir(), filepath.Join(root, "bags", hids[0].String()), filepath.Join(root, "bags", hids[1].String()), filepath.Join(root, "bags", hids[1].String()+".deposit.yaml"), filepath.Join(root, "renditions", hids[1].String())} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s survives: %v", gone, err)
		}
	}

	// purged: the record that is left goes too
	root, store = t.TempDir(), wf.NewMemStore()
	purged, _ := through(t, root, store, 7)
	purged.Finish(ctx, store)
	p := &Purger{Root: root, Now: func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }, Grace: 30 * 24 * time.Hour}
	if swept, err := p.Sweep(); err != nil || len(swept) != 1 {
		t.Fatalf("sweep: %v %v", swept, err)
	}
	if err := purged.Delete(ctx, store); err != nil {
		t.Fatalf("deleting a purged batch: %v", err)
	}
	if _, err := os.Stat(purged.Dir()); !os.IsNotExist(err) {
		t.Errorf("purged batch's directory survives: %v", err)
	}
}

// Stale data is what no readable batch claims: batch directories in a
// layout the code cannot read, and id directories under bags/ and
// renditions/ with no batch behind them and no answer from the archive.
// A rejected photo's bag, kept on purpose after its batch is purged, is
// not stale; nor is anything a readable batch claims.
func TestStaleDataIsListedAndDeleted(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	b, l := through(t, root, store, 7) // its ids claimed; one rejected, one held
	_, ids, _ := l.Found()
	b.Finish(ctx, store)
	p := &Purger{Root: root, Now: func() time.Time { return time.Now().Add(40 * 24 * time.Hour) }, Grace: DefaultGrace, Threshold: 0} // closed at the real clock
	if purged, err := p.Sweep(); err != nil || len(purged) != 1 {
		t.Fatalf("purge: %v %v", purged, err)
	}
	open := describe(t, root) // claims its ids
	run(t, open, store, twoSided())
	open.Confirm()
	run(t, open, store, twoSided())
	_, openIDs, _ := open.Found()

	old := filepath.Join(root, "batches", "20250101-1")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "prints.yaml"), []byte("prints: []\n"), 0o644)
	for _, dir := range []string{"bags/42", "renditions/42", "renditions/" + pid.ID(43).String()} {
		os.MkdirAll(filepath.Join(root, dir), 0o755)
	}

	stale, err := ListStale(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, s := range stale {
		paths = append(paths, s.Path)
	}
	want := []string{"batches/20250101-1", "bags/42", "renditions/" + pid.ID(43).String(), "renditions/42"}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Errorf("stale %v, want %v", paths, want)
	}
	for _, s := range stale {
		if err := DeleteStale(root, s.Path); err != nil {
			t.Errorf("delete %s: %v", s.Path, err)
		}
		if _, err := os.Stat(filepath.Join(root, s.Path)); !os.IsNotExist(err) {
			t.Errorf("%s survives: %v", s.Path, err)
		}
	}
	if stale, _ := ListStale(root); len(stale) != 0 {
		t.Errorf("still stale: %+v", stale)
	}
	// what is not listed cannot be deleted this way
	for _, path := range []string{"bags/" + ids[0].String(), "batches/" + open.Batch, "renditions/" + openIDs[0].String(), "../" + filepath.Base(root), "workflow.db"} {
		if err := DeleteStale(root, path); err == nil {
			t.Errorf("deleted %s, which is not stale", path)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "bags", ids[0].String())); err != nil {
		t.Errorf("the rejected photo's bag: %v", err)
	}
}
