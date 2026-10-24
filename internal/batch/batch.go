// Package batch runs a batch, one sitting of scanning, as a gobank-workflow
// instance keyed by batch id, and each load of the scanner bed within it
// as a child instance keyed by the load: describe, preview, find prints,
// confirm, the detail pass, develop, and an archived photo for each print.
//
// A batch's and a load's state lives in its directory under the staging
// root, not in the run: the workflow records only which steps are done,
// and every step reads back what it needs, so a load resumes after a
// crash.
package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/plan"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// PreviewPPI is the resolution of the preview prints are found on. The
// 4990 scans whole 8 px blocks across, so the preview falls short of the
// bed's right edge by up to 8 px: 2 mm at 75 ppi, 0.5 mm at 150.
const PreviewPPI = 150

// Scanner is the scanner as a batch uses it.
type Scanner interface {
	// Preview scans the whole bed at PreviewPPI.
	Preview(ctx context.Context) (image.Image, error)
	// Detail scans area at ppi, 16-bit colour, as a PNM stream.
	Detail(ctx context.Context, area find.Rect, ppi int) (io.ReadCloser, error)
	// Name is the scanner's make and model.
	Name() string
	// Settings are the fixed settings of every pass, as recorded with a master.
	Settings() []string
}

// Description is what the operator says about a batch before scanning.
type Description struct {
	Text       string // the batch's title: what is being scanned
	Person     pid.ID // the home person: whose collection the photos belong to
	ScanPPI    int    // 600, 1200 or 2400
	Provenance string // where the prints came from; may be unknown
	Credit     string // the rights line the photos are shown under
}

// Batch is one sitting of scanning: one home person, provenance, credit
// and scan ppi, over as many loads of the bed as the shoebox needs.
type Batch struct {
	ID         string    `yaml:"id"`
	Title      string    `yaml:"title"`
	Person     pid.ID    `yaml:"person"`
	PersonName string    `yaml:"person_name"`
	ScanPPI    int       `yaml:"scan_ppi"`
	Provenance string    `yaml:"provenance,omitempty"`
	Credit     string    `yaml:"credit"`
	Created    time.Time `yaml:"created"`

	root string
}

// Load is one load of the bed within a batch: its description, preview,
// prints, detail pass, masters and the photos archived from them.
type Load struct {
	Batch       string    `yaml:"batch"`
	N           int       `yaml:"load"` // counted from 1 within the batch
	Description string    `yaml:"description"`
	Created     time.Time `yaml:"created"`

	batch *Batch
	root  string
}

// Person is the home of a photo: whose album it belongs to.
type Person struct {
	ID   pid.ID `yaml:"id"`
	Name string `yaml:"name"`
}

// Photo is an archived photo's photo.yaml: what is known of one print at
// capture. Its provenance and credit are its batch's, its description its
// load's.
type Photo struct {
	ID          pid.ID `yaml:"id"`
	Batch       string `yaml:"batch"`
	Load        int    `yaml:"load,omitempty"`
	Description string `yaml:"description"`
	Home        Person `yaml:"home"`
	Provenance  string `yaml:"provenance,omitempty"`
	Credit      string `yaml:"credit"`
	Front       Side   `yaml:"front"`
}

// Side is the capture of one side of a print.
type Side struct {
	Captured  time.Time `yaml:"captured"`
	Print     find.Box  `yaml:"print"`  // the print on the bed, mm, square to itself
	Region    find.Box  `yaml:"region"` // the print with its margin, likewise
	Bounds    find.Rect `yaml:"bounds"` // what the master covers: the region's bounds on the bed, mm
	ScanPPI   int       `yaml:"scan_ppi"`
	MasterPPI int       `yaml:"master_ppi"`
	Averaging int       `yaml:"averaging"` // scan pixels per master pixel, each way
	Skew      float64   `yaml:"skew"`
	Fill      float64   `yaml:"fill"`
	Review    bool      `yaml:"review"`
	Warning   string    `yaml:"warning,omitempty"`
	Scanner   string    `yaml:"scanner"`
	Settings  []string  `yaml:"settings"`
}

// found is prints.yaml: the prints found on the preview and, once
// confirmed, their photo ids.
type found struct {
	Bed    find.Rect    `yaml:"bed"` // the whole bed, as the preview covers it
	Prints []find.Print `yaml:"prints"`
	IDs    []pid.ID     `yaml:"ids,omitempty"`
}

