package batch

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// twoSided is a bed of two prints whose backs, turned over in place, land
// where the fronts lay: the first a little to one side, the second 10 mm
// narrower than its front, which wants a look.
func twoSided() *batchtest.Scanner {
	return &batchtest.Scanner{
		Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}},
		Backs:  []batchtest.Print{{X0: 22, Y0: 20, X1: 62, Y1: 50, Value: 50000}, {X0: 90, Y0: 25, X1: 115, Y1: 60, Value: 50000}},
	}
}

// photoRun is the photo id's workflow run.
func photoRun(t *testing.T, store wf.Store, id pid.ID) *wf.RunRecord {
	t.Helper()
	run, err := store.FindRunByKey(context.Background(), PhotoWorkflow, id.String())
	if err != nil || run == nil {
		t.Fatalf("photo %s: no run (%v)", id, err)
	}
	return run
}

// Story 039: after the fronts, the operator turns the prints over in place
// and the load scans the backs where the fronts lay, at 300 ppi and 8 bits,
// reviewing a back that differs from its front. Each photo is a workflow of
// its own under the load, developing each side as it comes and archiving
// both.
func TestBacksAreScannedWhereTheFrontsLay(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	b := describe(t, root)
	run(t, b, store, scanner)
	if err := b.Turned(); err == nil {
		t.Error("turned over before the fronts were scanned")
	}
	b.Confirm()
	rec := run(t, b, store, scanner)
	if steps := runnertest.Steps(t, store, rec.ID); rec.Status != wf.StatusWaiting || steps["backs"] != wf.StatusWaiting || steps["fronts"] != wf.StatusCompleted || steps["photos"] != wf.StatusCompleted {
		t.Fatalf("after the fronts: %s %v", rec.Status, steps)
	}
	// the fronts are developed at once; each photo then waits for its back
	for i := range scanner.Prints {
		id := pid.ID(i)
		if _, err := rendition.Load(b.Renditions(id)); err != nil {
			t.Errorf("front of %s not developed: %v", id, err)
		}
		if pr := photoRun(t, store, id); pr.ParentID != rec.ID || pr.Status != wf.StatusWaiting || runnertest.Steps(t, store, pr.ID)["awaiting back"] != wf.StatusWaiting {
			t.Errorf("photo %s: %+v", id, pr)
		}
	}

	if err := b.Turned(); err != nil {
		t.Fatal(err)
	}
	rec = run(t, b, store, scanner)
	if steps := runnertest.Steps(t, store, rec.ID); rec.Status != wf.StatusWaiting || steps["check quality"] != wf.StatusWaiting || steps["backs"] != wf.StatusCompleted {
		t.Fatalf("after the backs: %s %v", rec.Status, steps)
	}
	if scanner.Previews != 2 || scanner.Details != 2 || scanner.LastPPI != 300 {
		t.Errorf("previews %d, details %d, last at %d ppi; want 2, 2 and 300", scanner.Previews, scanner.Details, scanner.LastPPI)
	}
	if _, err := os.Stat(b.path("backs.png")); err != nil {
		t.Error(err)
	}
	var photos []Photo
	for i, p := range scanner.Backs {
		id := pid.ID(i)
		var photo Photo
		if err := readYAML(filepath.Join(b.path("masters"), id.String()+".yaml"), &photo); err != nil {
			t.Fatal(err)
		}
		photos = append(photos, photo)
		back := photo.Back
		if back == nil {
			t.Fatalf("photo %s has no back", id)
		}
		if back.ScanPPI != 300 || back.MasterPPI != 300 || back.Averaging != 1 || back.Bounds != photo.Front.Bounds || back.Region != photo.Front.Region || !back.Captured.Equal(now) {
			t.Errorf("photo %s back %+v", id, back)
		}
		if math.Abs(back.Print.CX-(p.X0+p.X1)/2) > 1 || math.Abs(back.Print.W-(p.X1-p.X0)) > 1 {
			t.Errorf("photo %s back print at %+v, want about %+v", id, back.Print, p)
		}
		tif, err := os.ReadFile(filepath.Join(b.path("masters"), id.String()+".back.tif"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := master.Decode(bytes.NewReader(tif))
		if err != nil {
			t.Fatal(err)
		}
		if wantW := int(photo.Front.Bounds.X1-photo.Front.Bounds.X0) * 300 / 25; img.W < wantW*25/26 || img.W > wantW*26/25 {
			t.Errorf("photo %s back master %d px wide, want about %d", id, img.W, wantW)
		}
		if !bytes.Contains(tif, []byte("<bs:side>back</bs:side>")) || !bytes.Contains(tif, []byte("<bs:scanPPI>300</bs:scanPPI>")) || bytes.Contains(tif, []byte("<bs:side>front")) {
			t.Errorf("photo %s back's XMP is wrong", id)
		}
		if _, err := rendition.Load(filepath.Join(b.Renditions(id), "back")); err != nil {
			t.Errorf("back of %s not developed: %v", id, err)
		}
		if pr := photoRun(t, store, id); runnertest.Steps(t, store, pr.ID)["accepted"] != wf.StatusWaiting {
			t.Errorf("photo %s not waiting to be accepted", id)
		}
	}
	if photos[0].Back.Review || photos[0].Back.Warning != "" {
		t.Errorf("a back where its front lay was flagged: %+v", photos[0].Back)
	}
	if !photos[1].Back.Review || !strings.Contains(photos[1].Back.Warning, "25 × 35 mm") || !strings.Contains(photos[1].Back.Warning, "35 × 35 mm") {
		t.Errorf("a back 10 mm narrower than its front: %+v", photos[1].Back)
	}

	described(t, b)
	b.Accept()
	if rec = run(t, b, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s %s", rec.Status, rec.Error)
	}
	for i := range scanner.Prints {
		id := pid.ID(i)
		if pr := photoRun(t, store, id); pr.Status != wf.StatusCompleted {
			t.Errorf("photo %s: %s %s", id, pr.Status, pr.Error)
		}
		arc := filepath.Join(root, "bags")
		if !bag.Complete(arc, id) {
			t.Fatalf("photo %s not archived", id)
		}
		manifest, _ := os.ReadFile(filepath.Join(arc, id.String(), "manifest-sha256.txt"))
		if !strings.Contains(string(manifest), "data/back.tif") || !strings.Contains(string(manifest), "data/front.tif") {
			t.Errorf("photo %s manifest:\n%s", id, manifest)
		}
		if _, err := os.Stat(filepath.Join(b.path("masters"), id.String()+".back.tif")); err == nil {
			t.Errorf("photo %s back master left in the load after archiving", id)
		}
	}
	if done, _ := b.done(context.Background(), store); !done {
		t.Error("load not done once its photos are archived")
	}
}

// A back the preview cannot find is taken to lie where its front did,
// mirrored, and flagged for review.
func TestABackNotFoundTakesItsFrontsPlace(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	scanner.Backs = []batchtest.Print{} // blank backs on a white bed
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	run(t, b, store, scanner)
	b.Turned()
	run(t, b, store, scanner)
	var photo Photo
	if err := readYAML(filepath.Join(b.path("masters"), "0.yaml"), &photo); err != nil {
		t.Fatal(err)
	}
	front, back := photo.Front, photo.Back
	if back == nil || !back.Review || !strings.Contains(back.Warning, "no print") {
		t.Fatalf("back %+v", back)
	}
	if back.Print.W != front.Print.W || back.Print.H != front.Print.H || math.Abs(back.Print.CX-(2*front.Region.CX-front.Print.CX)) > 0.01 {
		t.Errorf("back print %+v, want the front's %+v mirrored in its region %+v", back.Print, front.Print, front.Region)
	}
}

// The backs can be scanned again, after the prints are turned once more;
// going back to the fronts clears the backs with them.
func TestRescanTheBacksOrTheFronts(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	run(t, b, store, scanner)
	b.Turned()
	run(t, b, store, scanner)

	redo(t, b, store, "backs", 0)
	if _, err := os.Stat(b.path("masters/0.back.tif")); err == nil {
		t.Error("back master survives a redo of the backs")
	}
	if _, err := os.Stat(filepath.Join(b.Renditions(0), "back")); err == nil {
		t.Error("back renditions survive a redo of the backs")
	}
	if _, err := rendition.Load(b.Renditions(0)); err != nil {
		t.Error("front renditions cleared by a redo of the backs")
	}
	if rec := run(t, b, store, scanner); runnertest.Steps(t, store, rec.ID)["backs"] != wf.StatusWaiting {
		t.Fatal("not waiting for the prints to be turned again")
	}
	if pr := photoRun(t, store, 0); runnertest.Steps(t, store, pr.ID)["awaiting back"] != wf.StatusWaiting {
		t.Error("photo not waiting for its back again")
	}
	b.Turned()
	run(t, b, store, scanner)
	if scanner.Details != 3 {
		t.Errorf("%d detail passes, want 3", scanner.Details)
	}

	redo(t, b, store, "fronts", 1200)
	for _, f := range []string{"masters", "turned", "backs.png"} {
		if _, err := os.Stat(b.path(f)); err == nil {
			t.Errorf("%s survives a redo of the fronts", f)
		}
	}
	if _, err := os.Stat(b.Renditions(0)); err == nil {
		t.Error("renditions survive a redo of the fronts")
	}
	if rec := run(t, b, store, scanner); runnertest.Steps(t, store, rec.ID)["backs"] != wf.StatusWaiting || scanner.LastPPI != 1200 || scanner.Details != 4 {
		t.Fatalf("after the fronts again: %v, %d passes, last at %d", runnertest.Steps(t, store, rec.ID), scanner.Details, scanner.LastPPI)
	}
	if pr := photoRun(t, store, 0); runnertest.Steps(t, store, pr.ID)["develop front"] != wf.StatusCompleted || pr.Status != wf.StatusWaiting {
		t.Errorf("photo after the fronts again: %+v %v", pr, runnertest.Steps(t, store, pr.ID))
	}
	b.Turned()
	run(t, b, store, scanner)
	described(t, b)
	b.Accept()
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("%s %s", rec.Status, rec.Error)
	}
	err := b.Redo(context.Background(), store, "fronts", 0)
	if err == nil || !strings.Contains(err.Error(), "final") {
		t.Errorf("redo once archived: %v, want a refusal", err)
	}
	// finding the prints again orphans the photos: their runs are cancelled
	b2 := describe(t, root)
	run(t, b2, store, scanner)
	b2.Confirm()
	run(t, b2, store, scanner)
	_, ids, _ := b2.Found()
	redo(t, b2, store, "preview", 0)
	if pr := photoRun(t, store, ids[0]); pr.Status != wf.StatusCancelled {
		t.Errorf("orphaned photo run: %s, want cancelled", pr.Status)
	}
}

// A load of prints with nothing on their backs skips the backs: the
// photos go on without one, and the bag has no back.tif. The backs can
// still be scanned after all.
func TestBacksCanBeSkipped(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	b := describe(t, root)
	if err := b.NoBacks(); err == nil {
		t.Error("backs skipped before the fronts were scanned")
	}
	run(t, b, store, scanner)
	b.Confirm()
	run(t, b, store, scanner)
	if err := b.NoBacks(); err != nil {
		t.Fatal(err)
	}
	rec := run(t, b, store, scanner)
	if steps := runnertest.Steps(t, store, rec.ID); steps["check quality"] != wf.StatusWaiting || steps["backs"] != wf.StatusCompleted || scanner.Details != 1 || scanner.Previews != 1 {
		t.Fatalf("after skipping the backs: %v, %d passes, %d previews", steps, scanner.Details, scanner.Previews)
	}
	if _, err := os.Stat(b.path("backs.png")); err == nil {
		t.Error("a backs preview without a backs pass")
	}
	var photo Photo
	readYAML(filepath.Join(b.path("masters"), "0.yaml"), &photo)
	if photo.Back != nil {
		t.Errorf("photo has a back: %+v", photo.Back)
	}
	if pr := photoRun(t, store, 0); runnertest.Steps(t, store, pr.ID)["accepted"] != wf.StatusWaiting {
		t.Errorf("photo not waiting to be accepted: %v", runnertest.Steps(t, store, pr.ID))
	}
	// the backs can be scanned after all
	redo(t, b, store, "backs", 0)
	if rec := run(t, b, store, scanner); runnertest.Steps(t, store, rec.ID)["backs"] != wf.StatusWaiting {
		t.Fatal("not waiting for the prints to be turned after a redo of the backs")
	}
	b.NoBacks()
	run(t, b, store, scanner)
	described(t, b)
	b.Accept()
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("%s %s", rec.Status, rec.Error)
	}
	if pr := photoRun(t, store, 0); pr.Status != wf.StatusCompleted {
		t.Errorf("photo: %s %s", pr.Status, pr.Error)
	}
	manifest, _ := os.ReadFile(filepath.Join(root, "bags", "0", "manifest-sha256.txt"))
	if strings.Contains(string(manifest), "back.tif") || !strings.Contains(string(manifest), "data/front.tif") {
		t.Errorf("manifest:\n%s", manifest)
	}
	if _, err := os.Stat(b.path("masters/0.yaml")); err == nil {
		t.Error("photo's record left in the load after archiving")
	}
}
