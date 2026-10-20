package batch

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/runnertest"
	"golang.org/x/image/tiff"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
)

var reg = register.Register{Dir: "testdata/people"}

var now = time.Date(2026, 10, 20, 15, 4, 5, 0, time.UTC)

func clock() time.Time { return now }

func describe(t *testing.T, root string) *Batch {
	t.Helper()
	b, err := Describe(root, reg, Description{Text: "SS Bonita and crew", Person: 10, ScanPPI: 600}, now)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func run(t *testing.T, b *Batch, store wf.Store, s Scanner) *wf.RunRecord {
	t.Helper()
	rec, err := b.Run(context.Background(), wf.NewRunner(store, nil), s, clock)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return rec
}

// Story 001, "Describe".
func TestDescribe(t *testing.T) {
	cases := []struct {
		name string
		d    Description
		want string // part of the error; empty for success
	}{
		{"recorded", Description{"SS Bonita", 10, 2400}, ""},
		{"no description", Description{"  ", 10, 2400}, "description"},
		{"not in the register", Description{"SS Bonita", 12, 2400}, "not in the register"},
		{"other scan ppi", Description{"SS Bonita", 10, 300}, "600, 1200 or 2400"},
	}
	for _, tc := range cases {
		root := t.TempDir()
		b, err := Describe(root, reg, tc.d, now)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: %v", tc.name, err)
				continue
			}
			again, err := Open(root, b.ID)
			if err != nil || again.Description != "SS Bonita" || again.Person != 10 || again.PersonName != "Ada Example" || again.ScanPPI != 2400 {
				t.Errorf("%s: reopened as %+v, %v", tc.name, again, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want one mentioning %q", tc.name, err, tc.want)
		}
	}
}

func TestBatchIDsAreTheDateAndASequence(t *testing.T) {
	root := t.TempDir()
	a, b := describe(t, root), describe(t, root)
	if a.ID != "20261020-1" || b.ID != "20261020-2" {
		t.Errorf("ids %s, %s, want 20261020-1, 20261020-2", a.ID, b.ID)
	}
}

func TestBatchRunsToBags(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, // 40 × 30 mm
		{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000},
	}}
	store := wf.NewMemStore()
	b := describe(t, root)

	rec := run(t, b, store, scanner)
	if rec.Status != wf.StatusWaiting {
		t.Fatalf("first tick: %s, want waiting at confirm", rec.Status)
	}
	for _, f := range []string{"preview.png", "prints.png", "prints.yaml"} {
		if _, err := os.Stat(filepath.Join(b.Dir(), f)); err != nil {
			t.Errorf("after the first tick: %v", err)
		}
	}
	if scanner.Details != 0 {
		t.Error("detail pass ran before confirm")
	}

	if err := b.Confirm(); err != nil {
		t.Fatal(err)
	}
	if rec = run(t, b, store, scanner); rec.Status != wf.StatusWaiting {
		t.Fatalf("after confirm: %s, want waiting for the quality check", rec.Status)
	}
	if err := b.Accept(); err != nil {
		t.Fatal(err)
	}
	rec = run(t, b, store, scanner)
	if rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s, want completed", rec.Status)
	}
	steps := runnertest.Steps(t, store, runnertest.OnlyRun(t, store).ID)
	for _, name := range []string{"describe", "preview", "find prints", "confirm", "detail pass", "check quality", "package"} {
		if steps[name] != wf.StatusCompleted {
			t.Errorf("step %s: %s", name, steps[name])
		}
	}

	bags := filepath.Join(root, "bags")
	for i, p := range scanner.Prints {
		id := pid.ID(i)
		if !bag.Complete(bags, id) {
			t.Fatalf("bag %s not complete", id)
		}
		f, err := os.Open(filepath.Join(bags, id.String(), "data", "front.tif"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := tiff.Decode(f)
		f.Close()
		if err != nil {
			t.Fatalf("bag %s front.tif: %v", id, err)
		}
		// region = print + 1.5 mm each side, at 600 ppi
		wantW := int((p.X1 - p.X0 + 3) / 25.4 * 600)
		if got := img.Bounds().Dx(); got < wantW-4 || got > wantW+4 {
			t.Errorf("bag %s master is %d px wide, want about %d", id, got, wantW)
		}
		c := img.Bounds().Size().Div(2)
		if r, _, _, _ := img.At(c.X, c.Y).RGBA(); uint16(r) != p.Value {
			t.Errorf("bag %s centre = %d, want the print's %d", id, r, p.Value)
		}

		var photo Photo
		raw, err := os.ReadFile(filepath.Join(bags, id.String(), "data", "photo.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(raw, &photo); err != nil {
			t.Fatal(err)
		}
		if photo.ID != id || photo.Batch != b.ID || photo.Description != "SS Bonita and crew" ||
			photo.Home.ID != 10 || photo.Home.Name != "Ada Example" ||
			photo.Front.ScanPPI != 600 || photo.Front.MasterPPI != 600 || photo.Front.Averaging != 1 ||
			!photo.Front.Captured.Equal(now) || photo.Front.Warning == "" {
			t.Errorf("bag %s photo.yaml = %+v", id, photo)
		}
		if math.Abs(photo.Front.Print.X0-p.X0) > 1 || math.Abs(photo.Front.Print.Y1-p.Y1) > 1 {
			t.Errorf("bag %s print at %+v, want about %v", id, photo.Front.Print, p)
		}

		tif, _ := os.ReadFile(filepath.Join(bags, id.String(), "data", "front.tif"))
		for _, want := range []string{"<dc:identifier>" + id.String() + "</dc:identifier>", "<bs:batch>" + b.ID + "</bs:batch>", "<bs:side>front</bs:side>"} {
			if !bytes.Contains(tif, []byte(want)) {
				t.Errorf("bag %s master's XMP lacks %s", id, want)
			}
		}
	}
}

func TestRetryAfterTheDetailPassFails(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}, FailDetails: 1}
	store := wf.NewMemStore()
	runner := wf.NewRunner(store, nil)
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()

	if _, err := b.Run(context.Background(), runner, scanner, clock); err == nil || !strings.Contains(err.Error(), "USB") {
		t.Fatalf("failed detail pass: error %v", err)
	}
	if bag.Complete(filepath.Join(root, "bags"), 0) {
		t.Error("bag complete after a failed detail pass")
	}
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusWaiting {
		t.Fatalf("retry: %s, want waiting for the quality check", rec.Status)
	}
	b.Accept()
	run(t, b, store, scanner)
	if scanner.Previews != 1 || scanner.Details != 2 {
		t.Errorf("previews %d, details %d; want 1 and 2", scanner.Previews, scanner.Details)
	}
	if !bag.Complete(filepath.Join(root, "bags"), 0) {
		t.Error("bag not complete after the retry")
	}
}

