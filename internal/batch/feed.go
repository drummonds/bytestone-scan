package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/quality"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// Feeder is the sheet feeder as a feed uses it (story 117): the FF-680W,
// or a stand-in.
type Feeder interface {
	// Feed runs the feeder until it is empty, handing each side as fed, at
	// ppi and 8 bits per channel, to each, numbered from 1 in the order
	// fed: a sheet's front, then its back.
	Feed(ctx context.Context, ppi int, each func(n int, img *master.Image) error) error
	// Name is the feeder's make and model.
	Name() string
	// Settings are the fixed settings of every feed, as recorded with a master.
	Settings() []string
}

// FeedPPI is the resolution every side of a letter is fed at, and
// FeedDepth its bits per channel: the feeder's limit, and FADGI's
// three-star level for documents, as the format statement says.
const (
	FeedPPI   = 600
	FeedDepth = 8
)

// sidePreviewWidth is the width of the small picture of each side the
// arrange form shows.
const sidePreviewWidth = 240

// Feeding makes a scan a feed: one load of the sheet feeder, its bed.
type Feeding struct {
	PPI int `yaml:"ppi"`
}

// Fed is fed.yaml: what one load of the feeder captured.
type Fed struct {
	Scanner  string    `yaml:"scanner"`
	Settings []string  `yaml:"settings"`
	Captured time.Time `yaml:"captured"`
	PPI      int       `yaml:"ppi"`
	Depth    int       `yaml:"depth"`
	Sides    int       `yaml:"sides"`
}

// SheetChoice is what the operator says of one sheet at arrange: which
// letter it is part of, counted from 1, and its role, sheet or envelope.
type SheetChoice struct {
	Letter int    `yaml:"letter"`
	Role   string `yaml:"role"`
}

// Roles a sheet may have.
var Roles = []string{"sheet", "envelope"}

// Arranged is letters.yaml: the letters the feed's sheets make, in letter
// order, each sheet by its number in the feed.
type Arranged struct {
	Letters []ArrangedLetter `yaml:"letters"`
}

// ArrangedLetter is one letter of a feed: its id once reserved and its
// sheets in order.
type ArrangedLetter struct {
	ID     *pid.ID         `yaml:"id,omitempty"`
	Sheets []ArrangedSheet `yaml:"sheets"`
}

// ArrangedSheet is one sheet of a letter: its number in the feed, from
// 1, and its role.
type ArrangedSheet struct {
	N    int    `yaml:"n"`
	Role string `yaml:"role"`
}

// Letter is letter.yaml: the record of a letter as an item (life-weaver
// story 035): its sheets as scanned, both sides of each, and what is
// known of the capture. Its provenance and credit are its batch's, its
// description its feed's.
type Letter struct {
	ID          pid.ID    `yaml:"id"`
	Batch       string    `yaml:"batch"`
	Feed        int       `yaml:"feed"`
	Description string    `yaml:"description"`
	Home        Person    `yaml:"home"`
	Provenance  string    `yaml:"provenance,omitempty"`
	Credit      string    `yaml:"credit"`
	Scanner     string    `yaml:"scanner"`
	Settings    []string  `yaml:"settings"`
	PPI         int       `yaml:"ppi"`
	Depth       int       `yaml:"depth"`
	Scanned     time.Time `yaml:"scanned"`
	Sheets      []Sheet   `yaml:"sheets"`
}

// Sheet is one piece of paper of a letter, kept whole: its role and the
// master of each side.
type Sheet struct {
	Role  string    `yaml:"role"`
	Front SheetSide `yaml:"front"`
	Back  SheetSide `yaml:"back"`
}

// SheetSide is one side of a sheet: its master's file under data/ and
// the turn its renditions are given, quarter turns clockwise in degrees.
type SheetSide struct {
	File string `yaml:"file"`
	Turn int    `yaml:"turn,omitempty"`
}

