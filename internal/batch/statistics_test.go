package batch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// #117: Statistics counts the batches by state, the scans and the photos
// by how they stand with the archive, and sizes what staging holds, each
// entry at its root.
func TestStatistics(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	closed, _ := through(t, root, store, 7) // one held, one rejected
	closed.Finish(ctx, store)
	open := describe(t, root) // two photos found, nothing deposited
	run(t, open, store, twoSided())
	open.Confirm()
	scanned(t, open, store, twoSided())
	if _, err := open.Sitting().NewScan("", now); err != nil { // a second scan, empty
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "workflow.db"), make([]byte, 3000), 0o644)

	s, err := Statistics(root)
	if err != nil {
		t.Fatal(err)
	}
	if s.Batches["open"] != 1 || s.Batches["closed"] != 1 || s.Batches["finished"] != 0 || s.Batches["purged"] != 0 {
		t.Errorf("batches %v, want 1 open, 1 closed", s.Batches)
	}
	if s.Scans != 3 {
		t.Errorf("scans %d, want 3", s.Scans)
	}
	if s.Photos.Held != 1 || len(s.Photos.Rejected) != 1 || s.Photos.Pending != 2 {
		t.Errorf("photos %+v, want 1 held, 1 rejected, 2 pending", s.Photos)
	}
	sizes := map[string]int64{}
	for _, u := range s.Staging {
		sizes[u.Name] = u.Bytes
	}
	if sizes["workflow.db"] != 3000 {
		t.Errorf("workflow.db %d bytes, want 3000", sizes["workflow.db"])
	}
	for _, dir := range []string{"batches", "bags", "renditions"} {
		if sizes[dir] <= 0 {
			t.Errorf("%s: %d bytes", dir, sizes[dir])
		}
	}
	if closed.State() != "closed" || open.Sitting().State() != "open" {
		t.Errorf("states %s %s", closed.State(), open.Sitting().State())
	}

	// purged, finished
	p := &Purger{Root: root, Now: func() time.Time { return time.Now().Add(40 * 24 * time.Hour) }, Grace: DefaultGrace, Threshold: 0}
	if swept, err := p.Sweep(); err != nil || len(swept) != 1 {
		t.Fatalf("sweep: %v %v", swept, err)
	}
	if s, _ = Statistics(root); s.Batches["purged"] != 1 || s.Batches["closed"] != 0 {
		t.Errorf("after purging: %v", s.Batches)
	}

	// an empty root
	if s, err := Statistics(t.TempDir()); err != nil || len(s.Batches) != 0 || s.Scans != 0 || len(s.Staging) != 0 {
		t.Errorf("empty root: %+v %v", s, err)
	}
}