func TestScannerExitingBadlyFailsTheDetailPass(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}, CloseErr: errors.New("scanimage: exit status 9")}
	store := wf.NewMemStore()
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	if _, err := b.Run(context.Background(), wf.NewRunner(store, nil), scanner, clock); err == nil || !strings.Contains(err.Error(), "exit status 9") {
		t.Errorf("error %v, want the scanner's", err)
	}
	if bag.Complete(filepath.Join(root, "bags"), 0) {
		t.Error("bag made from a scan that failed")
	}
}

func TestConfirmNeedsPrints(t *testing.T) {
	root := t.TempDir()
	store := wf.NewMemStore()
	b := describe(t, root)
	if err := b.Confirm(); err == nil {
		t.Error("confirmed before the prints were found")
	}
	if rec := run(t, b, store, &batchtest.Scanner{}); rec.Status != wf.StatusWaiting {
		t.Fatalf("empty bed: %s, want waiting", rec.Status)
	}
	if err := b.Confirm(); err == nil || !strings.Contains(err.Error(), "no prints") {
		t.Errorf("confirm on an empty bed: %v", err)
	}
}

func TestEveryStepHasADefinition(t *testing.T) {
	var def *wf.Definition
	for i := range wf.Definitions {
		if wf.Definitions[i].Type == Workflow {
			def = &wf.Definitions[i]
		}
	}
	if def == nil {
		t.Fatalf("no definition for %s", Workflow)
	}
	b := &Batch{}
	stages := b.Stages(nil, clock)
	if len(stages) != len(def.Steps) {
		t.Fatalf("%d stages, %d steps defined", len(stages), len(def.Steps))
	}
	for i, st := range stages {
		if def.Steps[i].Name != st.Name || !strings.HasPrefix(def.Steps[i].Source, "https://") {
			t.Errorf("step %d: defined %+v, stage %s", i, def.Steps[i], st.Name)
		}
	}
}