// Stem is the side's name without its extension: where its renditions
// sit, and how the operator names it.
func (s SheetSide) Stem() string { return stem(s.File) }

func stem(file string) string { return strings.TrimSuffix(file, filepath.Ext(file)) }

// Sides are the letter's sides in sheet order, each with the caption the
// pages give it: "Sheet 1, front", "Envelope, back".
func (r Letter) Sides() []LetterSide {
	var sides []LetterSide
	for i, s := range r.Sheets {
		for _, side := range []struct {
			SheetSide
			name string
		}{{s.Front, "front"}, {s.Back, "back"}} {
			if side.File == "" {
				continue
			}
			caption := fmt.Sprintf("Sheet %d, %s", i+1, side.name)
			if s.Role == "envelope" {
				caption = "Envelope, " + side.name
			}
			sides = append(sides, LetterSide{SheetSide: side.SheetSide, Caption: caption, Role: s.Role})
		}
	}
	return sides
}

// LetterSide is one side of a letter as the pages show it.
type LetterSide struct {
	SheetSide
	Caption string
	Role    string
}

// side is the letter's side named by stem, or nil.
func (r *Letter) side(stem string) *SheetSide {
	for i := range r.Sheets {
		for _, sd := range []*SheetSide{&r.Sheets[i].Front, &r.Sheets[i].Back} {
			if sd.File != "" && sd.Stem() == stem {
				return sd
			}
		}
	}
	return nil
}

// NewFeed adds the next feed to the batch, numbered among its scans: one
// load of the sheet feeder, described as given or, left blank, by the
// batch's title. The sheets are fed when it first runs.
func (b *Batch) NewFeed(description string, now time.Time) (*Scan, error) {
	l, err := b.newScan(description, now)
	if err != nil {
		return nil, err
	}
	l.Feed = &Feeding{PPI: FeedPPI}
	return l, writeYAML(l.path("scan.yaml"), l)
}

// IsFeed reports whether the scan is a feed: its bed the sheet feeder.
func (l *Scan) IsFeed() bool { return l.Feed != nil }

// Type is the scan's workflow type: a feed's is its own, since none of
// its steps is a bed scan's.
func (l *Scan) Type() wf.WorkflowType {
	if l.IsFeed() {
		return FeedWorkflow
	}
	return ScanWorkflow
}

// ItemType is the workflow type of the items the scan archives: letters
// for a feed, photos otherwise.
func (l *Scan) ItemType() wf.WorkflowType { return l.itemType() }

func (l *Scan) itemType() wf.WorkflowType {
	if l.IsFeed() {
		return LetterWorkflow
	}
	return PhotoWorkflow
}

// itemStages are one item's workflow steps, by the scan's kind.
func (l *Scan) itemStages(id pid.ID, env Env) []wf.Stage {
	if l.IsFeed() {
		return l.letterStages(id, env)
	}
	return l.photoStages(id, env)
}

// Accepted reports whether the operator has accepted the scan's masters:
// a feed's letters, a bed scan's photos.
func (l *Scan) Accepted() bool {
	_, err := os.Stat(l.path("accepted"))
	return err == nil
}

// Fed is what the feed captured, once the sheets are fed.
func (l *Scan) Fed() (Fed, error) {
	var f Fed
	if !l.IsFeed() {
		return f, fmt.Errorf("scan %s is not a feed", l.Key())
	}
	if err := readYAML(l.path("fed.yaml"), &f); err != nil {
		return f, fmt.Errorf("feed %s: the sheets are not fed yet: %w", l.Key(), err)
	}
	return f, nil
}

// sidePath is where side n of the feed is kept as fed, an 8-bit TIFF
// without an id; SidePreview the small picture of it the pages show.
func (l *Scan) sidePath(n int) string {
	return l.path(filepath.Join("sheets", fmt.Sprintf("side-%d.tif", n)))
}

