package batch

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
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
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
)

var reg = register.Register{Dir: "testdata/people"}

var now = time.Date(2026, 10, 20, 15, 4, 5, 0, time.UTC)

func clock() time.Time { return now }

// A test print: where it lies on the bed in mm, and the value every
// sample of it scans as.
type testPrint struct {
	x0, y0, x1, y1 float64
	value          uint16
}

// fakeScanner serves a bed with prints on the lid.
type fakeScanner struct {
	prints      []testPrint
	previews    int
	details     int
	failDetails int   // fail this many detail passes first
	closeErr    error // the scan's stream fails as it closes, as scanimage exiting badly does
}

type closeFails struct {
	io.Reader
	err error
}

func (c closeFails) Close() error { return c.err }

func (f *fakeScanner) valueAt(xmm, ymm float64) uint16 {
	for _, p := range f.prints {
		if xmm >= p.x0 && xmm < p.x1 && ymm >= p.y0 && ymm < p.y1 {
			return p.value
		}
	}
	return 65535
}

func (f *fakeScanner) Name() string       { return "Fake Scanner" }
func (f *fakeScanner) Settings() []string { return []string{"colour correction off"} }

func (f *fakeScanner) Preview(context.Context) (image.Image, error) {
	f.previews++
	const ppi = 75
	w, h := 637, 877 // the bed at 75 ppi
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(f.valueAt((float64(x)+0.5)*25.4/ppi, (float64(y)+0.5)*25.4/ppi) >> 8)
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img, nil
}

func (f *fakeScanner) Detail(_ context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	f.details++
	if f.details <= f.failDetails {
		return nil, errors.New("scanner: USB disconnected")
	}
	px := func(mm float64) int { return int(math.Round(mm / 25.4 * float64(ppi))) }
	w, h := px(area.X1-area.X0), px(area.Y1-area.Y0)
	var b bytes.Buffer
	fmt.Fprintf(&b, "P6\n%d %d\n65535\n", w, h)
	row := make([]byte, w*6)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := f.valueAt(area.X0+(float64(x)+0.5)*25.4/float64(ppi), area.Y0+(float64(y)+0.5)*25.4/float64(ppi))
			for c := 0; c < 3; c++ {
				binary.BigEndian.PutUint16(row[(x*3+c)*2:], v)
			}
		}
		b.Write(row)
	}
	return closeFails{&b, f.closeErr}, nil
}

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
	scanner := &fakeScanner{prints: []testPrint{
		{20, 20, 60, 50, 20000}, // 40 × 30 mm
		{90, 25, 125, 60, 40000},
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
	if scanner.details != 0 {
		t.Error("detail pass ran before confirm")
	}

	if err := b.Confirm(); err != nil {
		t.Fatal(err)
	}
	rec = run(t, b, store, scanner)
	if rec.Status != wf.StatusCompleted {
		t.Fatalf("after confirm: %s, want completed", rec.Status)
	}
	steps := runnertest.Steps(t, store, runnertest.OnlyRun(t, store).ID)
	for _, name := range []string{"describe", "preview", "find prints", "confirm", "detail pass", "package"} {
		if steps[name] != wf.StatusCompleted {
			t.Errorf("step %s: %s", name, steps[name])
		}
	}

	bags := filepath.Join(root, "bags")
	for i, p := range scanner.prints {
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
		// region = print + 3 mm each side, at 600 ppi
		wantW := int((p.x1 - p.x0 + 6) / 25.4 * 600)
		if got := img.Bounds().Dx(); got < wantW-4 || got > wantW+4 {
			t.Errorf("bag %s master is %d px wide, want about %d", id, got, wantW)
		}
		c := img.Bounds().Size().Div(2)
		if r, _, _, _ := img.At(c.X, c.Y).RGBA(); uint16(r) != p.value {
			t.Errorf("bag %s centre = %d, want the print's %d", id, r, p.value)
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
		if math.Abs(photo.Front.Print.X0-p.x0) > 1 || math.Abs(photo.Front.Print.Y1-p.y1) > 1 {
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
	scanner := &fakeScanner{prints: []testPrint{{20, 20, 60, 50, 20000}}, failDetails: 1}
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
	if rec := run(t, b, store, scanner); rec.Status != wf.StatusCompleted {
		t.Fatalf("retry: %s, want completed", rec.Status)
	}
	if scanner.previews != 1 || scanner.details != 2 {
		t.Errorf("previews %d, details %d; want 1 and 2", scanner.previews, scanner.details)
	}
	if !bag.Complete(filepath.Join(root, "bags"), 0) {
		t.Error("bag not complete after the retry")
	}
}

func TestScannerExitingBadlyFailsTheDetailPass(t *testing.T) {
	root := t.TempDir()
	scanner := &fakeScanner{prints: []testPrint{{20, 20, 60, 50, 20000}}, closeErr: errors.New("scanimage: exit status 9")}
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
	if rec := run(t, b, store, &fakeScanner{}); rec.Status != wf.StatusWaiting {
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
