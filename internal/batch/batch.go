// Package batch runs one load of the scanner bed as a gobank-workflow
// instance keyed by batch id: describe, preview, find prints, confirm, the
// detail pass, and a bag for each print.
//
// A batch's state lives in its directory under the staging root, not in
// the run: the workflow records only which steps are done, and every step
// reads back what it needs, so a batch resumes after a crash.
package batch

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/plan"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
)

// PreviewPPI is the resolution of the preview prints are found on.
const PreviewPPI = 75

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
	Text    string
	Person  pid.ID // the home person: whose album the photos belong to
	ScanPPI int    // 600, 1200 or 2400
}

// Batch is one load of the bed.
type Batch struct {
	ID          string    `yaml:"id"`
	Description string    `yaml:"description"`
	Person      pid.ID    `yaml:"person"`
	PersonName  string    `yaml:"person_name"`
	ScanPPI     int       `yaml:"scan_ppi"`
	Created     time.Time `yaml:"created"`

	root string
}

// Person is the home of a photo: whose album it belongs to.
type Person struct {
	ID   pid.ID `yaml:"id"`
	Name string `yaml:"name"`
}

// Photo is a bag's photo.yaml: what is known of one print.
type Photo struct {
	ID          pid.ID `yaml:"id"`
	Batch       string `yaml:"batch"`
	Description string `yaml:"description"`
	Home        Person `yaml:"home"`
	Front       Side   `yaml:"front"`
}