// SidePreview is the path of the small picture of side n as fed.
func (l *Scan) SidePreview(n int) string {
	return l.path(filepath.Join("sheets", fmt.Sprintf("side-%d.jpg", n)))
}

// feedSheets runs the feeder until it is empty, keeping each side as fed
// under the feed with a small picture beside it, and records the capture
// in fed.yaml. The feed is duplex, so sides come in pairs.
func (l *Scan) feedSheets(ctx context.Context, env Env) error {
	if env.Feeder == nil {
		return errors.New("no sheet feeder: the scanner runs without the FF-680W")
	}
	dir := l.path("sheets")
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	captured := env.Now().UTC()
	n := 0
	err := env.Feeder.Feed(ctx, l.Feed.PPI, func(k int, img *master.Image) error {
		n = k
		if err := writeAtomic(l.sidePath(k), func(w io.Writer) error {
			return master.Encode(w, img, master.Options{PPI: l.Feed.PPI, Depth: FeedDepth})
		}); err != nil {
			return err
		}
		return writeAtomic(l.SidePreview(k), func(w io.Writer) error {
			return jpeg.Encode(w, quality.Fit(img, sidePreviewWidth), &jpeg.Options{Quality: 80})
		})
	})
	if err != nil {
		return err
	}
	if n%2 != 0 {
		return fmt.Errorf("%d sides fed: the feed is duplex, so sides come in pairs; load the sheets and feed again", n)
	}
	return writeYAML(l.path("fed.yaml"), Fed{Scanner: env.Feeder.Name(), Settings: env.Feeder.Settings(), Captured: captured, PPI: l.Feed.PPI, Depth: FeedDepth, Sides: n})
}

// Sheets are the feed's sheets as offered for arranging: what the
// operator said of each, else every sheet in letter 1, a sheet.
func (l *Scan) Sheets() ([]SheetChoice, error) {
	fed, err := l.Fed()
	if err != nil {
		return nil, err
	}
	choices := make([]SheetChoice, fed.Sides/2)
	for i := range choices {
		choices[i] = SheetChoice{Letter: 1, Role: "sheet"}
	}
	var a Arranged
	if readYAML(l.path("letters.yaml"), &a) == nil {
		for i, lt := range a.Letters {
			for _, s := range lt.Sheets {
				if s.N >= 1 && s.N <= len(choices) {
					choices[s.N-1] = SheetChoice{Letter: i + 1, Role: s.Role}
				}
			}
		}
	}
	return choices, nil
}

