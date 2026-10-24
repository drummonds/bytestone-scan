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
)

// Story 040: before a photo is archived the operator describes it: the
// date taken, the photographer and what the back says, each a claim with
// its source, transcribed by a person. A photo not yet described waits
// after its scan is accepted; the bag carries the claims; an archived
// photo takes no more.
func TestAPhotoIsDescribedBeforeItIsArchived(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	scanner := twoSided()
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	scanned(t, b, store, scanner)

	for _, bad := range []PhotoDescription{{Date: "July 1952"}, {Date: "1952-7"}, {Date: "52"}, {Date: "1952-07-32"}, {Source: "guess"}} {
		if err := b.DescribePhoto(0, bad); err == nil {
			t.Errorf("described as %+v", bad)
		}
	}
	said := PhotoDescription{Date: "1952-07", Approximate: true, Photographer: "Jerome Ltd, Glasgow", Back: "Mum & Dad, Rothesay, summer 1952", Source: "back"}
	if err := b.DescribePhoto(0, said); err != nil {
		t.Fatal(err)
	}
	var photo Photo
	if err := readYAML(b.masterPath(0, "yaml"), &photo); err != nil {
		t.Fatal(err)
	}
	want := []Claim{
		{Kind: "date taken", Value: "1952-07", Approximate: true, Source: "back", Read: "transcribed"},
		{Kind: "photographer", Value: "Jerome Ltd, Glasgow", Source: "back", Read: "transcribed"},
		{Kind: "back", Value: "Mum & Dad, Rothesay, summer 1952", Source: "back", Read: "transcribed"},
	}
	if photo.Described == nil || len(photo.Claims) != len(want) {
		t.Fatalf("photo after describing: %+v", photo)
	}
	for i, c := range want {
		if photo.Claims[i] != c {
			t.Errorf("claim %d %+v, want %+v", i, photo.Claims[i], c)
		}
	}

	b.Accept()
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("scan: %s %s", rec.Status, rec.Error)
	}
	arc := filepath.Join(root, "bags")
	if !bag.Complete(arc, 0) || bag.Complete(arc, 1) {
		t.Fatal("the described photo should be archived, the other not")
	}
	if pr := photoRun(t, store, 1); runnertest.Steps(t, store, pr.ID)["describe"] != wf.StatusWaiting {
		t.Errorf("photo 1: %v, want waiting to be described", runnertest.Steps(t, store, pr.ID))
	}
	if done, _ := b.done(ctx, store); done {
		t.Error("scan done with a photo still to describe")
	}
	raw, _ := os.ReadFile(filepath.Join(arc, "0", "data", "photo.yaml"))
	for _, s := range []string{"kind: date taken", "value: 1952-07", "approximate: true", "read: transcribed", "Jerome Ltd"} {
		if !strings.Contains(string(raw), s) {
			t.Errorf("archived photo.yaml lacks %q:\n%s", s, raw)
		}
	}

	// nothing known is still an answer
	if err := b.DescribePhoto(1, PhotoDescription{}); err != nil {
		t.Fatal(err)
	}
	run(t, b, store, scanner)
	if !bag.Complete(arc, 1) {
		t.Error("the skipped photo was not archived")
	}
	raw, _ = os.ReadFile(filepath.Join(arc, "1", "data", "photo.yaml"))
	if strings.Contains(string(raw), "claims") || !strings.Contains(string(raw), "described:") {
		t.Errorf("skipped photo.yaml:\n%s", raw)
	}
	if done, _ := b.done(ctx, store); !done {
		t.Error("scan not done once every photo is archived")
	}
	if err := b.DescribePhoto(0, said); err == nil || !strings.Contains(err.Error(), "archived") {
		t.Errorf("described an archived photo: %v", err)
	}
	// the description can be changed until then, and the back's text goes with a back
	b2 := describe(t, root)
	run(t, b2, store, scanner)
	b2.Confirm()
	run(t, b2, store, scanner)
	b2.NoBacks()
	run(t, b2, store, scanner)
	_, ids, _ := b2.Found()
	if err := b2.DescribePhoto(ids[0], PhotoDescription{Back: "x"}); err == nil {
		t.Error("the back's text taken for a photo whose back was not scanned")
	}
	if err := b2.DescribePhoto(ids[0], PhotoDescription{Photographer: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := b2.DescribePhoto(ids[0], PhotoDescription{Photographer: "B", Source: "family"}); err != nil {
		t.Fatal(err)
	}
	readYAML(b2.masterPath(ids[0], "yaml"), &photo)
	if len(photo.Claims) != 1 || photo.Claims[0].Value != "B" || photo.Claims[0].Source != "family" {
		t.Errorf("claims after describing again: %+v", photo.Claims)
	}
}
