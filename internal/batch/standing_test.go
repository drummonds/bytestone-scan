package batch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// #132: a scan's standing in its batch is its own run rolled up with its
// photos', so that a scan whose run is through but whose photo failed at
// deposit stands as failed, naming the photo and the step, and the batch's
// finish refusal says the same.
func TestAScanStandsWhereItsPhotosDo(t *testing.T) {
	ctx := context.Background()
	root, store := t.TempDir(), wf.NewMemStore()
	svc := service(root)
	scanner := twoSided()
	b := describe(t, root)
	if s, err := b.Standing(ctx, store); err != nil || s.String() != "not started" {
		t.Errorf("before the run: %+v, %v", s, err)
	}
	run(t, b, store, scanner)
	b.Confirm()
	scanned(t, b, store, scanner)
	if s, _ := b.Standing(ctx, store); s.Status != wf.StatusWaiting || s.Step != "check quality" || s.String() != "waiting at check quality" {
		t.Errorf("at the quality check: %+v %q", s, s)
	}
	described(t, b)
	b.Accept()
	_, ids, _ := b.Found()
	stale := ids[0]
	recipe := filepath.Join(b.Renditions(stale), "recipe.yaml")
	if err := os.WriteFile(recipe, []byte("version: 1\nmaster: 0000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := Env{Store: store, Scanner: scanner, Archive: svc, Now: clock}
	if _, err := b.Run(ctx, env, nil); err == nil {
		t.Fatal("the stale recipe was deposited")
	}
	s, err := b.Standing(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	want := "photo " + stale.String() + " failed at deposit: "
	if s.Status != wf.StatusFailed || s.Step != "deposit" || s.Photo == nil || *s.Photo != stale || s.Through() || !strings.HasPrefix(s.String(), want) || !strings.Contains(s.String(), "renditions") {
		t.Errorf("a photo refused: %+v %q, want %q…", s, s, want)
	}
	sitting, err := Open(root, b.Batch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sitting.Finish(ctx, store); err == nil || !strings.Contains(err.Error(), "scan 1 is not through yet: "+want) {
		t.Errorf("finish: %v, want the photo named", err)
	}
	if _, err := b.Run(ctx, env, nil); err != nil {
		t.Fatalf("redeveloped, deposited again: %v", err)
	}
	if s, _ := b.Standing(ctx, store); !s.Through() || s.String() != "through" {
		t.Errorf("held: %+v %q", s, s)
	}
}