// Arrange records which sheets make which letter, and each sheet's
// role: one choice per sheet fed, the letters numbered from 1 without a
// gap. Once the letters have their ids the arrangement is final: a
// mistake means cancelling the feed and feeding again.
func (l *Scan) Arrange(choices []SheetChoice) error {
	fed, err := l.Fed()
	if err != nil {
		return err
	}
	if letters, err := l.Letters(); err == nil {
		for _, lt := range letters {
			if lt.ID != nil {
				return fmt.Errorf("feed %s: the letters have their ids; to arrange the sheets otherwise, cancel the feed and feed them again", l.Key())
			}
		}
	}
	if len(choices) != fed.Sides/2 {
		return fmt.Errorf("feed %s: %d choices for %d sheets", l.Key(), len(choices), fed.Sides/2)
	}
	byLetter := map[int][]ArrangedSheet{}
	for i, c := range choices {
		if !slicesContains(Roles, c.Role) {
			return fmt.Errorf("sheet %d: role %q, want sheet or envelope", i+1, c.Role)
		}
		if c.Letter < 1 {
			return fmt.Errorf("sheet %d: letter %d, want 1 or more", i+1, c.Letter)
		}
		byLetter[c.Letter] = append(byLetter[c.Letter], ArrangedSheet{N: i + 1, Role: c.Role})
	}
	numbers := make([]int, 0, len(byLetter))
	for n := range byLetter {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	var a Arranged
	for i, n := range numbers {
		if n != i+1 {
			return fmt.Errorf("feed %s: the letters are numbered %v, want 1 to %d without a gap", l.Key(), numbers, len(numbers))
		}
		a.Letters = append(a.Letters, ArrangedLetter{Sheets: byLetter[n]})
	}
	return writeYAML(l.path("letters.yaml"), a)
}

func slicesContains(list []string, s string) bool {
	return slices.Contains(list, s)
}

// Letters are the feed's letters as arranged, in letter order, with
// their ids once reserved; an error before the sheets are arranged.
func (l *Scan) Letters() ([]ArrangedLetter, error) {
	a, err := l.arranged()
	return a.Letters, err
}

// arranged is letters.yaml.
func (l *Scan) arranged() (Arranged, error) {
	var a Arranged
	if err := readYAML(l.path("letters.yaml"), &a); err != nil {
		return a, fmt.Errorf("feed %s: not arranged yet: %w", l.Key(), err)
	}
	return a, nil
}

// Arranged reports whether the operator has arranged the sheets.
func (l *Scan) Arranged() bool {
	_, err := os.Stat(l.path("letters.yaml"))
	return err == nil
}

// reserveLetterIDs gives each letter its id, once, from the archive
// service, homed with the batch's person, recorded after each so a crash
// cannot reserve a letter twice.
func (l *Scan) reserveLetterIDs(svc archive.Service) error {
	a, err := l.arranged()
	if err != nil {
		return err
	}
	for i := range a.Letters {
		if a.Letters[i].ID != nil {
			continue
		}
		id, err := svc.Reserve(l.batch.Person)
		if err != nil {
			return err
		}
		a.Letters[i].ID = &id
		if err := writeYAML(l.path("letters.yaml"), a); err != nil {
			return err
		}
	}
	return nil
}

// letterDir is where the letter's masters and letter.yaml are written
// before packaging.
func (l *Scan) letterDir(id pid.ID) string { return l.path(filepath.Join("masters", id.String())) }

// letterFileOf is where one of the letter's files is: in its bag once
// packaged, in the feed's masters before.
func (l *Scan) letterFileOf(id pid.ID, name string) string {
	if bag.Complete(l.bags(), id) {
		return filepath.Join(l.bags(), id.String(), "data", name)
	}
	return filepath.Join(l.letterDir(id), name)
}

// writeLetters writes each letter's masters, the sides as fed given their
// id and side in XMP, and letter.yaml beside them; a letter written, or
// packaged, already is left as it is.
func (l *Scan) writeLetters() error {
	letters, err := l.Letters()
	if err != nil {
		return err
	}
	fed, err := l.Fed()
	if err != nil {
		return err
	}
	for _, lt := range letters {
		if lt.ID == nil {
			return fmt.Errorf("feed %s: a letter has no id yet", l.Key())
		}
		id := *lt.ID
		if _, err := os.Stat(filepath.Join(l.letterDir(id), "letter.yaml")); err == nil || bag.Complete(l.bags(), id) {
			continue
		}
		rec := Letter{
			ID: id, Batch: l.Batch, Feed: l.N, Description: l.Description,
			Home: Person{l.batch.Person, l.batch.PersonName}, Provenance: l.batch.Provenance, Credit: l.batch.Credit,
			Scanner: fed.Scanner, Settings: fed.Settings, PPI: fed.PPI, Depth: fed.Depth, Scanned: fed.Captured,
		}
		for i, s := range lt.Sheets {
			rec.Sheets = append(rec.Sheets, Sheet{Role: s.Role,
				Front: SheetSide{File: fmt.Sprintf("sheet-%02d-front.tif", i+1)},
				Back:  SheetSide{File: fmt.Sprintf("sheet-%02d-back.tif", i+1)}})
		}
		if err := os.MkdirAll(l.letterDir(id), 0o755); err != nil {
			return err
		}
		for i, s := range lt.Sheets {
			for _, side := range []struct {
				n    int
				file string
			}{{2*s.N - 1, rec.Sheets[i].Front.File}, {2 * s.N, rec.Sheets[i].Back.File}} {
				raw, err := os.ReadFile(l.sidePath(side.n))
				if err != nil {
					return err
				}
				img, err := master.Decode(bytes.NewReader(raw))
				if err != nil {
					return fmt.Errorf("side %d: %w", side.n, err)
				}
				if err := writeAtomic(filepath.Join(l.letterDir(id), side.file), func(w io.Writer) error {
					return master.Encode(w, img, master.Options{PPI: fed.PPI, Depth: fed.Depth, XMP: xmpLetter(rec, stem(side.file), s.Role)})
				}); err != nil {
					return err
				}
			}
		}
		if err := writeYAML(filepath.Join(l.letterDir(id), "letter.yaml"), rec); err != nil {
			return err
		}
	}
	return nil
}

// startLetters starts a letter workflow for each letter, under the feed's run.
func (l *Scan) startLetters(ctx context.Context, store wf.Store) error {
	ids, err := l.Items()
	if err != nil {
		return err
	}
	run, err := store.FindRunByKey(ctx, FeedWorkflow, l.Key())
	if err != nil || run == nil {
		return fmt.Errorf("feed %s: no run to start its letters under: %v", l.Key(), err)
	}
	date := wf.NewBusinessDate(l.Created.Year(), l.Created.Month(), l.Created.Day())
	runner := wf.NewRunner(store, nil)
	for _, id := range ids {
		if _, err := runner.StartChild(ctx, run.ID, LetterWorkflow, id.String(), date); err != nil {
			return err
		}
	}
	return nil
}

// feedStages are the feed's workflow steps, in order.
func (l *Scan) feedStages(ctx context.Context, env Env) []wf.Stage {
	return []wf.Stage{
		{Name: "describe", Run: func(context.Context) error {
			_, err := os.Stat(l.path("scan.yaml"))
			return err
		}},
		{Name: "feed", Run: func(ctx context.Context) error { return l.feedSheets(ctx, env) }},
		{Name: "arrange",
			Until: func(context.Context) (bool, error) { return l.Arranged(), nil },
			Run:   func(context.Context) error { return l.reserveLetterIDs(env.Archive) },
		},
		{Name: "masters", Run: func(context.Context) error { return l.writeLetters() }},
		{Name: "letters", Run: func(context.Context) error { return l.startLetters(ctx, env.Store) }},
		{Name: "check quality", Until: func(context.Context) (bool, error) {
			_, err := os.Stat(l.path("accepted"))
			return err == nil, nil
		}},
	}
}

// letterStages are one letter's workflow steps: every side developed,
// then, once the feed is accepted, bagged, deposited and receipted.
func (l *Scan) letterStages(id pid.ID, env Env) []wf.Stage {
	return []wf.Stage{
		{Name: "develop", Run: func(context.Context) error { return l.developLetter(id) }},
		{Name: "accepted", Until: func(context.Context) (bool, error) {
			_, err := os.Stat(l.path("accepted"))
			return err == nil, nil
		}},
		// An archived letter is never altered, so nothing before packaging can be redone after it.
		{Name: "package", Run: func(context.Context) error { return l.packLetter(id, env.Now) }, Final: true},
		{Name: "deposit", Run: func(context.Context) error {
			return l.depositItem(id, env, func(again bool) error {
				if again {
					if err := os.RemoveAll(l.Renditions(id)); err != nil {
						return err
					}
				}
				return l.developLetter(id)
			})
		}},
		{Name: "receipt", Run: func(context.Context) error { return l.receipt(id, env) }},
	}
}

// readLetter is the letter's record, from its bag or its feed.
func (l *Scan) readLetter(id pid.ID) (Letter, error) {
	var rec Letter
	if err := readYAML(l.letterFileOf(id, "letter.yaml"), &rec); err != nil {
		return rec, fmt.Errorf("letter %s: not written yet: %w", id, err)
	}
	return rec, nil
}

// developLetter sees every side's renditions are there at the current
// recipe, with the side's turn, making any that are not.
func (l *Scan) developLetter(id pid.ID) error {
	rec, err := l.readLetter(id)
	if err != nil {
		return err
	}
	for _, s := range rec.Sides() {
		if err := l.developSide(id, s.SheetSide); err != nil {
			return err
		}
	}
	return nil
}

// developSide makes one side's renditions: the whole side fit to the
// screen, turned as the operator said, nothing cut or levelled, since
// the sheet is kept whole and its pages are cut later from these.
func (l *Scan) developSide(id pid.ID, sd SheetSide) error {
	dir := filepath.Join(l.Renditions(id), sd.Stem())
	if r, err := rendition.Load(dir); err == nil && r.Turn == sd.Turn {
		return nil
	}
	tif, err := os.ReadFile(l.letterFileOf(id, sd.File))
	if err != nil {
		return err
	}
	img, err := master.Decode(bytes.NewReader(tif))
	if err != nil {
		return fmt.Errorf("letter %s %s: %w", id, sd.Stem(), err)
	}
	r, imgs := rendition.Whole(quarterTurned(sideRGBA(img), sd.Turn))
	r.Turn = ((sd.Turn % 360) + 360) % 360
	sum := sha256.Sum256(tif)
	r.Master = hex.EncodeToString(sum[:])
	if err := rendition.Save(dir, r, imgs); err != nil {
		return fmt.Errorf("letter %s %s: %w", id, sd.Stem(), err)
	}
	return nil
}

// sideRGBA is the master as an 8-bit picture: the high byte of each
// sample, which for an 8-bit master is the sample.
func sideRGBA(m *master.Image) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, m.W, m.H))
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			i := (y*m.W + x) * 3
			out.Pix[y*out.Stride+x*4] = uint8(m.Pix[i] >> 8)
			out.Pix[y*out.Stride+x*4+1] = uint8(m.Pix[i+1] >> 8)
			out.Pix[y*out.Stride+x*4+2] = uint8(m.Pix[i+2] >> 8)
			out.Pix[y*out.Stride+x*4+3] = 0xff
		}
	}
	return out
}

