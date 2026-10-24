package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
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

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register/registertest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

var reg = registertest.Example

var now = time.Date(2026, 10, 20, 15, 4, 5, 0, time.UTC)

func clock() time.Time { return now }

// sitting describes a batch under root.
func sitting(t *testing.T, root string) *Batch {
	t.Helper()
	b, err := Describe(root, reg, Description{Text: "SS Bonita and crew", Person: 10, ScanPPI: 600,
		Provenance: "  Aunt Jean's shoebox,\n envelope marked 1950s ", Credit: "Drummond family collection, CC BY 4.0"}, now)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// describe is a batch's first load: one load of the bed.
func describe(t *testing.T, root string) *Load {
	t.Helper()
	l, err := sitting(t, root).NewLoad("", now)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func run(t *testing.T, b *Load, store wf.Store, s Scanner) *wf.RunRecord {
	t.Helper()
	rec, err := b.Run(context.Background(), store, s, clock, nil)
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
		{"recorded", Description{Text: "SS Bonita", Person: 10, ScanPPI: 2400, Provenance: " a  shoebox ", Credit: " ©  Example "}, ""},
		{"no provenance", Description{Text: "SS Bonita", Person: 10, ScanPPI: 2400, Credit: "© Example"}, ""},
		{"no title", Description{Text: "  ", Person: 10, ScanPPI: 2400, Credit: "© Example"}, "title"},
		{"no credit", Description{Text: "SS Bonita", Person: 10, ScanPPI: 2400, Credit: " "}, "credit"},
		{"not in the register", Description{Text: "SS Bonita", Person: 12, ScanPPI: 2400, Credit: "© Example"}, "not in the register"},
		{"other scan ppi", Description{Text: "SS Bonita", Person: 10, ScanPPI: 300, Credit: "© Example"}, "600, 1200 or 2400"},
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
			if err != nil || again.Title != "SS Bonita" || again.Person != 10 || again.PersonName != "Ada Example" || again.ScanPPI != 2400 ||
				again.Provenance != strings.Join(strings.Fields(tc.d.Provenance), " ") || again.Credit != "© Example" {
				t.Errorf("%s: reopened as %+v, %v", tc.name, again, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want one mentioning %q", tc.name, err, tc.want)
		}
	}
}

// Story 014: a blank provenance is left out of photo.yaml, and the latest
// batch is where the next one's provenance and credit come from.
func TestProvenanceIsOptional(t *testing.T) {
	root := t.TempDir()
	b, err := Describe(root, reg, Description{Text: "loose", Person: 10, ScanPPI: 600, Credit: "© Example"}, now)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(b.Dir(), "batch.yaml"))
	if strings.Contains(string(raw), "provenance") {
		t.Errorf("batch.yaml records a blank provenance:\n%s", raw)
	}
}

func TestLatestBatch(t *testing.T) {
	root := t.TempDir()
	if b, err := Latest(root); b != nil || err != nil {
		t.Fatalf("no batches: %v, %v, want nil, nil", b, err)
	}
	first, _ := Describe(root, reg, Description{Text: "first", Person: 10, ScanPPI: 600, Credit: "© One"}, now)
	later, _ := Describe(root, reg, Description{Text: "later", Person: 10, ScanPPI: 600, Provenance: "box 2", Credit: "© Two"}, now.Add(time.Hour))
	if first == nil || later == nil {
		t.Fatal("describe failed")
	}
	b, err := Latest(root)
	if err != nil || b == nil || b.ID != later.ID || b.Provenance != "box 2" || b.Credit != "© Two" {
		t.Errorf("latest = %+v, %v, want %s", b, err, later.ID)
	}
}

func TestBatchIDsAreTheDateAndASequence(t *testing.T) {
	root := t.TempDir()
	a, b := sitting(t, root), sitting(t, root)
	if a.ID != "20261020-1" || b.ID != "20261020-2" {
		t.Errorf("ids %s, %s, want 20261020-1, 20261020-2", a.ID, b.ID)
	}
}

func TestBatchRunsToTheArchive(t *testing.T) {
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
	for _, f := range []string{"preview.png", "prints.yaml"} {
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
	// Story 003: each master is developed before the quality check
	for i := range scanner.Prints {
		id := pid.ID(i)
		r, err := rendition.Load(b.Renditions(id))
		if err != nil {
			t.Fatalf("photo %s: %v", id, err)
		}
		tif, _ := os.ReadFile(filepath.Join(b.Dir(), "masters", id.String()+".tif"))
		sum := sha256.Sum256(tif)
		if r.Version != rendition.Version || r.Master != hex.EncodeToString(sum[:]) || r.Display.W == 0 {
			t.Errorf("photo %s recipe %+v, want version %d of master %x", id, r, rendition.Version, sum)
		}
		if _, err := os.Stat(filepath.Join(b.Renditions(id), "display.jpg")); err != nil {
			t.Error(err)
		}
	}
	if err := b.Accept(); err != nil {
		t.Fatal(err)
	}
	rec = run(t, b, store, scanner)
	if rec.Status != wf.StatusCompleted {
		t.Fatalf("after accept: %s, want completed", rec.Status)
	}
	steps := runnertest.Steps(t, store, rec.ID)
	for _, name := range []string{"describe", "preview", "find prints", "confirm", "detail pass", "develop", "check quality", "package"} {
		if steps[name] != wf.StatusCompleted {
			t.Errorf("step %s: %s", name, steps[name])
		}
	}

	arc := filepath.Join(root, "archive")
	for i, p := range scanner.Prints {
		id := pid.ID(i)
		if !archive.Complete(arc, id) {
			t.Fatalf("photo %s not complete", id)
		}
		f, err := os.Open(filepath.Join(arc, id.String(), "data", "front.tif"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := tiff.Decode(f)
		f.Close()
		if err != nil {
			t.Fatalf("photo %s front.tif: %v", id, err)
		}
		// region = print + 0.5 mm each side, at 600 ppi
		wantW := int((p.X1 - p.X0 + 1) / 25.4 * 600)
		if got := img.Bounds().Dx(); got < wantW-4 || got > wantW+4 {
			t.Errorf("photo %s master is %d px wide, want about %d", id, got, wantW)
		}
		c := img.Bounds().Size().Div(2)
		if r, _, _, _ := img.At(c.X, c.Y).RGBA(); uint16(r) != p.Value {
			t.Errorf("photo %s centre = %d, want the print's %d", id, r, p.Value)
		}

		var photo Photo
		raw, err := os.ReadFile(filepath.Join(arc, id.String(), "data", "photo.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(raw, &photo); err != nil {
			t.Fatal(err)
		}
		if photo.ID != id || photo.Batch != b.Batch || photo.Load != 1 || photo.Description != "SS Bonita and crew" ||
			photo.Home.ID != 10 || photo.Home.Name != "Ada Example" ||
			photo.Provenance != "Aunt Jean's shoebox, envelope marked 1950s" || photo.Credit != "Drummond family collection, CC BY 4.0" ||
			photo.Front.ScanPPI != 600 || photo.Front.MasterPPI != 600 || photo.Front.Averaging != 1 ||
			!photo.Front.Captured.Equal(now) || photo.Front.Warning == "" {
			t.Errorf("photo %s photo.yaml = %+v", id, photo)
		}
		if math.Abs(photo.Front.Print.Bounds().X0-p.X0) > 1 || math.Abs(photo.Front.Print.Bounds().Y1-p.Y1) > 1 {
			t.Errorf("photo %s print at %+v, want about %v", id, photo.Front.Print, p)
		}

		tif, _ := os.ReadFile(filepath.Join(arc, id.String(), "data", "front.tif"))
		for _, want := range []string{"<dc:identifier>" + id.String() + "</dc:identifier>", "<bs:batch>" + b.Batch + "</bs:batch>", "<bs:load>1</bs:load>", "<bs:side>front</bs:side>",
			"<dc:rights>Drummond family collection, CC BY 4.0</dc:rights>"} {
			if !bytes.Contains(tif, []byte(want)) {
				t.Errorf("photo %s master's XMP lacks %s", id, want)
			}
		}
	}
}

func TestRetryAfterTheDetailPassFails(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}, FailDetails: 1}
	store := wf.NewMemStore()
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()

	if _, err := b.Run(context.Background(), store, scanner, clock, nil); err == nil || !strings.Contains(err.Error(), "USB") {
		t.Fatalf("failed detail pass: error %v", err)
	}
	if archive.Complete(filepath.Join(root, "archive"), 0) {
		t.Error("photo archived after a failed detail pass")
	}
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusWaiting {
		t.Fatalf("retry: %s, want waiting for the quality check", rec.Status)
	}
	b.Accept()
	run(t, b, store, scanner)
	if scanner.Previews != 1 || scanner.Details != 2 {
		t.Errorf("previews %d, details %d; want 1 and 2", scanner.Previews, scanner.Details)
	}
	if !archive.Complete(filepath.Join(root, "archive"), 0) {
		t.Error("photo not archived after the retry")
	}
}

func TestScannerExitingBadlyFailsTheDetailPass(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}, CloseErr: errors.New("scanimage: exit status 9")}
	store := wf.NewMemStore()
	b := describe(t, root)
	run(t, b, store, scanner)
	b.Confirm()
	if _, err := b.Run(context.Background(), store, scanner, clock, nil); err == nil || !strings.Contains(err.Error(), "exit status 9") {
		t.Errorf("error %v, want the scanner's", err)
	}
	if archive.Complete(filepath.Join(root, "archive"), 0) {
		t.Error("photo archived from a scan that failed")
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
	b := &Batch{}
	for _, w := range []struct {
		typ    wf.WorkflowType
		stages []wf.Stage
	}{
		{Workflow, b.Stages(context.Background(), nil)},
		{LoadWorkflow, (&Load{batch: b}).Stages(nil, clock)},
	} {
		var def *wf.Definition
		for i := range Definitions {
			if Definitions[i].Type == w.typ {
				def = &Definitions[i]
			}
		}
		if def == nil {
			t.Fatalf("no definition for %s", w.typ)
		}
		if len(w.stages) != len(def.Steps) {
			t.Fatalf("%s: %d stages, %d steps defined", w.typ, len(w.stages), len(def.Steps))
		}
		for i, st := range w.stages {
			if def.Steps[i].Name != st.Name || !strings.HasPrefix(def.Steps[i].Source, "https://") {
				t.Errorf("%s step %d: defined %+v, stage %s", w.typ, i, def.Steps[i], st.Name)
			}
		}
	}
	// the batch's loads step is the sub-process its loads run under
	if Definitions[0].Steps[1].Child != LoadWorkflow || !Definitions[0].Steps[1].Many {
		t.Errorf("loads step: %+v", Definitions[0].Steps[1])
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
	if err != nil || photo.ID != 0 || photo.Batch != b.Batch || filepath.Base(path) != "front.tif" {
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
func prepared(t *testing.T, scanner *batchtest.Scanner) (string, *Load, wf.Store) {
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

func redo(t *testing.T, b *Load, store wf.Store, from string, ppi int) {
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
	if _, err := os.Stat(filepath.Join(root, "archive", "0.partial")); !os.IsNotExist(err) {
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
	again, _ := Open(root, b.Batch)
	if again.ScanPPI != 1200 {
		t.Errorf("batch scan ppi %d, want 1200", again.ScanPPI)
	}
	photo, path, err := FindPhoto(root, 0)
	if err != nil || photo.Front.ScanPPI != 1200 || photo.Front.MasterPPI != 1200 || photo.Front.Warning != "" {
		t.Errorf("photo after the rescan: %+v, %v", photo.Front, err)
	}
	// Story 003: the renditions are of the new master
	tif, _ := os.ReadFile(path)
	sum := sha256.Sum256(tif)
	if r, err := rendition.Load(b.Renditions(0)); err != nil || r.Master != hex.EncodeToString(sum[:]) {
		t.Errorf("recipe after the rescan: %+v, %v; want master %x", r, err, sum)
	}
}

// Story 003, "Develop": going back to the detail pass clears this version's
// renditions; an older version's stay on record.
func TestRedoTheDetailPassClearsTheRenditions(t *testing.T) {
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	root, b, store := prepared(t, scanner)
	old := filepath.Join(root, "renditions", "0", "0")
	os.MkdirAll(old, 0o755)
	redo(t, b, store, "detail pass", 0)
	if _, err := os.Stat(b.Renditions(0)); err == nil {
		t.Error("renditions of this version survive a redo of the detail pass")
	}
	if _, err := os.Stat(old); err != nil {
		t.Error("an older version's renditions were cleared")
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
		t.Errorf("redo once archived: %v, want a refusal", err)
	}
}

func TestAdjustARegionBeforeConfirming(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	store := wf.NewMemStore()
	b := describe(t, root)
	run(t, b, store, scanner)

	prints, _, _ := b.Found()
	before := prints[0].Region
	if err := b.Adjust(0, find.Right, 3); err != nil {
		t.Fatal(err)
	}
	if err := b.Adjust(0, find.Top, -0.5); err != nil {
		t.Fatal(err)
	}
	prints, _, _ = b.Found()
	if got, was := prints[0].Region.Bounds(), before.Bounds(); !same(got.X1, was.X1+3) || !same(got.Y0, was.Y0+0.5) || !same(got.X0, was.X0) {
		t.Errorf("region %+v after the moves, from %+v", got, before)
	}
	if bed := b.Bed(); bed.X1 < 215 || bed.Y1 < 297 {
		t.Errorf("bed %+v", bed)
	}

	b.Confirm()
	run(t, b, store, scanner)
	photo, _, _ := FindPhoto(root, 0)
	if photo.Front.Region != prints[0].Region || photo.Front.Bounds != prints[0].Region.On(b.Bed()) {
		t.Errorf("region %+v covering %+v, want the adjusted %+v", photo.Front.Region, photo.Front.Bounds, prints[0].Region)
	}
	if err := b.Adjust(0, find.Right, 0.5); err == nil {
		t.Error("adjusted a region after confirming")
	}
}

func TestCloseUpsAndTheAnnotatedPreview(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	b := describe(t, root)
	run(t, b, wf.NewMemStore(), scanner)
	if img, err := b.Annotated(); err != nil || img.Bounds().Dx() != 1272 { // the bed at 150 ppi, in whole 8 px as the 4990 scans it
		t.Errorf("annotated preview: %v, %v", img, err)
	}
	img, err := b.CloseUp(0)
	if err != nil || img.Bounds().Dx() < 300 {
		t.Errorf("close-up: %v, %v; want one enlarged to at least 300 px", img.Bounds(), err)
	}
	if _, err := b.CloseUp(1); err == nil {
		t.Error("close-up of a print that is not there")
	}
}

// readTIFFSize is a TIFF's size in pixels, read from its header.
func readTIFFSize(path string) (image.Point, error) {
	f, err := os.Open(path)
	if err != nil {
		return image.Point{}, err
	}
	defer f.Close()
	c, err := tiff.DecodeConfig(f)
	return image.Pt(c.Width, c.Height), err
}

func same(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Story 004: a print turned on the bed has a region turned with it; the
// detail pass scans the region's bounds, and the master is not rotated.
func TestATurnedPrintIsScannedAsItsBounds(t *testing.T) {
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 40, Y0: 40, X1: 140, Y1: 110, Angle: 5, Value: 20000}}}
	root, b, _ := prepared(t, scanner)
	photo, _, err := FindPhoto(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	f := photo.Front
	if math.Abs(f.Skew-5) > 0.3 || f.Region.Angle != f.Skew || f.Print.Angle != f.Skew || f.Review {
		t.Errorf("skew %.2f°, region at %.2f°, print at %.2f°, review %v; want about 5°, not for review", f.Skew, f.Region.Angle, f.Print.Angle, f.Review)
	}
	if math.Abs(f.Print.W-100) > 0.5 || math.Abs(f.Print.H-70) > 0.5 {
		t.Errorf("print %.1f × %.1f mm, want 100 × 70", f.Print.W, f.Print.H)
	}
	if f.Bounds != f.Region.On(b.Bed()) {
		t.Errorf("master covers %+v, want the region's bounds %+v", f.Bounds, f.Region.On(b.Bed()))
	}
	img, err := readTIFFSize(filepath.Join(b.Dir(), "masters", photo.ID.String()+".tif"))
	if err != nil {
		t.Fatal(err)
	}
	want := image.Pt(int(math.Round((f.Bounds.X1-f.Bounds.X0)/25.4*600)), int(math.Round((f.Bounds.Y1-f.Bounds.Y0)/25.4*600)))
	if d := img.Sub(want); max(d.X, -d.X, d.Y, -d.Y) > 1 {
		t.Errorf("master %v px, want the bounds at 600 ppi, %v", img, want)
	}
}

// Story 004: the close-up of a turned print is drawn square to the print.
func TestTheCloseUpOfATurnedPrintSitsSquare(t *testing.T) {
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 40, Y0: 40, X1: 140, Y1: 110, Angle: 5, Value: 20000}}}
	root := t.TempDir()
	b := describe(t, root)
	run(t, b, wf.NewMemStore(), scanner)
	img, err := b.CloseUp(0)
	if err != nil {
		t.Fatal(err)
	}
	// where the print starts on a row near its top and one near its bottom
	start := func(y int) int {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r>>8 < 128 {
				return x
			}
		}
		return -1
	}
	h := img.Bounds().Dy()
	top, bottom := start(h/4), start(3*h/4)
	if top < 0 || max(top-bottom, bottom-top) > 4 {
		t.Errorf("the print's left edge is at %d px near the top and %d px near the bottom; want it square", top, bottom)
	}
}

func TestAdjustRefusals(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 0, Y0: 0, X1: 60, Y1: 50, Value: 20000}}}
	store := wf.NewMemStore()
	b := describe(t, root)
	run(t, b, store, scanner)
	if err := b.Adjust(1, find.Right, 0.5); err == nil {
		t.Error("adjusted a region that is not there")
	}
	if err := b.Adjust(0, find.Left, 0.5); err == nil {
		t.Error("moved an edge past the bed")
	}
}