// Side is the capture of one side of a print.
type Side struct {
	Captured  time.Time `yaml:"captured"`
	Print     find.Rect `yaml:"print"`  // the print on the bed, mm
	Region    find.Rect `yaml:"region"` // what the master covers, mm
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
	text := strings.Join(strings.Fields(d.Text), " ")
	if text == "" {
		return nil, errors.New("a batch needs a description")
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
	b := &Batch{Description: text, Person: d.Person, PersonName: name, ScanPPI: d.ScanPPI, Created: now, root: root}
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

// Confirm accepts the prints found on the preview.
func (b *Batch) Confirm() error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return fmt.Errorf("batch %s: nothing to confirm yet: %w", b.ID, err)
	}
	if len(f.Prints) == 0 {
		return fmt.Errorf("batch %s: no prints were found on the bed", b.ID)
	}
	return os.WriteFile(b.path("confirmed"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Bed is the bed the preview covers, in mm; zero before the preview.
func (b *Batch) Bed() find.Rect {
	var f found
	readYAML(b.path("prints.yaml"), &f)
	return f.Bed
}

// Adjust moves one edge of print n's region by mm, outwards when mm is
// positive. Regions can be adjusted only until they are confirmed.
func (b *Batch) Adjust(n int, e find.Edge, mm float64) error {
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
func (b *Batch) editable(n int) (found, error) {
	var f found
	if _, err := os.Stat(b.path("confirmed")); err == nil {
		return f, fmt.Errorf("batch %s: the prints are confirmed; go back to the prints to change them", b.ID)
	}
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return f, fmt.Errorf("batch %s: no prints yet: %w", b.ID, err)
	}
	if n < 0 || n >= len(f.Prints) {
		return f, fmt.Errorf("batch %s has no print %d", b.ID, n+1)
	}
	return f, nil
}

// Split replaces print n's region, when it holds prints too close to be
// told apart, with one region for each print within it.
func (b *Batch) Split(n int) error {
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
		cx, cy := (p.Print.X0+p.Print.X1)/2, (p.Print.Y0+p.Print.Y1)/2
		if cx > r.X0 && cx < r.X1 && cy > r.Y0 && cy < r.Y1 {
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
func (b *Batch) Remove(n int) error {
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
func (b *Batch) Reshape(ctx context.Context, runner *wf.Runner, n int, e find.Edge, mm float64) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	if n < 0 || n >= len(f.Prints) {
		return fmt.Errorf("batch %s has no print %d", b.ID, n+1)
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
func (b *Batch) Accept() error {
	if _, err := os.Stat(b.path("masters")); err != nil {
		return fmt.Errorf("batch %s: no masters to accept yet", b.ID)
	}
	return os.WriteFile(b.path("accepted"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Redo sends the batch back to the step named from, which and every later
// step run again on the next tick. A ppi other than 0 changes the batch's
// scan ppi first. What the operator decided at the steps cleared is
// cleared with them: the confirmation, the acceptance, and, when the
// prints are found again, their photo ids.
func (b *Batch) Redo(ctx context.Context, runner *wf.Runner, from string, ppi int) error {
	if ppi != 0 && ppi != 600 && ppi != 1200 && ppi != 2400 {
		return fmt.Errorf("scan ppi %d, want 600, 1200 or 2400", ppi)
	}
	stages := b.Stages(nil, nil)
	if _, err := runner.Rewind(ctx, Workflow, b.ID, from, stages); err != nil {
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
	if ppi != 0 && ppi != b.ScanPPI {
		b.ScanPPI = ppi
		if err := writeYAML(b.path("batch.yaml"), b); err != nil {
			return err
		}
	}
	if cleared["find prints"] {
		if _, ids, err := b.Found(); err == nil {
			for _, id := range ids {
				if err := bag.Release(b.bags(), id); err != nil {
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
	return nil
}

// Run ticks the batch's workflow once: it runs until it completes, fails
// or waits for the operator.
func (b *Batch) Run(ctx context.Context, runner *wf.Runner, s Scanner, now func() time.Time) (*wf.RunRecord, error) {
	date := wf.NewBusinessDate(b.Created.Year(), b.Created.Month(), b.Created.Day())
	return runner.RunPipeline(ctx, Workflow, b.ID, date, b.Stages(s, now))
}

// Stages are the batch's workflow steps, in order.
func (b *Batch) Stages(s Scanner, now func() time.Time) []wf.Stage {
	return []wf.Stage{
		{Name: "describe", Run: func(context.Context) error {
			_, err := os.Stat(b.path("batch.yaml"))
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
		{Name: "check quality", Until: func(context.Context) (bool, error) {
			_, err := os.Stat(b.path("accepted"))
			return err == nil, nil
		}},
		// A bag is never altered, so nothing before packaging can be redone after it.
		{Name: "package", Run: func(context.Context) error { return b.pack(now) }, Final: true},
	}
}

func (b *Batch) bags() string { return filepath.Join(b.root, "bags") }

// reserveIDs gives each confirmed print its photo id, once.
func (b *Batch) reserveIDs(context.Context) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	if len(f.IDs) == len(f.Prints) {
		return nil
	}
	if err := os.MkdirAll(b.bags(), 0o755); err != nil {
		return err
	}
	for len(f.IDs) < len(f.Prints) {
		id, err := bag.Reserve(b.bags())
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

// detailPass scans every region once and writes each master, with its
// photo.yaml, into the batch's masters directory.
func (b *Batch) detailPass(ctx context.Context, s Scanner, now func() time.Time) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	p, err := plan.New(f.Prints, b.ScanPPI)
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
			ID: f.IDs[i], Batch: b.ID, Description: b.Description,
			Home: Person{b.Person, b.PersonName},
			Front: Side{
				Captured: captured, Print: pr.Print, Region: pr.Region,
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

// pack puts each print's master and photo.yaml into its bag, skipping
// bags already complete, then clears the batch's masters.
func (b *Batch) pack(now func() time.Time) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	dir := b.path("masters")
	for _, id := range f.IDs {
		if bag.Complete(b.bags(), id) {
			continue
		}
		bg, err := bag.Create(b.bags(), id)
		if err != nil {
			return err
		}
		for _, file := range [][2]string{{id.String() + ".tif", "front.tif"}, {id.String() + ".yaml", "photo.yaml"}} {
			if err := bg.Add(file[1], copyFrom(filepath.Join(dir, file[0]))); err != nil {
				return err
			}
		}
		if err := bg.Close(bag.Info{Batch: b.ID, Description: b.Description, Bagged: now()}); err != nil {
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
func (b *Batch) Found() ([]find.Print, []pid.ID, error) {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return nil, nil, err
	}
	return f.Prints, f.IDs, nil
}

// FindPhoto is a photo's record and the path of its front master: in its
// bag, or before packaging in its batch.
func FindPhoto(root string, id pid.ID) (Photo, string, error) {
	var p Photo
	data := filepath.Join(root, "bags", id.String(), "data")
	if err := readYAML(filepath.Join(data, "photo.yaml"), &p); err == nil {
		return p, filepath.Join(data, "front.tif"), nil
	}
	found, _ := filepath.Glob(filepath.Join(root, "batches", "*", "masters", id.String()+".yaml"))
	if len(found) == 0 {
		return Photo{}, "", fmt.Errorf("photo %s: %w", id, os.ErrNotExist)
	}
	if err := readYAML(found[0], &p); err != nil {
		return Photo{}, "", fmt.Errorf("photo %s: %w", id, err)
	}
	return p, strings.TrimSuffix(found[0], ".yaml") + ".tif", nil
}
