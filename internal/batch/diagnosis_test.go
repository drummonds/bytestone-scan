package batch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// #80: Keep for diagnosis copies what the scan has, with the operator's
// note, where a purge and a batch delete leave it: the work files, the
// photo records, the annotated previews as rendered, and the runs with
// their steps; the master TIFFs only when asked, else named in the
// manifest. Nothing of the scan's own is touched.
func TestKeepForDiagnosis(t *testing.T) {
	root, b, store := prepared(t, &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}})
	ctx := context.Background()
	now := time.Date(2026, 10, 25, 16, 30, 0, 0, time.UTC)
	before, _ := filepath.Glob(filepath.Join(b.Dir(), "*"))

	d, err := b.Keep(ctx, store, KeepOptions{Note: "the right print was cut", State: "check quality", Version: "v0.16.0", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "20261020-1-1-20261025T163000Z" || d.Scan != b.Key() || d.Note != "the right print was cut" || d.State != "check quality" || d.Version != "v0.16.0" || d.Masters {
		t.Errorf("kept %+v", d)
	}
	dir := filepath.Join(root, "diagnosis", d.Name)
	for _, f := range []string{"manifest.yaml", "scan.yaml", "prints.yaml", "preview.png", "backs.png", "turned", "annotated.png", "annotated-backs.png", "runs.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s not kept: %v", f, err)
		}
	}
	if yamls, _ := filepath.Glob(filepath.Join(dir, "masters", "*.yaml")); len(yamls) != 2 {
		t.Errorf("photo records kept: %v, want 2", yamls)
	}
	if tifs, _ := filepath.Glob(filepath.Join(dir, "masters", "*.tif")); len(tifs) != 0 {
		t.Errorf("masters kept without being asked for: %v", tifs)
	}
	manifest, _ := os.ReadFile(filepath.Join(dir, "manifest.yaml"))
	if s := string(manifest); !strings.Contains(s, "note: the right print was cut") || !strings.Contains(s, "masters/0.tif") || !strings.Contains(s, "kept: false") {
		t.Errorf("manifest:\n%s", s)
	}
	runs, _ := os.ReadFile(filepath.Join(dir, "runs.yaml"))
	if s := string(runs); !strings.Contains(s, string(ScanWorkflow)) || !strings.Contains(s, string(PhotoWorkflow)) || !strings.Contains(s, "check quality") {
		t.Errorf("runs.yaml:\n%s", s)
	}
	if after, _ := filepath.Glob(filepath.Join(b.Dir(), "*")); strings.Join(after, " ") != strings.Join(before, " ") {
		t.Errorf("the scan's own files changed: %v, was %v", after, before)
	}

	list, err := ListDiagnoses(root)
	if err != nil || len(list) != 1 || list[0].Name != d.Name || list[0].Note != d.Note || list[0].Size <= 0 {
		t.Fatalf("listed %+v, %v", list, err)
	}

	// kept again, with the masters: a second directory, newest first
	d2, err := b.Keep(ctx, store, KeepOptions{Note: "with masters", Masters: true, Now: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if tifs, _ := filepath.Glob(filepath.Join(root, "diagnosis", d2.Name, "masters", "*.tif")); len(tifs) == 0 {
		t.Error("masters asked for, none kept")
	}
	list, _ = ListDiagnoses(root)
	if len(list) != 2 || list[0].Name != d2.Name || list[0].Size <= list[1].Size {
		t.Errorf("listed %+v", list)
	}

	if err := DiscardDiagnosis(root, "../batches"); err == nil {
		t.Error("discarded a path outside diagnosis/")
	}
	if err := DiscardDiagnosis(root, d.Name); err != nil {
		t.Fatal(err)
	}
	if list, _ = ListDiagnoses(root); len(list) != 1 || list[0].Name != d2.Name {
		t.Errorf("after discarding: %+v", list)
	}
}

// Kept in the same second twice: the second gets a name of its own.
func TestKeepNeverOverwrites(t *testing.T) {
	root, b, store := prepared(t, &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}})
	now := time.Date(2026, 10, 25, 16, 30, 0, 0, time.UTC)
	d1, _ := b.Keep(context.Background(), store, KeepOptions{Now: now})
	d2, err := b.Keep(context.Background(), store, KeepOptions{Now: now})
	if err != nil || d1.Name == d2.Name {
		t.Errorf("%s then %s, %v", d1.Name, d2.Name, err)
	}
	if list, _ := ListDiagnoses(root); len(list) != 2 {
		t.Errorf("listed %d", len(list))
	}
}
