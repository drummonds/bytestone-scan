package batch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Story 041: each photo's id comes from the archive service, homed with
// the batch's person; once bagged the photo is deposited there and its
// receipt taken, checksum for checksum. A bag the archive refuses as a
// duplicate ends the photo, rejected; a service that is down fails the
// step until it is tried again.
func TestEachPhotoIsDepositedAndReceipted(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	svc := service(root)
	svc.Next = 40 // the collection has items already
	scanner := twoSided()
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	scanned(t, b, store, scanner)
	_, ids, _ := b.Found()
	if len(ids) != 2 || ids[0] != 40 || ids[1] != 41 {
		t.Fatalf("ids %v, want 40 and 41 from the archive", ids)
	}
	if rec, err := svc.Receipt(40); err != nil || rec.Home != 10 {
		t.Errorf("item 40 at the archive: %+v %v, want reserved for the batch's home person", rec, err)
	}
	if d, _ := DepositOf(root, 40); d != nil {
		t.Errorf("deposited before being bagged: %+v", d)
	}

	svc.RejectAs[41] = 7
	described(t, b)
	b.Accept()
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("load: %s %s", rec.Status, rec.Error)
	}
	for _, id := range ids {
		if pr := photoRun(t, store, id); pr.Status != wf.StatusCompleted {
			t.Errorf("photo %s: %s %s", id, pr.Status, pr.Error)
		}
	}
	held, err := DepositOf(root, 40)
	if err != nil || held == nil || held.Status != "held" || held.Home != 10 || held.At.IsZero() {
		t.Fatalf("deposit of 40: %+v %v", held, err)
	}
	manifest, err := bag.Manifest(filepath.Join(root, "bags", ids[0].String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 3 || len(held.Payload) != 3 || held.Payload["data/front.tif"] != manifest["data/front.tif"] {
		t.Errorf("receipt payload %v, bag manifest %v", held.Payload, manifest)
	}
	if rec, _ := svc.Receipt(40); rec.Status != "held" {
		t.Errorf("the archive does not hold 40: %+v", rec)
	}
	rejected, _ := DepositOf(root, 41)
	if rejected == nil || rejected.Status != "duplicate" || rejected.DuplicateOf != 7 || rejected.File == "" {
		t.Errorf("deposit of 41: %+v, want refused as a duplicate of 7", rejected)
	}
	if rec, _ := svc.Receipt(41); rec.Status != "reserved" {
		t.Errorf("a refused bag was held: %+v", rec)
	}
	if done, _ := b.done(ctx, store); !done {
		t.Error("load not done once every photo is held or rejected")
	}

	// a service that is down fails the deposit; the retry deposits
	b2 := describe(t, root)
	run(t, b2, store, scanner)
	b2.Confirm()
	scanned(t, b2, store, scanner)
	described(t, b2)
	b2.Accept()
	svc.Down = true
	if _, err := b2.Run(ctx, Env{Store: store, Scanner: scanner, Archive: svc, Now: clock}, nil); err == nil || !strings.Contains(err.Error(), "not answering") {
		t.Fatalf("service down: %v", err)
	}
	_, ids2, _ := b2.Found()
	if pr := photoRun(t, store, ids2[0]); pr.Status != wf.StatusFailed || runnertest.Steps(t, store, pr.ID)["package"] != wf.StatusCompleted {
		t.Errorf("photo with the service down: %s %v", pr.Status, runnertest.Steps(t, store, pr.ID))
	}
	svc.Down = false
	run(t, b2, store, scanner)
	if d, _ := DepositOf(root, ids2[0]); d == nil || d.Status != "held" {
		t.Errorf("after the retry: %+v", d)
	}
	// depositing again is harmless: the same bag twice is one item
	if err := b2.deposit(ids2[0], Env{Store: store, Scanner: scanner, Archive: svc, Now: clock}); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(root, "bags", ids2[0].String()+".deposit.yaml")); err != nil {
		t.Error(err)
	}
	_ = pid.ID(0)
}
