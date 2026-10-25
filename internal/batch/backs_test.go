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
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
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
// and the scan scans the backs where the fronts lay, at 300 ppi and 8 bits,
// reviewing a back that differs from its front. Each photo is a workflow of
// its own under the scan, developing each side as it comes and archiving
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
	if steps := runnertest.Steps(t, store, rec.ID); rec.Status != wf.StatusWaiting || steps["backs preview"] != wf.StatusWaiting || steps["fronts"] != wf.StatusCompleted || steps["photos"] != wf.StatusCompleted {
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
	run(t, b, store, scanner)
	if err := b.ConfirmBacks(); err != nil {
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
			t.Errorf("photo %s back master left in the scan after archiving", id)
		}
	}
	if done, _ := b.done(context.Background(), store); !done {
		t.Error("scan not done once its photos are archived")
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
	backs(t, b, store, scanner)
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
	backs(t, b, store, scanner)

	redo(t, b, store, "backs preview", 0)
	if _, err := os.Stat(b.path("masters/0.back.tif")); err == nil {
		t.Error("back master survives a redo of the backs")
	}
	if _, err := os.Stat(filepath.Join(b.Renditions(0), "back")); err == nil {
		t.Error("back renditions survive a redo of the backs")
	}
	if _, err := rendition.Load(b.Renditions(0)); err != nil {
		t.Error("front renditions cleared by a redo of the backs")
	}
	if rec := run(t, b, store, scanner); runnertest.Steps(t, store, rec.ID)["backs preview"] != wf.StatusWaiting {
		t.Fatal("not waiting for the prints to be turned again")
	}
	if pr := photoRun(t, store, 0); runnertest.Steps(t, store, pr.ID)["awaiting back"] != wf.StatusWaiting {
		t.Error("photo not waiting for its back again")
	}
	backs(t, b, store, scanner)
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
	if rec := run(t, b, store, scanner); runnertest.Steps(t, store, rec.ID)["backs preview"] != wf.StatusWaiting || scanner.LastPPI != 1200 || scanner.Details != 4 {
		t.Fatalf("after the fronts again: %v, %d passes, last at %d", runnertest.Steps(t, store, rec.ID), scanner.Details, scanner.LastPPI)
	}
	if pr := photoRun(t, store, 0); runnertest.Steps(t, store, pr.ID)["develop front"] != wf.StatusCompleted || pr.Status != wf.StatusWaiting {
		t.Errorf("photo after the fronts again: %+v %v", pr, runnertest.Steps(t, store, pr.ID))
	}
	backs(t, b, store, scanner)
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

// A scan of prints with nothing on their backs skips the backs: the
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
	redo(t, b, store, "backs preview", 0)
	if rec := run(t, b, store, scanner); runnertest.Steps(t, store, rec.ID)["backs preview"] != wf.StatusWaiting {
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
		t.Error("photo's record left in the scan after archiving")
	}
}