// TurnSide adds by, a quarter turn clockwise (90) or anticlockwise
// (-90), to the turn of letter id's side named by stem, and makes that
// side's renditions again. The turn is the operator's word on which way
// up the side is, said before the feed is accepted: once accepted the
// letter's record is packaged, so it is refused from then.
func (l *Scan) TurnSide(id pid.ID, stem string, by int) error {
	if by != 90 && by != -90 {
		return fmt.Errorf("turn by %d°: a turn is a quarter turn, 90 or -90", by)
	}
	if _, err := os.Stat(l.path("accepted")); err == nil || bag.Complete(l.bags(), id) {
		return fmt.Errorf("letter %s is accepted; its record is no longer the feed's to change", id)
	}
	rec, err := l.readLetter(id)
	if err != nil {
		return err
	}
	sd := rec.side(stem)
	if sd == nil {
		return fmt.Errorf("letter %s has no side %q", id, stem)
	}
	sd.Turn = turned(sd.Turn, by)
	if err := writeYAML(l.letterFileOf(id, "letter.yaml"), rec); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(l.Renditions(id), stem)); err != nil {
		return err
	}
	return l.developSide(id, *sd)
}

// packLetter archives the letter's masters and letter.yaml, unless it is
// complete already, then clears them from the feed.
func (l *Scan) packLetter(id pid.ID, now func() time.Time) error {
	if !bag.Complete(l.bags(), id) {
		rec, err := l.readLetter(id)
		if err != nil {
			return err
		}
		ph, err := bag.Create(l.bags(), id)
		if err != nil {
			return err
		}
		for _, s := range rec.Sides() {
			if err := ph.Add(s.File, copyFrom(filepath.Join(l.letterDir(id), s.File))); err != nil {
				return err
			}
		}
		if err := ph.Add("letter.yaml", copyFrom(filepath.Join(l.letterDir(id), "letter.yaml"))); err != nil {
			return err
		}
		if err := ph.Close(bag.Info{Batch: l.Batch, Scan: l.N, Description: l.Description, Bagged: now()}); err != nil {
			return err
		}
	}
	return os.RemoveAll(l.letterDir(id))
}

