package batch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// Story 165: an id the archive gives out is refused when staging holds
// something under it already, whichever of the three it is; the print is
// given no id, so nothing is scanned, bagged or deposited under it, and
// with the files gone the next id is taken and the scan goes on.
func TestAnIDStagingAlreadyHoldsIsRefused(t *testing.T) {
	for _, held := range []string{"bags/0", "bags/0.deposit.yaml", "renditions/0"} {
		t.Run(held, func(t *testing.T) {
			root := t.TempDir()
			scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
			store := wf.NewMemStore()
			b := describe(t, root)
			run(t, b, store, scanner)
			// left by an earlier deposit, to an archive that has since given the id out again
			path := filepath.Join(root, filepath.FromSlash(held))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("status: held\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := b.Confirm(); err != nil {
				t.Fatal(err)
			}

			_, err := b.Run(context.Background(), Env{Store: store, Scanner: scanner, Archive: service(root), Now: clock}, nil)
			if err == nil || !strings.Contains(err.Error(), "item 0") || !strings.Contains(err.Error(), held) {
				t.Fatalf("confirm with %s in staging: error %v, want one naming item 0 and the file", held, err)
			}
			if _, ids, _ := b.Found(); len(ids) != 0 {
				t.Errorf("the print was given %v, want no id", ids)
			}
			if scanner.Details != 0 {
				t.Error("the detail pass ran under an id staging holds")
			}
			if raw, _ := os.ReadFile(path); string(raw) != "status: held\n" {
				t.Errorf("%s was changed: %q", held, raw)
			}

			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			scanned(t, b, store, scanner)
			if _, ids, _ := b.Found(); len(ids) != 1 || ids[0] != pid.ID(1) {
				t.Errorf("after the file is moved away the print has %v, want the next id, 1", ids)
			}
		})
	}
}

// Story 165: a letter's id is refused the same way.
func TestALetterIDStagingAlreadyHoldsIsRefused(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	l, err := sitting(t, root).NewFeed("", now)
	if err != nil {
		t.Fatal(err)
	}
	f := &batchtest.Feeder{Sides: []uint8{40, 255}}
	runFeed(t, l, store, f)
	if err := os.MkdirAll(filepath.Join(root, "bags", "0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := l.Arrange([]SheetChoice{{Letter: 1, Role: "sheet"}}); err != nil {
		t.Fatal(err)
	}
	_, err = l.Run(t.Context(), Env{Store: store, Feeder: f, Archive: service(root), Now: clock}, nil)
	if err == nil || !strings.Contains(err.Error(), "item 0") || !strings.Contains(err.Error(), "bags/0") {
		t.Fatalf("arrange with bags/0 in staging: error %v, want one naming item 0 and the bag", err)
	}
	if letters, _ := l.Letters(); len(letters) != 1 || letters[0].ID != nil {
		t.Errorf("letters %+v, want one with no id", letters)
	}
}