// #77: the backs pass waits twice, like the fronts. Once the prints are
// turned the bed is previewed and each back found within its front's
// region, and the scan waits for the operator to confirm before the
// detail pass; the backs found, and what is doubtful about them, can be
// read meanwhile.
func TestTheBacksAreConfirmedOnTheirPreview(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	scanner.Backs = []batchtest.Print{{X0: 22, Y0: 20, X1: 62, Y1: 50, Value: 50000}} // the second back is not found
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	run(t, b, store, scanner)
	if err := b.ConfirmBacks(); err == nil {
		t.Error("backs confirmed before they were previewed")
	}
	b.Turned()
	rec := run(t, b, store, scanner)
	if steps := runnertest.Steps(t, store, rec.ID); rec.Status != wf.StatusWaiting || steps["backs preview"] != wf.StatusCompleted || steps["backs"] != wf.StatusWaiting {
		t.Fatalf("after the backs preview: %s %v", rec.Status, steps)
	}
	if scanner.Previews != 2 || scanner.Details != 1 {
		t.Errorf("previews %d, details %d; want 2 and 1: the backs are not scanned until confirmed", scanner.Previews, scanner.Details)
	}
	if _, err := os.Stat(b.path("backs.png")); err != nil {
		t.Error(err)
	}
	backs, err := b.BacksFound()
	if err != nil || len(backs) != 2 || backs[0].Review || !backs[1].Review || !strings.Contains(backs[1].Warning, "no print") {
		t.Fatalf("backs found: %+v, %v", backs, err)
	}
	if _, err := os.Stat(b.path("masters/0.back.tif")); err == nil {
		t.Error("a back master before the backs were confirmed")
	}
	if err := b.Accept(); err == nil {
		t.Error("accepted before the backs were confirmed")
	}
	if err := b.ConfirmBacks(); err != nil {
		t.Fatal(err)
	}
	rec = run(t, b, store, scanner)
	if steps := runnertest.Steps(t, store, rec.ID); rec.Status != wf.StatusWaiting || steps["backs"] != wf.StatusCompleted || steps["check quality"] != wf.StatusWaiting {
		t.Fatalf("after the backs: %s %v", rec.Status, steps)
	}
	if scanner.Previews != 2 || scanner.Details != 2 {
		t.Errorf("previews %d, details %d; want 2 and 2: confirming does not preview again", scanner.Previews, scanner.Details)
	}
	var photo Photo
	readYAML(filepath.Join(b.path("masters"), "1.yaml"), &photo)
	if photo.Back == nil || !photo.Back.Review {
		t.Errorf("the doubtful back's record: %+v", photo.Back)
	}
}

// #77: a print moved while being turned is put back and the backs
// previewed again: the preview and the backs found are cleared, the
// prints stay turned, and nothing is scanned.
func TestPreviewTheBacksAgain(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	run(t, b, store, scanner)
	b.Turned()
	run(t, b, store, scanner) // previewed, waiting to be confirmed
	if err := b.PreviewBacksAgain(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"backs.png", "backs.yaml"} {
		if _, err := os.Stat(b.path(f)); err == nil {
			t.Errorf("%s survives a second preview", f)
		}
	}
	rec := run(t, b, store, scanner)
	if steps := runnertest.Steps(t, store, rec.ID); steps["backs"] != wf.StatusWaiting || scanner.Previews != 3 || scanner.Details != 1 {
		t.Fatalf("after previewing again: %v, previews %d, details %d", steps, scanner.Previews, scanner.Details)
	}
	if _, err := b.BacksFound(); err != nil {
		t.Error(err)
	}
}

// backs turns the prints over, previews them, confirms the backs found
// and scans them.
func backs(t *testing.T, b *Scan, store wf.Store, scanner Scanner) {
	t.Helper()
	if err := b.Turned(); err != nil {
		t.Fatal(err)
	}
	run(t, b, store, scanner)
	if err := b.ConfirmBacks(); err != nil {
		t.Fatal(err)
	}
	run(t, b, store, scanner)
}