// Items are the items the scan archives of its own kind: a feed's
// letters, once they have ids, else the photos of its prints; none
// before they are known.
func (l *Scan) Items() ([]pid.ID, error) {
	if l.IsFeed() {
		letters, err := l.Letters()
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var ids []pid.ID
		for _, lt := range letters {
			if lt.ID != nil {
				ids = append(ids, *lt.ID)
			}
		}
		return ids, nil
	}
	_, ids, err := l.Found()
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return ids, err
}

// FindLetter is a letter's record and the directory its files are in:
// its bag, or before packaging its feed.
func FindLetter(root string, id pid.ID) (Letter, string, error) {
	var rec Letter
	data := filepath.Join(root, "bags", id.String(), "data")
	if err := readYAML(filepath.Join(data, "letter.yaml"), &rec); err == nil {
		return rec, data, nil
	}
	found, _ := filepath.Glob(filepath.Join(root, "batches", "*", "scans", "*", "masters", id.String(), "letter.yaml"))
	if len(found) == 0 {
		return Letter{}, "", fmt.Errorf("letter %s: %w", id, os.ErrNotExist)
	}
	if err := readYAML(found[0], &rec); err != nil {
		return Letter{}, "", fmt.Errorf("letter %s: %w", id, err)
	}
	return rec, filepath.Dir(found[0]), nil
}