func TestFindPhoto(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	store := wf.NewMemStore()
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	run(t, b, store, scanner)

	// before packaging, the master is in the batch
	photo, path, err := FindPhoto(root, 0)
	if err != nil || photo.ID != 0 || filepath.Base(path) != "0.tif" {
		t.Errorf("before packaging, FindPhoto(0) = %+v, %q, %v", photo, path, err)
	}
	b.Accept()
	run(t, b, store, scanner)

	photo, path, err = FindPhoto(root, 0)
	if err != nil || photo.ID != 0 || photo.Batch != b.ID || filepath.Base(path) != "front.tif" {
		t.Errorf("FindPhoto(0) = %+v, %q, %v", photo, path, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error(err)
	}
	if _, _, err := FindPhoto(root, 7); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("FindPhoto(7): %v, want not found", err)
	}
}

// prepared is a batch whose detail pass is done, waiting for the quality check.
func prepared(t *testing.T, scanner *batchtest.Scanner) (string, *Batch, wf.Store) {
	t.Helper()
	root := t.TempDir()
	store := wf.NewMemStore()
	b := describe(t, root)
	run(t, b, store, scanner)
	if err := b.Confirm(); err != nil {
		t.Fatal(err)
	}
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusWaiting {
		t.Fatalf("%s, want waiting for the quality check", rec.Status)
	}
	return root, b, store
}

func redo(t *testing.T, b *Batch, store wf.Store, from string, ppi int) {
	t.Helper()
	if err := b.Redo(context.Background(), wf.NewRunner(store, nil), from, ppi); err != nil {
		t.Fatalf("redo from %s: %v", from, err)
	}
}

func TestRedoFromThePreviewAfterMovingPrints(t *testing.T) {
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	root, b, store := prepared(t, scanner)

	scanner.Prints = append(scanner.Prints, batchtest.Print{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000})
	redo(t, b, store, "preview", 0)
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusWaiting {
		t.Fatalf("%s, want waiting", rec.Status)
	}
	prints, ids, _ := b.Found()
	if len(prints) != 2 || len(ids) != 0 {
		t.Errorf("after the redo: %d prints, %d ids; want 2 prints waiting to be confirmed", len(prints), len(ids))
	}
	if scanner.Previews != 2 || scanner.Details != 1 {
		t.Errorf("previews %d, details %d; want 2 and 1", scanner.Previews, scanner.Details)
	}
	if _, err := os.Stat(filepath.Join(root, "bags", "0.partial")); !os.IsNotExist(err) {
		t.Error("photo id 0's reservation was not released")
	}
	b.Confirm()
	run(t, b, store, scanner)
	if _, ids, _ = b.Found(); len(ids) != 2 || ids[0] != 0 || ids[1] != 1 {
		t.Errorf("ids %v after confirming again, want 0, 1", ids)
	}
}

func TestRedoTheDetailPassAtAnotherPPI(t *testing.T) {
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	root, b, store := prepared(t, scanner)
	redo(t, b, store, "detail pass", 1200)
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusWaiting {
		t.Fatalf("%s, want waiting for the quality check", rec.Status)
	}
	if scanner.LastPPI != 1200 || scanner.Previews != 1 {
		t.Errorf("rescanned at %d ppi with %d previews; want 1200 and 1", scanner.LastPPI, scanner.Previews)
	}
	again, _ := Open(root, b.ID)
	if again.ScanPPI != 1200 {
		t.Errorf("batch scan ppi %d, want 1200", again.ScanPPI)
	}
	photo, _, err := FindPhoto(root, 0)
	if err != nil || photo.Front.ScanPPI != 1200 || photo.Front.MasterPPI != 1200 || photo.Front.Warning != "" {
		t.Errorf("photo after the rescan: %+v, %v", photo.Front, err)
	}
}

func TestRedoRefusals(t *testing.T) {
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	_, b, store := prepared(t, scanner)
	if err := b.Redo(context.Background(), wf.NewRunner(store, nil), "detail pass", 300); err == nil {
		t.Error("redo at 300 ppi accepted")
	}
	b.Accept()
	run(t, b, store, scanner)
	err := b.Redo(context.Background(), wf.NewRunner(store, nil), "detail pass", 0)
	if err == nil || !strings.Contains(err.Error(), "final") {
		t.Errorf("redo once bagged: %v, want a refusal", err)
	}
}
