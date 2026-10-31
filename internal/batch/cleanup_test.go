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

// batchAt describes a batch created at the given time, with one scan.
func batchAt(t *testing.T, root string, store wf.Store, at time.Time) *Scan {
	t.Helper()
	b, err := Describe(root, service(root), store, Description{Text: "at " + at.Format("2 Jan"), Person: 10, ScanPPI: 600, Provenance: "a shoebox", Credit: "c"}, at)
	if err != nil {
		t.Fatal(err)
	}
	l, err := b.NewScan("", at)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// openAt is a batch created at the given time, its fronts scanned and
// nothing deposited.
func openAt(t *testing.T, root string, store wf.Store, at time.Time) *Batch {
	t.Helper()
	l := batchAt(t, root, store, at)
	run(t, l, store, twoSided())
	l.Confirm()
	scanned(t, l, store, twoSided())
	return l.Sitting()
}

// closedAt is a batch created at the given time, every photo held and the
// batch closed.
func closedAt(t *testing.T, root string, store wf.Store, at time.Time) *Batch {
	t.Helper()
	l := batchAt(t, root, store, at)
	scanner := twoSided()
	run(t, l, store, scanner)
	l.Confirm()
	scanned(t, l, store, scanner)
	described(t, l)
	l.Accept()
	if rec := run(t, l, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("scan: %s %s", rec.Status, rec.Error)
	}
	b := l.Sitting()
	if _, err := b.Finish(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Closed(); !ok {
		t.Fatal("not closed")
	}
	return b
}

func ids(bs []*Batch) string {
	var s []string
	for _, b := range bs {
		s = append(s, b.ID)
	}
	return strings.Join(s, " ")
}

// #116: Clean up deletes every closed or purged batch below the one
// chosen in the list, newest first, each as Delete does; the open ones
// below it are kept. With them go the orphans: what no readable batch
// claims under bags/ and renditions/, whatever the archive answered, and
// the runs whose batch has no directory.
func TestCleanUpDeletesCompletedBatchesBelow(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	day := func(d int) time.Time { return now.AddDate(0, 0, d) }
	first := closedAt(t, root, store, day(-4))
	open := openAt(t, root, store, day(-3))
	second := closedAt(t, root, store, day(-2))
	chosen := closedAt(t, root, store, day(-1))
	newest := openAt(t, root, store, day(0))
	_, openIDs, _ := must(open.Scans())[0].Found()
	_, secondIDs, _ := must(second.Scans())[0].Found()

	// a deletion that failed part way: the batch directory gone, its runs,
	// bags and renditions left; and a bag with the archive's answer beside
	// it, and a renditions directory, that no batch claims
	lost := closedAt(t, root, store, day(-5))
	_, lostIDs, _ := must(lost.Scans())[0].Found()
	os.RemoveAll(lost.Dir())
	os.MkdirAll(filepath.Join(root, "bags", "42"), 0o755)
	os.WriteFile(filepath.Join(root, "bags", "42.deposit.yaml"), []byte("status: held\nhome: 10\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "renditions", pid.ID(43).String()), 0o755)

	plan, err := PlanCleanup(ctx, root, store, chosen.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(plan.Batches); got != second.ID+" "+first.ID {
		t.Errorf("to delete %q, want %q", got, second.ID+" "+first.ID)
	}
	if got := ids(plan.Kept); got != open.ID {
		t.Errorf("kept %q, want %q", got, open.ID)
	}
	orphans := map[string]bool{}
	for _, o := range plan.Orphans {
		orphans[o.String()] = true
	}
	for _, want := range []string{"bags/42", "bags/42.deposit.yaml", "renditions/" + pid.ID(43).String(), "bags/" + lostIDs[0].String(), "bags/" + lostIDs[0].String() + ".deposit.yaml", "renditions/" + lostIDs[0].String(), "run " + lost.ID, "run " + lost.ID + "/1", "run photo " + lostIDs[0].String()} {
		if !orphans[want] {
			t.Errorf("orphans %v lack %q", plan.Orphans, want)
		}
	}
	for _, claimed := range []string{"renditions/" + openIDs[0].String(), "bags/" + secondIDs[0].String(), "run " + open.ID, "run photo " + openIDs[0].String()} {
		if orphans[claimed] {
			t.Errorf("orphans list %s, which a batch claims", claimed)
		}
	}

	if err := plan.Run(ctx, store); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, gone := range []string{first.Dir(), second.Dir(), filepath.Join(root, "bags", secondIDs[0].String()), filepath.Join(root, "bags", secondIDs[0].String()+".deposit.yaml"),
		filepath.Join(root, "bags", "42"), filepath.Join(root, "bags", "42.deposit.yaml"), filepath.Join(root, "renditions", pid.ID(43).String()), filepath.Join(root, "bags", lostIDs[0].String())} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s survives: %v", gone, err)
		}
	}
	for _, kept := range []string{open.Dir(), chosen.Dir(), newest.Dir(), filepath.Join(root, "renditions", openIDs[0].String())} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s: %v", kept, err)
		}
	}
	for _, key := range []string{first.ID, second.ID, lost.ID} {
		if r, _ := store.FindRunByKey(ctx, Workflow, key); r != nil {
			t.Errorf("run %s survives", key)
		}
	}
	if r, _ := store.FindRunByKey(ctx, PhotoWorkflow, lostIDs[0].String()); r != nil {
		t.Errorf("the lost batch's photo run survives")
	}
	for _, key := range []string{open.ID, chosen.ID, newest.ID} {
		if r, _ := store.FindRunByKey(ctx, Workflow, key); r == nil {
			t.Errorf("run %s gone", key)
		}
	}

	// nothing left to do: the plan from the oldest row is empty, and from
	// no row at all it is the orphans alone, now none
	for _, below := range []string{open.ID, ""} {
		plan, err := PlanCleanup(ctx, root, store, below)
		if err != nil || len(plan.Batches) != 0 || len(plan.Kept) != 0 || len(plan.Orphans) != 0 {
			t.Errorf("plan below %q: %+v %v, want nothing", below, plan, err)
		}
	}
	if _, err := PlanCleanup(ctx, root, store, "20250101-9"); err == nil {
		t.Error("a plan below a batch that is not there")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