// LetterStanding is where one letter's own run stands.
func LetterStanding(ctx context.Context, store wf.Store, id pid.ID) (Standing, error) {
	return standingOf(ctx, store, LetterWorkflow, id.String())
}

// xmpLetter is the packet embedded in one side of a letter: only what is
// fixed at capture.
func xmpLetter(r Letter, stem, role string) []byte {
	esc := func(s string) string {
		var b strings.Builder
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var settings strings.Builder
	for _, s := range r.Settings {
		fmt.Fprintf(&settings, "<rdf:li>%s</rdf:li>", esc(s))
	}
	return []byte(`<?xpacket begin="` + "\ufeff" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:xmp="http://ns.adobe.com/xap/1.0/"
    xmlns:bs="https://bytestone.uk/ns/scan/1.0/">
   <dc:identifier>` + r.ID.String() + `</dc:identifier>
   <dc:rights>` + esc(r.Credit) + `</dc:rights>
   <xmp:CreateDate>` + r.Scanned.UTC().Format(time.RFC3339) + `</xmp:CreateDate>
   <xmp:CreatorTool>bytestone-scan</xmp:CreatorTool>
   <bs:side>` + esc(stem) + `</bs:side>
   <bs:role>` + esc(role) + `</bs:role>
   <bs:batch>` + esc(r.Batch) + `</bs:batch>
   <bs:feed>` + fmt.Sprint(r.Feed) + `</bs:feed>
   <bs:scanner>` + esc(r.Scanner) + `</bs:scanner>
   <bs:scanPPI>` + fmt.Sprint(r.PPI) + `</bs:scanPPI>
   <bs:masterPPI>` + fmt.Sprint(r.PPI) + `</bs:masterPPI>
   <bs:depth>` + fmt.Sprint(r.Depth) + `</bs:depth>
   <bs:settings><rdf:Bag>` + settings.String() + `</rdf:Bag></bs:settings>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="r"?>`)
}
