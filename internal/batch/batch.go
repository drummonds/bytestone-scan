// Package batch runs a batch, one sitting of scanning, as a gobank-workflow
// instance keyed by batch id, and each scan of the scanner bed within it
// as a child instance keyed by the scan: describe, preview, find prints,
// confirm, the detail pass, develop, and an archived photo for each print.
//
// A batch's and a scan's state lives in its directory under the staging
// root, not in the run: the workflow records only which steps are done,
// and every step reads back what it needs, so a scan resumes after a
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
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/plan"
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

// Env is what a scan's workflow runs with: the workflow store, the
// scanner, the archive service (ids, deposits, receipts) and the clock.
type Env struct {
	Store   wf.Store
	Scanner Scanner
	Archive archive.Service
	Now     func() time.Time
}

// Turnable is a scanner that can be told which side of the prints the
// operator has turned to the glass. The real scanner scans whatever lies
// on the bed and has no need of it; the fakes serve that side.
type Turnable interface {
	TurnTo(side string)
}

func turnTo(s Scanner, side string) {
	if t, ok := s.(Turnable); ok {
		t.TurnTo(side)
	}
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
// and scan ppi, over as many scans of the bed as the shoebox needs.
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

// Scan is one placing of prints on the bed within a batch: its description, preview,
// prints, detail pass, masters and the photos archived from them.
type Scan struct {
	Batch       string    `yaml:"batch"`
	N           int       `yaml:"scan"` // counted from 1 within the batch
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
// scan's.
type Photo struct {
	ID          pid.ID `yaml:"id"`
	Batch       string `yaml:"batch"`
	Scan        int    `yaml:"scan,omitempty"`
	Description string `yaml:"description"`
	Home        Person `yaml:"home"`
	Provenance  string `yaml:"provenance,omitempty"`
	Credit      string `yaml:"credit"`
	Front       Side   `yaml:"front"`
	Back        *Side  `yaml:"back,omitempty"` // once the backs are scanned
	// Claims are what the operator said of the photo at its describe
	// step; Described is when, nothing known being an answer too.
	Claims    []Claim    `yaml:"claims,omitempty"`
	Described *time.Time `yaml:"described,omitempty"`
}

// Claim is one thing said about a photo, with where it came from and how
// it was read, as the format statement has it.
type Claim struct {
	Kind        string `yaml:"kind"` // date taken, photographer, back
	Value       string `yaml:"value"`
	Approximate bool   `yaml:"approximate,omitempty"`
	Source      string `yaml:"source"` // back, front: that side of the print; family: a relative's word
	Read        string `yaml:"read"`   // transcribed: by a person
}

// PhotoDescription is what the operator says of a photo: the date taken
// as an ISO 8601 prefix (1952, 1952-07, 1952-07-14), the photographer and
// what the back says, all from Source. Every field may be blank.
type PhotoDescription struct {
	Date         string
	Approximate  bool
	Photographer string
	Back         string // what is written or printed on the back
	Source       string // back, front or family; blank is the back when scanned, else family
}

var isoPrefix = regexp.MustCompile(`^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$`)

// claims are the description's claims, or why it is refused.
func (d PhotoDescription) claims(hasBack bool) ([]Claim, error) {
	source := d.Source
	switch source {
	case "":
		source = "family"
		if hasBack {
			source = "back"
		}
	case "back", "front", "family":
	default:
		return nil, fmt.Errorf("source %q, want back, front or family", d.Source)
	}
	var claims []Claim
	if date := strings.TrimSpace(d.Date); date != "" {
		m := isoPrefix.FindStringSubmatch(date)
		if m == nil || (m[2] != "" && (m[2] < "01" || m[2] > "12")) || (m[3] != "" && (m[3] < "01" || m[3] > "31")) {
			return nil, fmt.Errorf("date %q: want an ISO 8601 prefix such as 1952, 1952-07 or 1952-07-14", d.Date)
		}
		claims = append(claims, Claim{Kind: "date taken", Value: date, Approximate: d.Approximate, Source: source, Read: "transcribed"})
	}
	if p := oneLine(d.Photographer); p != "" {
		claims = append(claims, Claim{Kind: "photographer", Value: p, Source: source, Read: "transcribed"})
	}
	if back := strings.TrimSpace(d.Back); back != "" {
		if !hasBack {
			return nil, errors.New("the back's text: this photo's back was not scanned")
		}
		claims = append(claims, Claim{Kind: "back", Value: back, Source: source, Read: "transcribed"})
	}
	return claims, nil
}

// side is the photo's front or back.
func (p Photo) side(name string) Side {
	if name == "back" && p.Back != nil {
		return *p.Back
	}
	return p.Front
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

// Describe records a new batch under root, its home person looked up in
// the archive service's register, refusing a description the story's
// rules reject. Its id is the day's next: one no batch directory and, when
// store is given, no batch run has taken, so a deleted batch's id is not
// given out again.
func Describe(root string, svc archive.Service, store wf.Store, d Description, now time.Time) (*Batch, error) {
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
	name, err := svc.Person(d.Person)
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
		if store != nil {
			if r, err := store.FindRunByKey(context.Background(), Workflow, b.ID); err != nil {
				return nil, err
			} else if r != nil {
				continue
			}
		}
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

// All are the readable batches under root, newest first.
func All(root string) ([]*Batch, error) {
	entries, err := os.ReadDir(filepath.Join(root, "batches"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []*Batch
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if b, err := Open(root, e.Name()); err == nil {
			all = append(all, b)
		}
	}
	slices.SortFunc(all, func(a, b *Batch) int { return b.Created.Compare(a.Created) })
	return all, nil
}

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

// NewScan adds the next scan of the bed to the batch, described as given
// or, left blank, by the batch's title. A finished batch takes no more.
func (b *Batch) NewScan(description string, now time.Time) (*Scan, error) {
	if b.Finished() {
		return nil, fmt.Errorf("batch %s: scanning is finished", b.ID)
	}
	description = oneLine(description)
	if description == "" {
		description = b.Title
	}
	if err := os.MkdirAll(b.path("scans"), 0o755); err != nil {
		return nil, err
	}
	l := &Scan{Batch: b.ID, Description: description, Created: now, batch: b, root: b.root}
	for l.N = 1; ; l.N++ {
		err := os.Mkdir(l.Dir(), 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	return l, writeYAML(l.path("scan.yaml"), l)
}

// Scans are the batch's scans in order.
func (b *Batch) Scans() ([]*Scan, error) {
	entries, err := os.ReadDir(b.path("scans"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var scans []*Scan
	for _, e := range entries {
		n, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		l, err := b.Scan(n)
		if err != nil {
			return nil, err
		}
		scans = append(scans, l)
	}
	slices.SortFunc(scans, func(a, b *Scan) int { return a.N - b.N })
	return scans, nil
}

// Scan reads the batch's scan n.
func (b *Batch) Scan(n int) (*Scan, error) {
	l := &Scan{batch: b, root: b.root}
	if err := readYAML(filepath.Join(b.path("scans"), strconv.Itoa(n), "scan.yaml"), l); err != nil {
		return nil, fmt.Errorf("scan %s/%d: %w", b.ID, n, err)
	}
	return l, nil
}

// OpenScan reads the scan under root by its key, <batch id>/<n>.
func OpenScan(root, key string) (*Scan, error) {
	id, n, ok := strings.Cut(key, "/")
	num, err := strconv.Atoi(n)
	if !ok || err != nil {
		return nil, fmt.Errorf("scan %q: want <batch id>/<n>", key)
	}
	b, err := Open(root, id)
	if err != nil {
		return nil, err
	}
	return b.Scan(num)
}

// Finished reports whether the operator has ended the batch's scanning.
func (b *Batch) Finished() bool {
	_, err := os.Stat(b.path("finished"))
	return err == nil
}

// Closed is when the batch closed: scanning finished and every photo held
// by the archive or rejected by it, its work files redundant from then.
func (b *Batch) Closed() (time.Time, bool) { return b.marked("closed") }

// Purged is when the batch's work files were deleted.
func (b *Batch) Purged() (time.Time, bool) { return b.marked("purged") }

// marked is when the marker was written, if it was.
func (b *Batch) marked(name string) (time.Time, bool) {
	raw, err := os.ReadFile(b.path(name))
	if err != nil {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
	return at, err == nil
}

func (b *Batch) mark(name string, at time.Time) error {
	return os.WriteFile(b.path(name), []byte(at.UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Finish ends the batch's scanning, once every scan is through, and ticks
// the batch's workflow on.
func (b *Batch) Finish(ctx context.Context, store wf.Store) (*wf.RunRecord, error) {
	scans, err := b.Scans()
	if err != nil {
		return nil, err
	}
	if len(scans) == 0 {
		return nil, fmt.Errorf("batch %s: nothing has been scanned", b.ID)
	}
	for _, l := range scans {
		if done, err := l.done(ctx, store); err != nil {
			return nil, err
		} else if !done {
			return nil, fmt.Errorf("batch %s: scan %d is not through yet", b.ID, l.N)
		}
	}
	if err := os.WriteFile(b.path("finished"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return nil, err
	}
	return b.Run(ctx, store)
}

// Run ticks the batch's workflow once: it waits at scans until scanning
// is finished and every scan is through, then closes.
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
		{Name: "scans", Until: func(context.Context) (bool, error) {
			if !b.Finished() {
				return false, nil
			}
			scans, err := b.Scans()
			if err != nil {
				return false, err
			}
			for _, l := range scans {
				if done, err := l.done(ctx, store); err != nil || !done {
					return false, err
				}
			}
			return true, nil
		}},
		{Name: "close", Run: func(context.Context) error {
			if _, ok := b.Closed(); ok {
				return nil
			}
			return b.mark("closed", time.Now())
		}},
	}
}

// Purger deletes the work files of closed batches: Grace after each
// closed, or sooner, oldest closed first, while staging has less than
// Threshold bytes free. An open batch is never touched. What stays of a
// purged batch is its record, its markers and the archive's answers for
// its photos; a photo the archive rejected keeps its bag too, since the
// archive does not hold its record.
type Purger struct {
	Root      string
	Now       func() time.Time
	Grace     time.Duration
	Threshold uint64
	Free      func(root string) (uint64, error) // bytes free on staging
}

// DefaultGrace and DefaultThreshold are the purger's settings until there
// is a reason for others.
const (
	DefaultGrace     = 30 * 24 * time.Hour
	DefaultThreshold = 10 << 30
)

// Purge sweeps root once with the default purger, returning the batches
// purged.
func Purge(root string) ([]string, error) {
	return (&Purger{Root: root, Now: time.Now, Grace: DefaultGrace, Threshold: DefaultThreshold, Free: freeSpace}).Sweep()
}

// Sweep purges what is due and returns the batches purged.
func (p *Purger) Sweep() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(p.Root, "batches"))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	type closed struct {
		b  *Batch
		at time.Time
	}
	var due []closed
	for _, e := range entries {
		b, err := Open(p.Root, e.Name())
		if err != nil {
			continue
		}
		if _, ok := b.Purged(); ok {
			continue
		}
		if at, ok := b.Closed(); ok {
			due = append(due, closed{b, at})
		}
	}
	slices.SortFunc(due, func(a, b closed) int { return a.at.Compare(b.at) })
	purged := []string{}
	for _, c := range due {
		short := false
		if p.Threshold > 0 && p.Free != nil {
			free, err := p.Free(p.Root)
			if err != nil {
				return purged, err
			}
			short = free < p.Threshold
		}
		if !short && p.Now().Sub(c.at) < p.Grace {
			continue
		}
		if err := p.purge(c.b); err != nil {
			return purged, err
		}
		purged = append(purged, c.b.ID)
	}
	return purged, nil
}

// purge deletes the batch's work files: its scans, and the bags and
// renditions of its photos the archive holds.
func (p *Purger) purge(b *Batch) error {
	scans, err := b.Scans()
	if err != nil {
		return err
	}
	for _, l := range scans {
		_, ids, err := l.Found()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, id := range ids {
			d, err := DepositOf(p.Root, id)
			if err != nil {
				return err
			}
			if d == nil || d.Status != string(archive.Held) {
				continue // the archive does not hold it: its bag stays
			}
			for _, dir := range []string{filepath.Join(p.Root, "bags", id.String()), filepath.Join(p.Root, "renditions", id.String())} {
				if err := os.RemoveAll(dir); err != nil {
					return err
				}
			}
		}
	}
	if err := os.RemoveAll(b.path("scans")); err != nil {
		return err
	}
	return b.mark("purged", p.Now())
}

// freeSpace is the bytes free on the file system holding root.
func freeSpace(root string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}

// Key identifies the scan's workflow instance: <batch id>/<n>.
func (l *Scan) Key() string { return l.Batch + "/" + strconv.Itoa(l.N) }

// Dir is the scan's directory.
func (l *Scan) Dir() string { return filepath.Join(l.batch.Dir(), "scans", strconv.Itoa(l.N)) }

func (l *Scan) path(name string) string { return filepath.Join(l.Dir(), name) }

// Sitting is the batch the scan belongs to.
func (l *Scan) Sitting() *Batch { return l.batch }

// done reports whether the scan's workflow and every one of its photos'
// have completed.
func (l *Scan) done(ctx context.Context, store wf.Store) (bool, error) {
	run, err := store.FindRunByKey(ctx, ScanWorkflow, l.Key())
	if err != nil || run == nil || run.Status != wf.StatusCompleted {
		return false, err
	}
	_, ids, err := l.Found()
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		pr, err := store.FindRunByKey(ctx, PhotoWorkflow, id.String())
		if err != nil || pr == nil || pr.Status != wf.StatusCompleted {
			return false, err
		}
	}
	return true, nil
}

// Confirm accepts the prints found on the preview.
func (b *Scan) Confirm() error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return fmt.Errorf("scan %s: nothing to confirm yet: %w", b.Key(), err)
	}
	if len(f.Prints) == 0 {
		return fmt.Errorf("scan %s: no prints were found on the bed", b.Key())
	}
	return os.WriteFile(b.path("confirmed"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Bed is the bed the preview covers, in mm; zero before the preview.
func (b *Scan) Bed() find.Rect {
	var f found
	readYAML(b.path("prints.yaml"), &f)
	return f.Bed
}

// Adjust moves one edge of print n's region by mm, outwards when mm is
// positive. Regions can be adjusted only until they are confirmed.
func (b *Scan) Adjust(n int, e find.Edge, mm float64) error {
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

// Confirmed reports whether the operator has confirmed the prints found.
func (b *Scan) Confirmed() bool {
	_, err := os.Stat(b.path("confirmed"))
	return err == nil
}

// editable reads the prints for an edit, refusing once they are confirmed
// or when there is no print n.
func (b *Scan) editable(n int) (found, error) {
	var f found
	if b.Confirmed() {
		return f, fmt.Errorf("scan %s: the prints are confirmed; go back to the prints to change them", b.Key())
	}
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return f, fmt.Errorf("scan %s: no prints yet: %w", b.Key(), err)
	}
	if n < 0 || n >= len(f.Prints) {
		return f, fmt.Errorf("scan %s has no print %d", b.Key(), n+1)
	}
	return f, nil
}

// Split replaces print n's region, when it holds prints too close to be
// told apart, with one region for each print within it.
func (b *Scan) Split(n int) error {
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
func (b *Scan) Remove(n int) error {
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
func (b *Scan) Reshape(ctx context.Context, store wf.Store, n int, e find.Edge, mm float64) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	if n < 0 || n >= len(f.Prints) {
		return fmt.Errorf("scan %s has no print %d", b.Key(), n+1)
	}
	if _, err := f.Prints[n].Region.Move(e, mm, f.Bed); err != nil {
		return fmt.Errorf("print %d: %w", n+1, err)
	}
	if err := b.Redo(ctx, store, "confirm", 0); err != nil {
		return err
	}
	if err := b.Adjust(n, e, mm); err != nil {
		return err
	}
	return b.Confirm()
}

// Accept passes the masters at the quality check, once both sides are
// scanned.
func (b *Scan) Accept() error {
	if _, err := os.Stat(b.path("masters")); err != nil {
		return fmt.Errorf("scan %s: no masters to accept yet", b.Key())
	}
	if _, err := os.Stat(b.path("backs.png")); err != nil && !b.BacksSkipped() {
		return fmt.Errorf("scan %s: the backs are not scanned yet", b.Key())
	}
	return os.WriteFile(b.path("accepted"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// Turned records that the operator has turned the prints over in place,
// once the fronts are scanned: the next tick scans the backs.
func (b *Scan) Turned() error {
	if _, err := os.Stat(b.path("masters")); err != nil {
		return fmt.Errorf("scan %s: the fronts are not scanned yet", b.Key())
	}
	return os.WriteFile(b.path("turned"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// NoBacks records that the prints have nothing on their backs, once the
// fronts are scanned: the backs are skipped and each photo goes on
// without one.
func (b *Scan) NoBacks() error {
	if _, err := os.Stat(b.path("masters")); err != nil {
		return fmt.Errorf("scan %s: the fronts are not scanned yet", b.Key())
	}
	return os.WriteFile(b.path("no-backs"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// BacksSkipped reports whether the operator said the backs are blank.
func (b *Scan) BacksSkipped() bool {
	_, err := os.Stat(b.path("no-backs"))
	return err == nil
}

// DescribePhoto records what the operator says of photo id, replacing
// what was said before, until the photo is archived. Saying nothing still
// describes it.
func (b *Scan) DescribePhoto(id pid.ID, d PhotoDescription) error {
	if bag.Complete(b.bags(), id) {
		return fmt.Errorf("photo %s is archived; its record is no longer the scan's to change", id)
	}
	var photo Photo
	if err := readYAML(b.masterPath(id, "yaml"), &photo); err != nil {
		return fmt.Errorf("photo %s: not scanned yet: %w", id, err)
	}
	claims, err := d.claims(photo.Back != nil)
	if err != nil {
		return fmt.Errorf("photo %s: %w", id, err)
	}
	now := time.Now().UTC()
	photo.Claims, photo.Described = claims, &now
	return writeYAML(b.masterPath(id, "yaml"), photo)
}

// Redo sends the scan back to the step named from, which and every later
// step run again on the next tick. A ppi other than 0 changes the batch's
// scan ppi first. What the operator decided at the steps cleared is
// cleared with them: the confirmation, the turning over, the acceptance,
// and, when the prints are found again, their photo ids. The scan's
// photos go back with it: to developing the front when the fronts are
// scanned again, to awaiting the back when the backs are; a photo whose
// prints are found again is orphaned, its run cancelled. Nothing is
// redone once a photo is archived.
func (b *Scan) Redo(ctx context.Context, store wf.Store, from string, ppi int) error {
	if ppi != 0 && ppi != 600 && ppi != 1200 && ppi != 2400 {
		return fmt.Errorf("scan ppi %d, want 600, 1200 or 2400", ppi)
	}
	_, ids, _ := b.Found()
	for _, id := range ids {
		if bag.Complete(b.bags(), id) {
			return fmt.Errorf("scan %s: photo %s is archived and final; nothing before packaging can be redone", b.Key(), id)
		}
	}
	runner := wf.NewRunner(store, nil)
	stages := b.Stages(ctx, Env{Store: store})
	if _, err := runner.Rewind(ctx, ScanWorkflow, b.Key(), from, stages); err != nil {
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
	// the photos' runs follow the scan back
	for _, id := range ids {
		pr, err := store.FindRunByKey(ctx, PhotoWorkflow, id.String())
		if err != nil || pr == nil {
			continue
		}
		switch {
		case cleared["find prints"]:
			if err := store.CancelRun(ctx, pr.ID); err != nil {
				return err
			}
		case cleared["fronts"]:
			if _, err := runner.Rewind(ctx, PhotoWorkflow, id.String(), "develop front", b.photoStages(id, Env{})); err != nil {
				return err
			}
		case cleared["backs"]:
			if _, err := runner.Rewind(ctx, PhotoWorkflow, id.String(), "awaiting back", b.photoStages(id, Env{})); err != nil {
				return err
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
		// the ids stay reserved with the archive: a gap in the series, not a reuse
		if err := os.Remove(b.path("prints.yaml")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for step, markers := range map[string][]string{"confirm": {"confirmed"}, "check quality": {"accepted"}, "fronts": {"masters"}, "backs": {"turned", "no-backs", "backs.png"}} {
		if cleared[step] {
			for _, m := range markers {
				if err := os.RemoveAll(b.path(m)); err != nil {
					return err
				}
			}
		}
	}
	if cleared["backs"] && !cleared["fronts"] {
		for _, id := range ids {
			if err := os.Remove(b.masterPath(id, "back")); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			var photo Photo
			if err := readYAML(b.masterPath(id, "yaml"), &photo); err == nil && photo.Back != nil {
				photo.Back = nil
				if err := writeYAML(b.masterPath(id, "yaml"), photo); err != nil {
					return err
				}
			}
		}
	}
	// a master about to change takes this recipe's renditions of it with
	// it; an older recipe's stay on record
	for _, id := range ids {
		switch {
		case cleared["fronts"]:
			if err := os.RemoveAll(b.Renditions(id)); err != nil {
				return err
			}
		case cleared["backs"]:
			if err := os.RemoveAll(filepath.Join(b.Renditions(id), "back")); err != nil {
				return err
			}
		}
	}
	return nil
}

// masterPath is where the scan keeps one of a photo's files before it is
// archived: its front ("front"), its back ("back") or its record ("yaml").
func (b *Scan) masterPath(id pid.ID, what string) string {
	name := id.String()
	switch what {
	case "back":
		name += ".back.tif"
	case "yaml":
		name += ".yaml"
	default:
		name += ".tif"
	}
	return filepath.Join(b.path("masters"), name)
}

// Run ticks the scan's workflow once: it runs until it completes, fails
// or waits for the operator. The scan runs as a child of its batch's
// workflow, which is ticked first so that it exists to be the parent.
// progress, when given, is told each step as it starts.
func (b *Scan) Run(ctx context.Context, env Env, progress func(step string)) (*wf.RunRecord, error) {
	store := env.Store
	date := wf.NewBusinessDate(b.Created.Year(), b.Created.Month(), b.Created.Day())
	runner := wf.NewRunner(store, nil)
	if _, err := b.batch.Run(ctx, store); err != nil {
		return nil, err
	}
	parent, err := store.FindRunByKey(ctx, Workflow, b.Batch)
	if err != nil || parent == nil {
		return nil, fmt.Errorf("scan %s: no run for batch %s: %v", b.Key(), b.Batch, err)
	}
	if _, err := runner.StartChild(ctx, parent.ID, ScanWorkflow, b.Key(), date); err != nil {
		return nil, err
	}
	watched := func(stages []wf.Stage) []wf.Stage {
		if progress == nil {
			return stages
		}
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
		return stages
	}
	rec, err := runner.RunPipeline(ctx, ScanWorkflow, b.Key(), date, watched(b.Stages(ctx, env)))
	if err != nil {
		return rec, err
	}
	// then each photo the scan has started, as far as it can go
	_, ids, _ := b.Found()
	for _, id := range ids {
		pr, err := store.FindRunByKey(ctx, PhotoWorkflow, id.String())
		if err != nil || pr == nil {
			continue
		}
		if _, err := runner.RunPipeline(ctx, PhotoWorkflow, id.String(), date, watched(b.photoStages(id, env))); err != nil {
			return rec, err
		}
	}
	return rec, nil
}

// photoStages are one photo's workflow steps: each side developed as it
// comes; once the scan's masters are accepted and the photo described,
// it is bagged, deposited with the archive and its receipt taken.
func (b *Scan) photoStages(id pid.ID, env Env) []wf.Stage {
	return []wf.Stage{
		{Name: "develop front", Run: func(context.Context) error { return b.develop(id, "front") }},
		{Name: "awaiting back", Until: func(context.Context) (bool, error) {
			_, err := os.Stat(b.masterPath(id, "back"))
			return err == nil || b.BacksSkipped(), nil
		}},
		{Name: "develop back", Run: func(context.Context) error {
			if _, err := os.Stat(b.masterPath(id, "back")); err != nil && b.BacksSkipped() {
				return nil
			}
			return b.develop(id, "back")
		}},
		{Name: "accepted", Until: func(context.Context) (bool, error) {
			_, err := os.Stat(b.path("accepted"))
			return err == nil, nil
		}},
		{Name: "describe", Until: func(context.Context) (bool, error) {
			var photo Photo
			if err := readYAML(b.masterPath(id, "yaml"), &photo); err != nil {
				return false, err
			}
			return photo.Described != nil, nil
		}},
		// An archived photo is never altered, so nothing before packaging can be redone after it.
		{Name: "package", Run: func(context.Context) error { return b.pack(id, env.Now) }, Final: true},
		{Name: "deposit", Run: func(context.Context) error { return b.deposit(id, env) }},
		{Name: "receipt", Run: func(context.Context) error { return b.receipt(id, env) }},
	}
}

// Deposit is the archive's answer for a photo's bag, kept beside the bag.
type Deposit struct {
	Status      string            `yaml:"status"` // held, or duplicate
	Home        pid.ID            `yaml:"home"`
	Payload     map[string]string `yaml:"payload,omitempty"` // the checksums the archive holds
	DuplicateOf pid.ID            `yaml:"duplicate_of"`      // when a duplicate: the item that holds the picture
	File        string            `yaml:"file,omitempty"`    // when a duplicate: which picture
	At          time.Time         `yaml:"at"`
}

// DepositOf is the archive's answer for photo id under root, or nil when
// it has not been deposited.
func DepositOf(root string, id pid.ID) (*Deposit, error) {
	var d Deposit
	err := readYAML(filepath.Join(root, "bags", id.String()+".deposit.yaml"), &d)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Outcome is how photos stand with the archive: how many it holds, which
// it rejected as duplicates, and how many are not there yet.
type Outcome struct {
	Held     int
	Rejected []pid.ID
	Pending  int
}

// Outcome is how the scan's photos stand with the archive.
func (l *Scan) Outcome() (Outcome, error) {
	var o Outcome
	_, ids, err := l.Found()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return o, err
	}
	for _, id := range ids {
		d, err := DepositOf(l.root, id)
		switch {
		case err != nil:
			return o, err
		case d == nil:
			o.Pending++
		case d.Status == "duplicate":
			o.Rejected = append(o.Rejected, id)
		default:
			o.Held++
		}
	}
	return o, nil
}

// Outcome is how the batch's photos stand with the archive, over all its
// scans.
func (b *Batch) Outcome() (Outcome, error) {
	var o Outcome
	scans, err := b.Scans()
	if err != nil {
		return o, err
	}
	for _, l := range scans {
		lo, err := l.Outcome()
		if err != nil {
			return o, err
		}
		o.Held += lo.Held
		o.Rejected = append(o.Rejected, lo.Rejected...)
		o.Pending += lo.Pending
	}
	return o, nil
}

func (b *Scan) depositPath(id pid.ID) string {
	return filepath.Join(b.bags(), id.String()+".deposit.yaml")
}

// deposit hands the photo's deposit to the archive, a tar of its bag and
// its renditions at the current recipe, and keeps the answer: held, or
// refused as a duplicate of an item the archive holds already, which
// ends the photo. A refusal for the renditions redevelops both sides at
// the current recipe and fails the step, to deposit again; a conflict or
// a failure fails the step, to be tried again.
func (b *Scan) deposit(id pid.ID, env Env) error {
	if d, err := DepositOf(b.root, id); err != nil || d != nil {
		return err
	}
	if err := b.developed(id); err != nil {
		return err
	}
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(bag.Deposit(pw, filepath.Join(b.bags(), id.String()), b.Renditions(id), rendition.Version))
	}()
	rec, err := env.Archive.Deposit(id, pr)
	pr.Close()
	var refusal *archive.Refusal
	switch {
	case err == nil:
		return writeYAML(b.depositPath(id), Deposit{Status: string(rec.Status), Home: rec.Home, Payload: rec.Payload, At: env.Now().UTC()})
	case errors.As(err, &refusal) && refusal.Status == "duplicate":
		return writeYAML(b.depositPath(id), Deposit{Status: "duplicate", Home: refusal.Home, DuplicateOf: refusal.DuplicateOf, File: refusal.File, At: env.Now().UTC()})
	case errors.As(err, &refusal) && refusal.Status == "renditions":
		if rerr := b.redevelop(id); rerr != nil {
			return fmt.Errorf("photo %s: %w; redeveloping: %v", id, err, rerr)
		}
		return fmt.Errorf("photo %s: %w; redeveloped at recipe %d, to deposit again", id, err, rendition.Version)
	}
	return err
}

// redevelop makes the photo's renditions again at the current recipe,
// both sides, whatever was there.
func (b *Scan) redevelop(id pid.ID) error {
	if err := os.RemoveAll(b.Renditions(id)); err != nil {
		return err
	}
	return b.developed(id)
}

// developed sees the photo's renditions are there at the current recipe,
// both sides, making any that are not.
func (b *Scan) developed(id pid.ID) error {
	if err := b.develop(id, "front"); err != nil {
		return err
	}
	if _, err := os.Stat(b.masterOf(id, "back")); err == nil {
		return b.develop(id, "back")
	}
	return nil
}

// masterOf is where the photo's master of kind (front, back, yaml) is: in
// its bag once packaged, among the scan's masters before.
func (b *Scan) masterOf(id pid.ID, kind string) string {
	if bag.Complete(b.bags(), id) {
		return filepath.Join(b.bags(), id.String(), "data", map[string]string{"front": "front.tif", "back": "back.tif", "yaml": "photo.yaml"}[kind])
	}
	return b.masterPath(id, kind)
}

// receipt reads the archive's record of the photo and checks it holds
// the bag, checksum for checksum; a photo refused as a duplicate has no
// receipt to take.
func (b *Scan) receipt(id pid.ID, env Env) error {
	d, err := DepositOf(b.root, id)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("photo %s: not deposited", id)
	}
	if d.Status == "duplicate" {
		return nil
	}
	rec, err := env.Archive.Receipt(id)
	if err != nil {
		return err
	}
	if rec.Status != archive.Held {
		return fmt.Errorf("photo %s: the archive has it %s, not held", id, rec.Status)
	}
	manifest, err := bag.Manifest(filepath.Join(b.bags(), id.String()))
	if err != nil {
		return err
	}
	for file, sum := range manifest {
		if rec.Payload[file] != sum {
			return fmt.Errorf("photo %s: the archive's checksum of %s differs from the bag's", id, file)
		}
	}
	d.Status, d.Home, d.Payload = string(rec.Status), rec.Home, rec.Payload
	return writeYAML(b.depositPath(id), d)
}

// startPhotos starts a photo workflow for each print, under the scan's run.
func (b *Scan) startPhotos(ctx context.Context, store wf.Store) error {
	_, ids, err := b.Found()
	if err != nil {
		return err
	}
	run, err := store.FindRunByKey(ctx, ScanWorkflow, b.Key())
	if err != nil || run == nil {
		return fmt.Errorf("scan %s: no run to start its photos under: %v", b.Key(), err)
	}
	date := wf.NewBusinessDate(b.Created.Year(), b.Created.Month(), b.Created.Day())
	runner := wf.NewRunner(store, nil)
	for _, id := range ids {
		if _, err := runner.StartChild(ctx, run.ID, PhotoWorkflow, id.String(), date); err != nil {
			return err
		}
	}
	return nil
}

// Stages are the scan's workflow steps, in order.
func (b *Scan) Stages(ctx context.Context, env Env) []wf.Stage {
	store, s, now := env.Store, env.Scanner, env.Now
	return []wf.Stage{
		{Name: "describe", Run: func(context.Context) error {
			_, err := os.Stat(b.path("scan.yaml"))
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
			Run: func(context.Context) error { return b.reserveIDs(env.Archive) },
		},
		{Name: "fronts", Run: func(ctx context.Context) error {
			turnTo(s, "front")
			return b.detailPass(ctx, s, now)
		}},
		{Name: "photos", Run: func(context.Context) error { return b.startPhotos(ctx, store) }},
		{Name: "backs",
			Until: func(context.Context) (bool, error) {
				_, err := os.Stat(b.path("turned"))
				return err == nil || b.BacksSkipped(), nil
			},
			Run: func(ctx context.Context) error {
				if b.BacksSkipped() {
					return nil
				}
				turnTo(s, "back")
				return b.backsPass(ctx, s, now)
			},
		},
		{Name: "check quality", Until: func(context.Context) (bool, error) {
			_, err := os.Stat(b.path("accepted"))
			return err == nil, nil
		}},
	}
}

// bags is where the scan's photos are bagged, and kept until the archive
// holds them.
func (b *Scan) bags() string { return filepath.Join(b.root, "bags") }

// reserveIDs gives each confirmed print its photo id, once, from the
// archive service: the next in the collection's series, homed with the
// batch's person.
func (b *Scan) reserveIDs(svc archive.Service) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	if len(f.IDs) == len(f.Prints) {
		return nil
	}
	for len(f.IDs) < len(f.Prints) {
		id, err := svc.Reserve(b.batch.Person)
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
func (b *Scan) pass() (plan.Pass, found, error) {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return plan.Pass{}, f, err
	}
	p, err := plan.New(f.Prints, f.Bed, b.batch.ScanPPI)
	return p, f, err
}

// detailPass scans every region once and writes each front's master, with
// its photo.yaml, into the scan's masters directory.
func (b *Scan) detailPass(ctx context.Context, s Scanner, now func() time.Time) error {
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
			ID: f.IDs[i], Batch: b.Batch, Scan: b.N, Description: b.Description,
			Home: Person{b.batch.Person, b.batch.PersonName}, Provenance: b.batch.Provenance, Credit: b.batch.Credit,
			Front: Side{
				Captured: captured, Print: pr.Print, Region: pr.Region, Bounds: pr.Region.On(f.Bed),
				ScanPPI: p.ScanPPI, MasterPPI: m.PPI, Averaging: m.Factor,
				Skew: pr.Skew, Fill: pr.Fill, Review: pr.Review, Warning: m.Warning,
				Scanner: s.Name(), Settings: s.Settings(),
			},
		}
		if err := writeAtomic(b.masterPath(photo.ID, "front"), func(w io.Writer) error {
			return master.Encode(w, img, master.Options{PPI: m.PPI, XMP: xmp(photo, "front")})
		}); err != nil {
			return err
		}
		if err := writeYAML(b.masterPath(photo.ID, "yaml"), photo); err != nil {
			return err
		}
	}
	return nil
}

// backsPass scans the backs of the prints, turned over in place: a preview
// to see where each back lies, then every front's region once more at
// BackPPI, each back's master written beside its front's at 8 bits.
func (b *Scan) backsPass(ctx context.Context, s Scanner, now func() time.Time) error {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return err
	}
	preview, err := s.Preview(ctx)
	if err != nil {
		return err
	}
	if err := writeAtomic(b.path("backs.png"), func(w io.Writer) error { return png.Encode(w, preview) }); err != nil {
		return err
	}
	backs := find.Prints(preview, PreviewPPI)
	p, err := plan.Backs(f.Prints, f.Bed)
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
	for i, img := range imgs {
		var photo Photo
		if err := readYAML(b.masterPath(f.IDs[i], "yaml"), &photo); err != nil {
			return err
		}
		side := backOf(f.Prints[i], backs, f.Bed)
		side.Captured, side.ScanPPI, side.MasterPPI, side.Averaging = captured, p.ScanPPI, p.Masters[i].PPI, p.Masters[i].Factor
		side.Scanner, side.Settings = s.Name(), s.Settings()
		photo.Back = &side
		if err := writeAtomic(b.masterPath(photo.ID, "back"), func(w io.Writer) error {
			return master.Encode(w, img, master.Options{PPI: side.MasterPPI, Depth: 8, XMP: xmp(photo, "back")})
		}); err != nil {
			return err
		}
		if err := writeYAML(b.masterPath(photo.ID, "yaml"), photo); err != nil {
			return err
		}
	}
	return nil
}

// backTolerance is how far a back's size may differ from its front's, in
// mm, before it is flagged: a print turned over in place keeps its size.
const backTolerance = 5.0

// backOf is the side record of front's back: the print found on the
// backs preview within the front's region, flagged when its size says it
// is not the same print; or, none found, the front's own print mirrored
// in its region, flagged for a look.
func backOf(front find.Print, backs []find.Print, bed find.Rect) Side {
	side := Side{Region: front.Region, Bounds: front.Region.On(bed)}
	var best *find.Print
	for i := range backs {
		c := &backs[i]
		if front.Region.Contains(c.Print.CX, c.Print.CY) && (best == nil || c.Print.W*c.Print.H > best.Print.W*best.Print.H) {
			best = c
		}
	}
	if best == nil {
		m := front.Print
		m.CX, m.Angle = 2*front.Region.CX-m.CX, -m.Angle
		side.Print, side.Review = m, true
		side.Warning = "no print found on the backs preview within the front's region: the back is taken to lie where the front did"
		return side
	}
	side.Print, side.Skew, side.Fill = best.Print, best.Skew, best.Fill
	if math.Abs(best.Print.W-front.Print.W) > backTolerance || math.Abs(best.Print.H-front.Print.H) > backTolerance {
		side.Review = true
		side.Warning = fmt.Sprintf("back %.0f × %.0f mm, front %.0f × %.0f mm: was the print turned over in place?",
			best.Print.W, best.Print.H, front.Print.W, front.Print.H)
	}
	return side
}

// Renditions is where the photo id's renditions at the current recipe
// are kept: beside the archive, never in it.
func (b *Scan) Renditions(id pid.ID) string {
	return filepath.Join(b.root, "renditions", id.String(), strconv.Itoa(rendition.Version))
}

// renditionsOf is where one side's renditions at the current recipe are
// kept: the front's in the photo's directory, the back's under it.
func (b *Scan) renditionsOf(id pid.ID, side string) string {
	if side == "back" {
		return filepath.Join(b.Renditions(id), "back")
	}
	return b.Renditions(id)
}

// develop makes one side's renditions at the current recipe, unless it is
// developed at it already.
func (b *Scan) develop(id pid.ID, side string) error {
	dir := b.renditionsOf(id, side)
	if _, err := rendition.Load(dir); err == nil {
		return nil
	}
	var photo Photo
	if err := readYAML(b.masterOf(id, "yaml"), &photo); err != nil {
		return err
	}
	tif, err := os.ReadFile(b.masterOf(id, side))
	if err != nil {
		return err
	}
	img, err := master.Decode(bytes.NewReader(tif))
	if err != nil {
		return fmt.Errorf("photo %s %s: %w", id, side, err)
	}
	s := photo.side(side)
	r, imgs := rendition.Develop(img, s.Print.InImage(s.Bounds, s.MasterPPI), s.MasterPPI)
	sum := sha256.Sum256(tif)
	r.Master = hex.EncodeToString(sum[:])
	if err := rendition.Save(dir, r, imgs); err != nil {
		return fmt.Errorf("photo %s %s: %w", id, side, err)
	}
	return nil
}

// pack archives the photo's masters and photo.yaml, unless it is complete
// already, then clears them from the scan.
func (b *Scan) pack(id pid.ID, now func() time.Time) error {
	if !bag.Complete(b.bags(), id) {
		ph, err := bag.Create(b.bags(), id)
		if err != nil {
			return err
		}
		for _, file := range [][2]string{{"front", "front.tif"}, {"back", "back.tif"}, {"yaml", "photo.yaml"}} {
			if _, err := os.Stat(b.masterPath(id, file[0])); err != nil && file[0] == "back" {
				continue // a blank back was not scanned: its absence is the record
			}
			if err := ph.Add(file[1], copyFrom(b.masterPath(id, file[0]))); err != nil {
				return err
			}
		}
		if err := ph.Close(bag.Info{Batch: b.Batch, Scan: b.N, Description: b.Description, Bagged: now()}); err != nil {
			return err
		}
	}
	for _, what := range []string{"front", "back", "yaml"} {
		if err := os.Remove(b.masterPath(id, what)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
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
func (b *Scan) Found() ([]find.Print, []pid.ID, error) {
	var f found
	if err := readYAML(b.path("prints.yaml"), &f); err != nil {
		return nil, nil, err
	}
	return f.Prints, f.IDs, nil
}

// FindPhoto is a photo's record and the path of its front master: in its
// archive, or before packaging in its scan.
func FindPhoto(root string, id pid.ID) (Photo, string, error) {
	var p Photo
	data := filepath.Join(root, "bags", id.String(), "data")
	if err := readYAML(filepath.Join(data, "photo.yaml"), &p); err == nil {
		return p, filepath.Join(data, "front.tif"), nil
	}
	found, _ := filepath.Glob(filepath.Join(root, "batches", "*", "scans", "*", "masters", id.String()+".yaml"))
	if len(found) == 0 {
		return Photo{}, "", fmt.Errorf("photo %s: %w", id, os.ErrNotExist)
	}
	if err := readYAML(found[0], &p); err != nil {
		return Photo{}, "", fmt.Errorf("photo %s: %w", id, err)
	}
	return p, strings.TrimSuffix(found[0], ".yaml") + ".tif", nil
}
