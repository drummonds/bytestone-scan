package batch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// #38: a batch is a sitting of scanning, over as many loads of the bed as
// the shoebox needs; each load is one workflow of its own under the
// batch's, and every photo records both.
func TestABatchIsASittingOfLoads(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := wf.NewMemStore()
	b := sitting(t, root)
	if b.Title != "SS Bonita and crew" || b.ID != "20261020-1" {
		t.Fatalf("batch %+v", b)
	}
	if _, err := b.Finish(ctx, store); err == nil {
		t.Error("finished a batch with no loads")
	}

	// a load's description is its batch's title unless the operator says otherwise
	one, err := b.NewLoad("", now)
	if err != nil {
		t.Fatal(err)
	}
	two, err := b.NewLoad("  the second  envelope ", now)
	if err != nil {
		t.Fatal(err)
	}
	if one.N != 1 || one.Key() != "20261020-1/1" || one.Description != b.Title || two.Description != "the second envelope" {
		t.Errorf("loads %+v %+v", one, two)
	}
	if _, err := os.Stat(filepath.Join(root, "batches", b.ID, "loads", "2", "load.yaml")); err != nil {
		t.Error(err)
	}
	loads, err := b.Loads()
	if err != nil || len(loads) != 2 || loads[1].Description != "the second envelope" {
		t.Errorf("loads: %v %v", loads, err)
	}
	if again, err := OpenLoad(root, "20261020-1/2"); err != nil || again.Description != "the second envelope" || again.batch.ScanPPI != 600 {
		t.Errorf("open load: %+v %v", again, err)
	}

	// a load runs as a child of its batch's run, which waits at loads
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	if rec := run(t, one, store, s); rec.Status != wf.StatusWaiting {
		t.Fatalf("load one: %s", rec.Status)
	}
	parent, _ := store.FindRunByKey(ctx, Workflow, b.ID)
	child, _ := store.FindRunByKey(ctx, LoadWorkflow, one.Key())
	if parent == nil || parent.Status != wf.StatusWaiting || child == nil || child.ParentID != parent.ID {
		t.Fatalf("parent %+v child %+v", parent, child)
	}

	// scanning is finished only once every load is through
	if _, err := b.Finish(ctx, store); err == nil || !strings.Contains(err.Error(), "load") {
		t.Errorf("finished with loads in hand: %v", err)
	}
	for _, l := range []*Load{one, two} {
		if l == two {
			run(t, l, store, s)
		}
		if err := l.Confirm(); err != nil {
			t.Fatal(err)
		}
		scanned(t, l, store, s)
		if err := l.Accept(); err != nil {
			t.Fatal(err)
		}
		if rec := run(t, l, store, s); rec.Status != wf.StatusCompleted {
			t.Fatalf("load %s: %s %s", l.Key(), rec.Status, rec.Error)
		}
	}
	rec, err := b.Finish(ctx, store)
	if err != nil || rec.Status != wf.StatusCompleted {
		t.Fatalf("finish: %v %+v", err, rec)
	}
	if _, err := b.NewLoad("", now); err == nil {
		t.Error("a finished batch took another load")
	}

	// each photo says which batch and load captured it
	_, ids, _ := two.Found()
	photo, _, err := FindPhoto(root, ids[0])
	if err != nil || photo.Batch != b.ID || photo.Load != 2 || photo.Description != "the second envelope" {
		t.Errorf("photo %+v %v", photo, err)
	}
	packet := string(xmp(photo, "front"))
	for _, want := range []string{"<bs:batch>20261020-1</bs:batch>", "<bs:load>2</bs:load>"} {
		if !strings.Contains(packet, want) {
			t.Errorf("xmp lacks %s", want)
		}
	}
	info, _ := os.ReadFile(filepath.Join(root, "archive", ids[0].String(), "bag-info.txt"))
	if !strings.Contains(string(info), "Bytestone-Batch: 20261020-1\nBytestone-Load: 2\n") {
		t.Errorf("bag-info:\n%s", info)
	}
}