// Describe records a new batch under root, refusing a description the
// story's rules reject.
func Describe(root string, reg register.Register, d Description, now time.Time) (*Batch, error) {
	text, provenance, credit := oneLine(d.Text), oneLine(d.Provenance), oneLine(d.Credit)
	if text == "" {
		return nil, errors.New("a batch needs a title")
	}
	if credit == "" {
		return nil, errors.New("a batch needs a credit: the terms its photos are shown under")
	}
	if d.ScanPPI != 600 && d.ScanPPI != 1200 && d.ScanPPI != 2400 {
		return nil, fmt.Errorf("scan ppi %d, want 600, 1200 or 2400", d.ScanPPI)
	}
	name, err := reg.Person(d.Person)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "batches")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	b := &Batch{Title: text, Person: d.Person, PersonName: name, ScanPPI: d.ScanPPI,
		Provenance: provenance, Credit: credit, Created: now, root: root}
	for n := 1; ; n++ {
		b.ID = now.Format("20060102") + "-" + strconv.Itoa(n)
		err := os.Mkdir(b.Dir(), 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	return b, writeYAML(filepath.Join(b.Dir(), "batch.yaml"), b)
}

// oneLine collapses what the operator typed to single spaces.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Latest is the most recently created batch under root, or nil when there
// is none.
func Latest(root string) (*Batch, error) {
	entries, err := os.ReadDir(filepath.Join(root, "batches"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var latest *Batch
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := Open(root, e.Name())
		if err != nil {
			continue // a batch whose record is missing or broken is not the one to copy
		}
		if latest == nil || b.Created.After(latest.Created) {
			latest = b
		}
	}
	return latest, nil
}

// Open reads the batch id under root.
func Open(root, id string) (*Batch, error) {
	b := &Batch{root: root}
	if err := readYAML(filepath.Join(root, "batches", id, "batch.yaml"), b); err != nil {
		return nil, fmt.Errorf("batch %s: %w", id, err)
	}
	return b, nil
}

// Dir is the batch's directory.
func (b *Batch) Dir() string { return filepath.Join(b.root, "batches", b.ID) }

func (b *Batch) path(name string) string { return filepath.Join(b.Dir(), name) }

// NewLoad adds the next load of the bed to the batch, described as given
// or, left blank, by the batch's title. A finished batch takes no more.
func (b *Batch) NewLoad(description string, now time.Time) (*Load, error) {
	if b.Finished() {
		return nil, fmt.Errorf("batch %s: scanning is finished", b.ID)
	}
	description = oneLine(description)
	if description == "" {
		description = b.Title
	}
	if err := os.MkdirAll(b.path("loads"), 0o755); err != nil {
		return nil, err
	}
	l := &Load{Batch: b.ID, Description: description, Created: now, batch: b, root: b.root}
	for l.N = 1; ; l.N++ {
		err := os.Mkdir(l.Dir(), 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	return l, writeYAML(l.path("load.yaml"), l)
}

// Loads are the batch's loads in order.
func (b *Batch) Loads() ([]*Load, error) {
	entries, err := os.ReadDir(b.path("loads"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var loads []*Load
	for _, e := range entries {
		n, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		l, err := b.Load(n)
		if err != nil {
			return nil, err
		}
		loads = append(loads, l)
	}
	slices.SortFunc(loads, func(a, b *Load) int { return a.N - b.N })
	return loads, nil
}

// Load reads the batch's load n.
func (b *Batch) Load(n int) (*Load, error) {
	l := &Load{batch: b, root: b.root}
	if err := readYAML(filepath.Join(b.path("loads"), strconv.Itoa(n), "load.yaml"), l); err != nil {
		return nil, fmt.Errorf("load %s/%d: %w", b.ID, n, err)
	}
	return l, nil
}

// OpenLoad reads the load under root by its key, <batch id>/<n>.
func OpenLoad(root, key string) (*Load, error) {
	id, n, ok := strings.Cut(key, "/")
	num, err := strconv.Atoi(n)
	if !ok || err != nil {
		return nil, fmt.Errorf("load %q: want <batch id>/<n>", key)
	}
	b, err := Open(root, id)
	if err != nil {
		return nil, err
	}
	return b.Load(num)
}

// Finished reports whether the operator has ended the batch's scanning.
func (b *Batch) Finished() bool {
	_, err := os.Stat(b.path("finished"))
	return err == nil
}

// Finish ends the batch's scanning, once every load is through, and ticks
// the batch's workflow on.
func (b *Batch) Finish(ctx context.Context, store wf.Store) (*wf.RunRecord, error) {
	loads, err := b.Loads()
	if err != nil {
		return nil, err
	}
	if len(loads) == 0 {
		return nil, fmt.Errorf("batch %s: nothing has been scanned", b.ID)
	}
	for _, l := range loads {
		if done, err := l.done(ctx, store); err != nil {
			return nil, err
		} else if !done {
			return nil, fmt.Errorf("batch %s: load %d is not through yet", b.ID, l.N)
		}
	}
	if err := os.WriteFile(b.path("finished"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return nil, err
	}
	return b.Run(ctx, store)
}

// Run ticks the batch's workflow once: it waits at loads until scanning
// is finished and every load is through.
func (b *Batch) Run(ctx context.Context, store wf.Store) (*wf.RunRecord, error) {
	date := wf.NewBusinessDate(b.Created.Year(), b.Created.Month(), b.Created.Day())
	return wf.NewRunner(store, nil).RunPipeline(ctx, Workflow, b.ID, date, b.Stages(ctx, store))
}

// Stages are the batch's workflow steps, in order.
func (b *Batch) Stages(ctx context.Context, store wf.Store) []wf.Stage {
	return []wf.Stage{
		{Name: "describe", Run: func(context.Context) error {
			_, err := os.Stat(b.path("batch.yaml"))
			return err
		}},
		{Name: "loads", Until: func(context.Context) (bool, error) {
			if !b.Finished() {
				return false, nil
			}
			loads, err := b.Loads()
			if err != nil {
				return false, err
			}
			for _, l := range loads {
				if done, err := l.done(ctx, store); err != nil || !done {
					return false, err
				}
			}
			return true, nil
		}},
	}
}

// Key identifies the load's workflow instance: <batch id>/<n>.
func (l *Load) Key() string { return l.Batch + "/" + strconv.Itoa(l.N) }

// Dir is the load's directory.
func (l *Load) Dir() string { return filepath.Join(l.batch.Dir(), "loads", strconv.Itoa(l.N)) }

func (l *Load) path(name string) string { return filepath.Join(l.Dir(), name) }

// Sitting is the batch the load belongs to.
func (l *Load) Sitting() *Batch { return l.batch }

// done reports whether the load's workflow has completed.
func (l *Load) done(ctx context.Context, store wf.Store) (bool, error) {
	run, err := store.FindRunByKey(ctx, LoadWorkflow, l.Key())
	if err != nil {
		return false, err
	}
	return run != nil && run.Status == wf.StatusCompleted, nil
}

// Confirm accepts the prints found on the preview.
func (b *Load) Confirm() error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return fmt.Errorf("load %s: nothing to confirm yet: %w", b.Key(), err)
	}
	if len(f.Prints) == 0 {
		return fmt.Errorf("load %s: no prints were found on the bed", b.Key())
	}
	return os.WriteFile(b.path("confirmed"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Bed is the bed the preview covers, in mm; zero before the preview.
func (b *Load) Bed() find.Rect {
	var f found
	readYAML(b.path("prints.yaml"), &f)
	return f.Bed
}

// Adjust moves one edge of print n's region by mm, outwards when mm is
// positive. Regions can be adjusted only until they are confirmed.
func (b *Load) Adjust(n int, e find.Edge, mm float64) error {
	f, err := b.editable(n)
	if err != nil {
		return err
	}
	r, err := f.Prints[n].Region.Move(e, mm, f.Bed)
	if err != nil {
		return fmt.Errorf("print %d: %w", n+1, err)
	}
	f.Prints[n].Region = r
	return writeYAML(b.path("prints.yaml"), f)
}

// editable reads the prints for an edit, refusing once they are confirmed
// or when there is no print n.
func (b *Load) editable(n int) (found, error) {
	var f found
	if _, err := os.Stat(b.path("confirmed")); err == nil {
		return f, fmt.Errorf("load %s: the prints are confirmed; go back to the prints to change them", b.Key())
	}
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return f, fmt.Errorf("load %s: no prints yet: %w", b.Key(), err)
	}
	if n < 0 || n >= len(f.Prints) {
		return f, fmt.Errorf("load %s has no print %d", b.Key(), n+1)
	}
	return f, nil
}

// Split replaces print n's region, when it holds prints too close to be
// told apart, with one region for each print within it.
func (b *Load) Split(n int) error {
	f, err := b.editable(n)
	if err != nil {
		return err
	}
	img, err := b.Preview()
	if err != nil {
		return err
	}
	r := f.Prints[n].Region
	var parts []find.Print
	for _, p := range find.PrintsApart(img, PreviewPPI) {
		if r.Contains(p.Print.CX, p.Print.CY) {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 {
		return fmt.Errorf("print %d: no separate prints found within it to split", n+1)
	}
	f.Prints = append(f.Prints[:n], append(parts, f.Prints[n+1:]...)...)
	return writeYAML(b.path("prints.yaml"), f)
}

// Remove drops print n's region: a mark that is not a print.
func (b *Load) Remove(n int) error {
	f, err := b.editable(n)
	if err != nil {
		return err
	}
	f.Prints = append(f.Prints[:n], f.Prints[n+1:]...)
	return writeYAML(b.path("prints.yaml"), f)
}

// Reshape moves one edge of print n's region from the quality check: the
// batch goes back to confirm, the edge moves, and the prints are confirmed
// again, so the next tick rescans them. A move that cannot happen is
// refused before anything is undone.
func (b *Load) Reshape(ctx context.Context, runner *wf.Runner, n int, e find.Edge, mm float64) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	if n < 0 || n >= len(f.Prints) {
		return fmt.Errorf("load %s has no print %d", b.Key(), n+1)
	}
	if _, err := f.Prints[n].Region.Move(e, mm, f.Bed); err != nil {
		return fmt.Errorf("print %d: %w", n+1, err)
	}
	if err := b.Redo(ctx, runner, "confirm", 0); err != nil {
		return err
	}
	if err := b.Adjust(n, e, mm); err != nil {
		return err
	}
	return b.Confirm()
}

// Accept passes the masters at the quality check.
func (b *Load) Accept() error {
	if _, err := os.Stat(b.path("masters")); err != nil {
		return fmt.Errorf("load %s: no masters to accept yet", b.Key())
	}
	return os.WriteFile(b.path("accepted"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Redo sends the batch back to the step named from, which and every later
// step run again on the next tick. A ppi other than 0 changes the batch's
// scan ppi first. What the operator decided at the steps cleared is
// cleared with them: the confirmation, the acceptance, and, when the
// prints are found again, their photo ids.
func (b *Load) Redo(ctx context.Context, runner *wf.Runner, from string, ppi int) error {
	if ppi != 0 && ppi != 600 && ppi != 1200 && ppi != 2400 {
		return fmt.Errorf("scan ppi %d, want 600, 1200 or 2400", ppi)
	}
	stages := b.Stages(nil, nil)
	if _, err := runner.Rewind(ctx, LoadWorkflow, b.Key(), from, stages); err != nil {
		return err
	}
	cleared := map[string]bool{}
	for i, st := range stages {
		if st.Name == from {
			for _, later := range stages[i:] {
				cleared[later.Name] = true
			}
		}
	}
	if ppi != 0 && ppi != b.batch.ScanPPI {
		b.batch.ScanPPI = ppi
		if err := writeYAML(b.batch.path("batch.yaml"), b.batch); err != nil {
			return err
		}
	}
	if cleared["find prints"] {
		if _, ids, err := b.Found(); err == nil {
			for _, id := range ids {
				if err := archive.Release(b.archive(), id); err != nil {
					return err
				}
			}
		}
		if err := os.Remove(b.path("prints.yaml")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for step, marker := range map[string]string{"confirm": "confirmed", "check quality": "accepted", "detail pass": "masters"} {
		if cleared[step] {
			if err := os.RemoveAll(b.path(marker)); err != nil {
				return err
			}
		}
	}
	if cleared["develop"] {
		// the master is about to change: this recipe's renditions of it go,
		// an older recipe's stay on record
		if _, ids, err := b.Found(); err == nil {
			for _, id := range ids {
				if err := os.RemoveAll(b.Renditions(id)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Run ticks the load's workflow once: it runs until it completes, fails
// or waits for the operator. The load runs as a child of its batch's
// workflow, which is ticked first so that it exists to be the parent.
// progress, when given, is told each step as it starts.
func (b *Load) Run(ctx context.Context, store wf.Store, s Scanner, now func() time.Time, progress func(step string)) (*wf.RunRecord, error) {
	date := wf.NewBusinessDate(b.Created.Year(), b.Created.Month(), b.Created.Day())
	runner := wf.NewRunner(store, nil)
	if _, err := b.batch.Run(ctx, store); err != nil {
		return nil, err
	}
	parent, err := store.FindRunByKey(ctx, Workflow, b.Batch)
	if err != nil || parent == nil {
		return nil, fmt.Errorf("load %s: no run for batch %s: %v", b.Key(), b.Batch, err)
	}
	if _, err := runner.StartChild(ctx, parent.ID, LoadWorkflow, b.Key(), date); err != nil {
		return nil, err
	}
	stages := b.Stages(s, now)
	if progress != nil {
		for i := range stages {
			name, run := stages[i].Name, stages[i].Run
			if run == nil {
				continue
			}
			stages[i].Run = func(ctx context.Context) error {
				progress(name)
				return run(ctx)
			}
		}
	}
	return runner.RunPipeline(ctx, LoadWorkflow, b.Key(), date, stages)
}

// Stages are the load's workflow steps, in order.
func (b *Load) Stages(s Scanner, now func() time.Time) []wf.Stage {
	return []wf.Stage{
		{Name: "describe", Run: func(context.Context) error {
			_, err := os.Stat(b.path("load.yaml"))
			return err
		}},
		{Name: "preview", Run: func(ctx context.Context) error {
			img, err := s.Preview(ctx)
			if err != nil {
				return err
			}
			return writeAtomic(b.path("preview.png"), func(w io.Writer) error { return png.Encode(w, img) })
		}},
		{Name: "find prints", Run: func(context.Context) error {
			img, err := readPNG(b.path("preview.png"))
			if err != nil {
				return err
			}
			prints := find.Prints(img, PreviewPPI)
			mm := func(px int) float64 { return float64(px) * 25.4 / PreviewPPI }
			bed := find.Rect{X1: mm(img.Bounds().Dx()), Y1: mm(img.Bounds().Dy())}
			return writeYAML(b.path("prints.yaml"), found{Bed: bed, Prints: prints})
		}},
		{Name: "confirm",
			Until: func(context.Context) (bool, error) {
				_, err := os.Stat(b.path("confirmed"))
				return err == nil, nil
			},
			Run: b.reserveIDs,
		},
		{Name: "detail pass", Run: func(ctx context.Context) error { return b.detailPass(ctx, s, now) }},
		{Name: "develop", Run: func(context.Context) error { return b.develop() }},
		{Name: "check quality", Until: func(context.Context) (bool, error) {
			_, err := os.Stat(b.path("accepted"))
			return err == nil, nil
		}},
		// An archived photo is never altered, so nothing before packaging can be redone after it.
		{Name: "package", Run: func(context.Context) error { return b.pack(now) }, Final: true},
	}
}

func (b *Load) archive() string { return filepath.Join(b.root, "archive") }

// reserveIDs gives each confirmed print its photo id, once.
func (b *Load) reserveIDs(context.Context) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	if len(f.IDs) == len(f.Prints) {
		return nil
	}
	if err := os.MkdirAll(b.archive(), 0o755); err != nil {
		return err
	}
	for len(f.IDs) < len(f.Prints) {
		id, err := archive.Reserve(b.archive())
		if err != nil {
			return err
		}
		f.IDs = append(f.IDs, id)
		// recorded after each, so a crash cannot reserve a print twice
		if err := writeYAML(b.path("prints.yaml"), f); err != nil {
			return err
		}
	}
	return nil
}

// pass is the detail pass the batch's prints call for, with the prints.
func (b *Load) pass() (plan.Pass, found, error) {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return plan.Pass{}, f, err
	}
	p, err := plan.New(f.Prints, f.Bed, b.batch.ScanPPI)
	return p, f, err
}

// detailPass scans every region once and writes each master, with its
// photo.yaml, into the batch's masters directory.
func (b *Load) detailPass(ctx context.Context, s Scanner, now func() time.Time) error {
	p, f, err := b.pass()
	if err != nil {
		return err
	}
	captured := now().UTC()
	rc, err := s.Detail(ctx, p.Area, p.ScanPPI)
	if err != nil {
		return err
	}
	imgs, err := detailpass.Read(rc, p.Regions)
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	dir := b.path("masters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i, img := range imgs {
		pr, m := f.Prints[i], p.Masters[i]
		photo := Photo{
			ID: f.IDs[i], Batch: b.Batch, Load: b.N, Description: b.Description,
			Home: Person{b.batch.Person, b.batch.PersonName}, Provenance: b.batch.Provenance, Credit: b.batch.Credit,
			Front: Side{
				Captured: captured, Print: pr.Print, Region: pr.Region, Bounds: pr.Region.On(f.Bed),
				ScanPPI: p.ScanPPI, MasterPPI: m.PPI, Averaging: m.Factor,
				Skew: pr.Skew, Fill: pr.Fill, Review: pr.Review, Warning: m.Warning,
				Scanner: s.Name(), Settings: s.Settings(),
			},
		}
		name := photo.ID.String()
		if err := writeAtomic(filepath.Join(dir, name+".tif"), func(w io.Writer) error {
			return master.Encode(w, img, master.Options{PPI: m.PPI, XMP: xmp(photo, "front")})
		}); err != nil {
			return err
		}
		if err := writeYAML(filepath.Join(dir, name+".yaml"), photo); err != nil {
			return err
		}
	}
	return nil
}

// Renditions is where the photo id's renditions at the current recipe
// are kept: beside the archive, never in it.
func (b *Load) Renditions(id pid.ID) string {
	return filepath.Join(b.root, "renditions", id.String(), strconv.Itoa(rendition.Version))
}

// develop makes each master's renditions at the current recipe, skipping
// a photo already developed at it.
func (b *Load) develop() error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	for _, id := range f.IDs {
		dir := b.Renditions(id)
		if _, err := rendition.Load(dir); err == nil {
			continue
		}
		var photo Photo
		name := filepath.Join(b.path("masters"), id.String())
		if err := readYAML(name+".yaml", &photo); err != nil {
			return err
		}
		tif, err := os.ReadFile(name + ".tif")
		if err != nil {
			return err
		}
		img, err := master.Decode(bytes.NewReader(tif))
		if err != nil {
			return fmt.Errorf("photo %s: %w", id, err)
		}
		side := photo.Front
		r, imgs := rendition.Develop(img, side.Print.InImage(side.Bounds, side.MasterPPI), side.MasterPPI)
		sum := sha256.Sum256(tif)
		r.Master = hex.EncodeToString(sum[:])
		if err := rendition.Save(dir, r, imgs); err != nil {
			return fmt.Errorf("photo %s: %w", id, err)
		}
	}
	return nil
}

// pack archives each print's master and photo.yaml as a photo, skipping
// photos already complete, then clears the batch's masters.
func (b *Load) pack(now func() time.Time) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	dir := b.path("masters")
	for _, id := range f.IDs {
		if archive.Complete(b.archive(), id) {
			continue
		}
		ph, err := archive.Create(b.archive(), id)
		if err != nil {
			return err
		}
		for _, file := range [][2]string{{id.String() + ".tif", "front.tif"}, {id.String() + ".yaml", "photo.yaml"}} {
			if err := ph.Add(file[1], copyFrom(filepath.Join(dir, file[0]))); err != nil {
				return err
			}
		}
		if err := ph.Close(archive.Info{Batch: b.Batch, Load: b.N, Description: b.Description, Bagged: now()}); err != nil {
			return err
		}
	}
	return os.RemoveAll(dir)
}

func copyFrom(path string) func(io.Writer) error {
	return func(w io.Writer) error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}
}

// writeAtomic writes path through a temporary file renamed into place, so
// a crash never leaves a part-written file under the real name.
func writeAtomic(path string, write func(io.Writer) error) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeYAML(path string, v any) error {
	return writeAtomic(path, func(w io.Writer) error {
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		if err := enc.Encode(v); err != nil {
			return err
		}
		return enc.Close()
	})
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// Found is the prints found on the preview and, once confirmed, their
// photo ids.
func (b *Load) Found() ([]find.Print, []pid.ID, error) {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return nil, nil, err
	}
	return f.Prints, f.IDs, nil
}

// FindPhoto is a photo's record and the path of its front master: in its
// archive, or before packaging in its load.
func FindPhoto(root string, id pid.ID) (Photo, string, error) {
	var p Photo
	data := filepath.Join(root, "archive", id.String(), "data")
	if err := readYAML(filepath.Join(data, "photo.yaml"), &p); err == nil {
		return p, filepath.Join(data, "front.tif"), nil
	}
	found, _ := filepath.Glob(filepath.Join(root, "batches", "*", "loads", "*", "masters", id.String()+".yaml"))
	if len(found) == 0 {
		return Photo{}, "", fmt.Errorf("photo %s: %w", id, os.ErrNotExist)
	}
	if err := readYAML(found[0], &p); err != nil {
		return Photo{}, "", fmt.Errorf("photo %s: %w", id, err)
	}
	return p, strings.TrimSuffix(found[0], ".yaml") + ".tif", nil
}