func TestReshapeFromTheQualityCheck(t *testing.T) {
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	root, b, store := prepared(t, scanner)
	before, _, _ := FindPhoto(root, 0)
	runner := wf.NewRunner(store, nil)
	if err := b.Reshape(context.Background(), runner, 0, find.Right, -0.5); err != nil {
		t.Fatal(err)
	}
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusWaiting {
		t.Fatalf("%s, want back at the quality check", rec.Status)
	}
	after, _, err := FindPhoto(root, 0)
	if err != nil || !same(after.Front.Region.Bounds().X1, before.Front.Region.Bounds().X1-0.5) {
		t.Errorf("region %+v after the reshape, from %+v (%v)", after.Front.Region, before.Front.Region, err)
	}
	if scanner.Details != 2 || scanner.Previews != 1 {
		t.Errorf("details %d, previews %d; want 2 and 1", scanner.Details, scanner.Previews)
	}
	if _, ids, _ := b.Found(); len(ids) != 1 || ids[0] != 0 {
		t.Errorf("ids %v, want the photo id kept", ids)
	}
	// a move that cannot happen leaves the batch where it was
	if err := b.Reshape(context.Background(), runner, 0, find.Bottom, -30); err == nil {
		t.Error("reshape under 20 mm accepted")
	}
	if _, err := os.Stat(filepath.Join(b.Dir(), "masters")); err != nil {
		t.Error("a refused reshape cleared the masters")
	}
}