// #78: one print's back can be blank. Declared at the backs preview, that
// photo goes on without a back while the others' backs are scanned;
// declared at the quality check, a back already scanned is discarded with
// its renditions, since a back the operator says should not exist is
// withdrawn before the photo is bagged. The record says blank rather than
// not scanned.
func TestABlankBackIsDeclaredPerPrint(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	b := describe(t, root)
	run(t, b, store, scanner)
	if err := b.SetBackBlank(0, true); err == nil {
		t.Error("a back declared blank before the print was scanned")
	}
	b.Confirm()
	run(t, b, store, scanner)
	b.Turned()
	run(t, b, store, scanner) // the backs preview, waiting to be confirmed
	if err := b.SetBackBlank(1, true); err != nil {
		t.Fatal(err)
	}
	if err := b.SetBackBlank(1, false); err != nil {
		t.Fatal("not yet scanned, the declaration can be withdrawn:", err)
	}
	if err := b.SetBackBlank(1, true); err != nil {
		t.Fatal(err)
	}
	b.ConfirmBacks()
	run(t, b, store, scanner)
	var p0, p1 Photo
	readYAML(filepath.Join(b.path("masters"), "0.yaml"), &p0)
	readYAML(filepath.Join(b.path("masters"), "1.yaml"), &p1)
	if p0.Back == nil || p0.BackBlank {
		t.Errorf("photo 0: back %+v, blank %v", p0.Back, p0.BackBlank)
	}
	if p1.Back != nil || !p1.BackBlank {
		t.Errorf("photo 1: back %+v, blank %v; want no back, blank", p1.Back, p1.BackBlank)
	}
	if _, err := os.Stat(b.path("masters/1.back.tif")); err == nil {
		t.Error("a back master for the blank back")
	}
	if pr := photoRun(t, store, 1); runnertest.Steps(t, store, pr.ID)["accepted"] != wf.StatusWaiting {
		t.Errorf("the blank-backed photo: %v, want waiting to be accepted", runnertest.Steps(t, store, pr.ID))
	}
	if err := b.SetBackBlank(1, false); err == nil {
		t.Error("the backs scanned, the declaration withdrawn without a rescan")
	}

	// at the quality check: a back already scanned is discarded
	if err := b.SetBackBlank(0, true); err != nil {
		t.Fatal(err)
	}
	var again Photo // a fresh record: a key left out leaves a reused one as it was
	readYAML(filepath.Join(b.path("masters"), "0.yaml"), &again)
	if again.Back != nil || !again.BackBlank {
		t.Errorf("photo 0 after: back %+v, blank %v", again.Back, again.BackBlank)
	}
	if _, err := os.Stat(b.path("masters/0.back.tif")); err == nil {
		t.Error("the discarded back master is still there")
	}
	if _, err := os.Stat(filepath.Join(b.Renditions(0), "back")); err == nil {
		t.Error("the discarded back's renditions are still there")
	}
	if _, err := rendition.Load(b.Renditions(0)); err != nil {
		t.Error("the front's renditions went with the back:", err)
	}
	if err := b.DescribePhoto(0, PhotoDescription{Back: "Mum"}); err == nil {
		t.Error("the back's text taken for a blank back")
	}
	described(t, b)
	b.Accept()
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("%s %s", rec.Status, rec.Error)
	}
	for _, id := range []string{"0", "1"} {
		if _, err := os.Stat(filepath.Join(root, "bags", id, "data", "back.tif")); err == nil {
			t.Errorf("bag %s has a back.tif", id)
		}
		raw, _ := os.ReadFile(filepath.Join(root, "bags", id, "data", "photo.yaml"))
		if !strings.Contains(string(raw), "back_blank: true") {
			t.Errorf("bag %s's record does not say the back was blank:\n%s", id, raw)
		}
	}
	if err := b.SetBackBlank(0, false); err == nil {
		t.Error("an archived photo's record changed")
	}
}

// A back is checked against the front as the operator left it: when the
// finder took a band of bed for print and the region was brought in to
// the print, the back is the size of that print, not of the finder's.
func TestABackIsCheckedAgainstTheAdjustedFront(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := &batchtest.Scanner{
		Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}},
		Backs:  []batchtest.Print{{X0: 20, Y0: 20, X1: 50, Y1: 50, Value: 50000}},
	}
	b := describe(t, root)
	run(t, b, store, scanner)
	if err := b.Adjust(0, find.Right, -10); err != nil {
		t.Fatal(err)
	}
	b.Confirm()
	run(t, b, store, scanner)
	backs(t, b, store, scanner)
	var photo Photo
	if err := readYAML(filepath.Join(b.path("masters"), "0.yaml"), &photo); err != nil {
		t.Fatal(err)
	}
	if math.Abs(photo.Front.Print.W-30) > 0.5 {
		t.Errorf("front print %.1f mm wide, want 30: the region's 30.5 less the margin", photo.Front.Print.W)
	}
	if back := photo.Back; back == nil || back.Review || math.Abs(back.Print.W-30) > 1 {
		t.Errorf("back %+v, want a 30 mm back found and not for review", back)
	}
}