func TestSplitAMergedRegion(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 10, Y0: 10, X1: 70, Y1: 70, Value: 20000},
		{X0: 73, Y0: 10, X1: 133, Y1: 70, Value: 20000}, // 3 mm apart: found as one
		{X0: 10, Y0: 100, X1: 70, Y1: 150, Value: 20000},
	}}
	b := describe(t, root)
	run(t, b, wf.NewMemStore(), scanner)
	prints, _, _ := b.Found()
	if len(prints) != 2 || prints[0].Parts != 2 {
		t.Fatalf("found %d regions, first with %d parts; want 2 regions, the first merged", len(prints), prints[0].Parts)
	}
	if err := b.Split(0); err != nil {
		t.Fatal(err)
	}
	prints, _, _ = b.Found()
	if len(prints) != 3 {
		t.Fatalf("%d regions after the split, want 3", len(prints))
	}
	if math.Abs(prints[0].Print.Bounds().X1-70) > 0.5 || math.Abs(prints[1].Print.Bounds().X0-73) > 0.5 || math.Abs(prints[2].Print.Bounds().Y0-100) > 0.5 {
		t.Errorf("after the split: %+v, %+v, %+v", prints[0].Print, prints[1].Print, prints[2].Print)
	}
	if err := b.Split(2); err == nil {
		t.Error("split a region holding one print")
	}
}

func TestRemoveARegion(t *testing.T) {
	root := t.TempDir()
	scanner := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 10, Y0: 10, X1: 70, Y1: 70, Value: 20000},
		{X0: 10, Y0: 100, X1: 70, Y1: 150, Value: 20000},
	}}
	b := describe(t, root)
	store := wf.NewMemStore()
	run(t, b, store, scanner)
	if err := b.Remove(0); err != nil {
		t.Fatal(err)
	}
	prints, _, _ := b.Found()
	if len(prints) != 1 || math.Abs(prints[0].Print.Bounds().Y0-100) > 0.5 {
		t.Errorf("after removing the first: %+v", prints)
	}
	if err := b.Remove(1); err == nil {
		t.Error("removed a region that is not there")
	}
	b.Confirm()
	if err := b.Remove(0); err == nil {
		t.Error("removed a region after confirming")
	}
}

// #37: how long a step should take is judged from the steps completed
// before it.
func TestExpectedDurationOfAStep(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := wf.NewMemStore()
	bed := find.Rect{X1: 216, Y1: 297}
	// past is a batch scanned at ppi, one print of w × h mm, whose preview
	// and detail pass took as given.
	past := func(ppi int, w, h float64, preview, detail time.Duration) {
		t.Helper()
		past, err := Describe(root, reg, Description{Text: "past", Person: 10, ScanPPI: ppi, Credit: "c"}, now)
		if err != nil {
			t.Fatal(err)
		}
		b, err := past.NewLoad("", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeYAML(b.path("prints.yaml"), found{Bed: bed, Prints: []find.Print{{Region: find.Box{CX: 100, CY: 100, W: w, H: h}}}}); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateRun(ctx, &wf.RunRecord{ID: b.Key(), WorkflowType: LoadWorkflow, Key: b.Key(), Status: wf.StatusCompleted, StartedAt: now, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		for name, d := range map[string]time.Duration{"preview": preview, "detail pass": detail} {
			if err := store.RecordStep(ctx, &wf.StepResult{ID: b.Key() + name, RunID: b.Key(), StepName: name, Status: wf.StatusCompleted, DurationNs: int64(d)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	b := describe(t, root) // 600 ppi
	if err := writeYAML(b.path("prints.yaml"), found{Bed: bed, Prints: []find.Print{{Region: find.Box{CX: 100, CY: 100, W: 80, H: 60}}}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := Expected(ctx, store, root, b, "preview"); ok {
		t.Error("nothing scanned yet: want no expectation")
	}
	past(600, 40, 30, 10*time.Second, 12*time.Second)  // 1200 mm² in 12 s: 10 ms per mm²
	past(600, 40, 60, 14*time.Second, 36*time.Second)  // 2400 mm² in 36 s: 15 ms per mm²
	past(1200, 40, 30, 12*time.Second, 90*time.Second) // another ppi: says nothing about this batch's detail pass
	for _, c := range []struct {
		step string
		want time.Duration
	}{
		{"preview", 12 * time.Second},     // the median of 10, 14 and 12
		{"detail pass", 60 * time.Second}, // 4800 mm² at the median 12.5 ms per mm²
		{"find prints", 0},                // never timed
	} {
		got, ok := Expected(ctx, store, root, b, c.step)
		if ok != (c.want != 0) || got != c.want {
			t.Errorf("%s: got %v %v, want %v", c.step, got, ok, c.want)
		}
	}
}
