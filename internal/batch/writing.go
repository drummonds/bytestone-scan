package batch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/writing"
)

// WritingRoles are the parts a writing names a person in.
var WritingRoles = []string{"author", "subject"}

// TransmissionParts are the parts a hand may have had in a writing's
// reaching the archive.
var TransmissionParts = []string{"transcribed", "typed", "edited", "kept", "sent"}

// WrittenSources are where a writing's written date may come from: a
// relative's word, or the writing itself.
var WrittenSources = []string{"family", "writing"}

// WritingImport is what the operator brings in: one piece of writing as
// received, a Word document or plain text.
type WritingImport struct {
	File  string // the name as sent
	Title string // blank for the file's name without its extension
	Data  io.Reader
}

// ReceivedFile is a writing's file as it came, for the record only: the
// file itself is let go once the bag is written.
type ReceivedFile struct {
	Name    string          `yaml:"name"`
	Format  string          `yaml:"format"`
	Size    int64           `yaml:"size"`
	SHA256  string          `yaml:"sha256"`
	LeftOut writing.LeftOut `yaml:"left_out"`
}

// By is whose hand a writing passed through: a person of the register,
// or a name as written where the register does not hold them. In the
// record it is the one scalar, the id as pid writes it or the name, as
// life-weaver reads it.
type By struct {
	Person pid.ID
	Name   string
}

func (b By) MarshalYAML() (any, error) {
	if b.Name != "" {
		return b.Name, nil
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Value: b.Person.String()}, nil
}

func (b *By) UnmarshalYAML(n *yaml.Node) error {
	if id, ok := readsAsID(n.Value); ok {
		*b = By{Person: id}
		return nil
	}
	*b = By{Name: n.Value}
	return nil
}

// readsAsID is whether the text is a person's id exactly as pid writes
// it, which is how a record's reader tells an id from a name.
func readsAsID(s string) (pid.ID, bool) {
	id, err := pid.Parse(s)
	return id, err == nil && id.String() == s
}

// Hand is one of the hands a writing's text passed through to reach the
// archive: its part, by whom, when, with a note.
type Hand struct {
	Part string `yaml:"part"`
	By   By     `yaml:"by"`
	Date string `yaml:"date,omitempty"`
	Note string `yaml:"note,omitempty"`
}

// HandChoice is one hand as the operator gives it: a person picked, by
// their id, or a name typed; one or the other.
type HandChoice struct {
	Part, Person, Name, Date, Note string
}

// Written is a writing import's own record, kept in scan.yaml: the file
// as received and what was left out of it, the title, then what the
// operator says of it, and the writing's id once reserved.
type Written struct {
	Received ReceivedFile `yaml:"received"`
	Title    string       `yaml:"title"`
	// Read is when the operator, having read the page, went on.
	Read         *time.Time `yaml:"read,omitempty"`
	People       []Role     `yaml:"people,omitempty"`
	Written      *Made      `yaml:"written,omitempty"`
	Transmission []Hand     `yaml:"transmission,omitempty"`
	Described    *time.Time `yaml:"described,omitempty"`
	ID           *pid.ID    `yaml:"id,omitempty"`
}

// WritingDescription is what the operator says of a writing: who it
// names and in what part, when it was written, and the hands it passed
// through, in order. Nothing is required.
type WritingDescription struct {
	People       []RoleChoice
	Written      MadeClaim
	Transmission []HandChoice
}

// WritingRecord is writing.yaml: the record of a writing as an item
// (life-weaver stories 288, 336). Its home, provenance and credit are
// its batch's.
type WritingRecord struct {
	ID           pid.ID       `yaml:"id"`
	Batch        string       `yaml:"batch"`
	Writing      int          `yaml:"writing"`
	Title        string       `yaml:"title"`
	Home         Person       `yaml:"home"`
	Provenance   string       `yaml:"provenance,omitempty"`
	Credit       string       `yaml:"credit"`
	People       []Role       `yaml:"people,omitempty"`
	Written      *Made        `yaml:"written,omitempty"`
	Transmission []Hand       `yaml:"transmission,omitempty"`
	Received     ReceivedFile `yaml:"received"`
}

// NewWriting adds a writing import to the batch, numbered among its
// scans: the file is written to staging as it arrives and converted to
// djot, both kept under the import until the writing is bagged. The
// title starts as the file's name. What cannot be converted is refused
// and nothing of it kept.
func (b *Batch) NewWriting(in WritingImport, now time.Time) (*Scan, error) {
	if in.File == "" || in.Data == nil {
		return nil, errors.New("nothing to import: choose a file")
	}
	if b.Finished() {
		return nil, fmt.Errorf("batch %s: scanning is finished", b.ID)
	}
	name := in.File
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	format, err := writing.Format(name)
	if err == nil && strings.TrimSuffix(name, filepath.Ext(name)) == "" {
		err = writing.ErrFormat
	}
	if err != nil {
		return nil, fmt.Errorf("file name %q: %w", name, err)
	}
	tmp, err := os.CreateTemp(b.Dir(), "writing-*.upload")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		tmp.Close()
		if !keep {
			os.Remove(tmp.Name())
		}
	}()
	sum := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, sum), in.Data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	text, err := writing.Convert(name, tmp, size)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	title := oneLine(in.Title)
	if title == "" {
		title = oneLine(strings.TrimSuffix(name, filepath.Ext(name)))
	}
	l, err := b.newScan(title, now)
	if err != nil {
		return nil, err
	}
	l.Writing = &Written{Title: title, Received: ReceivedFile{Name: name, Format: format, Size: size, SHA256: hex.EncodeToString(sum.Sum(nil)), LeftOut: text.LeftOut}}
	if err := os.Mkdir(l.path("writing"), 0o755); err != nil {
		return nil, err
	}
	if err := writeAtomic(l.writingDjot(), func(w io.Writer) error { _, err := w.Write(text.Djot); return err }); err != nil {
		return nil, err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), l.path(filepath.Join("writing", "received."+format))); err != nil {
		return nil, err
	}
	keep = true
	return l, writeYAML(l.path("scan.yaml"), l)
}

// IsWriting reports whether the scan is a writing import.
func (l *Scan) IsWriting() bool { return l.Writing != nil }

// writingDjot is where the import keeps the djot until it is bagged.
func (l *Scan) writingDjot() string { return l.path(filepath.Join("writing", "writing.dj")) }

// writingPackaged reports whether the writing is bagged: final.
func (l *Scan) writingPackaged() bool {
	return l.Writing.ID != nil && bag.Complete(l.bags(), *l.Writing.ID)
}

// WritingText is the writing's djot: from its bag once packaged, from
// the import before.
func (l *Scan) WritingText() ([]byte, error) {
	if !l.IsWriting() {
		return nil, fmt.Errorf("scan %s is not a writing import", l.Key())
	}
	if l.writingPackaged() {
		return os.ReadFile(filepath.Join(l.bags(), l.Writing.ID.String(), "data", "writing.dj"))
	}
	return os.ReadFile(l.writingDjot())
}

// writingOpen is nil while the import's record is the operator's to
// change: a writing import, and not yet bagged.
func (l *Scan) writingOpen() error {
	switch {
	case !l.IsWriting():
		return fmt.Errorf("scan %s is not a writing import", l.Key())
	case l.writingPackaged():
		return fmt.Errorf("writing %s is packaged; its record is no longer the import's to change", l.Writing.ID)
	}
	return nil
}

// ConfirmWriting records that the operator has read the page, that the
// right file came and came whole, and goes on, under the title given:
// a writing has one. The djot is not edited here.
func (l *Scan) ConfirmWriting(title string) error {
	if err := l.writingOpen(); err != nil {
		return err
	}
	if title = oneLine(title); title == "" {
		return errors.New("a writing has a title")
	}
	at := time.Now().UTC()
	l.Writing.Title, l.Writing.Read = title, &at
	l.Description = title
	return writeYAML(l.path("scan.yaml"), l)
}

// DescribeWriting records who the writing names and in what part, each a
// person of the register; when it was written; and its transmission,
// each hand a person picked or a name typed, in the order given. Nothing
// known is an answer. Refused before the writing is read and once it is
// bagged.
func (l *Scan) DescribeWriting(svc archive.Service, d WritingDescription, now time.Time) error {
	if err := l.writingOpen(); err != nil {
		return err
	}
	if l.Writing.Read == nil {
		return fmt.Errorf("writing import %s: read it and confirm its title first", l.Key())
	}
	person := func(what, who string) (pid.ID, error) {
		id, err := pid.Parse(who)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", what, err)
		}
		if id == 0 {
			return 0, fmt.Errorf("%s: %w", what, ErrNoID)
		}
		if _, err := svc.Person(id); err != nil {
			return 0, fmt.Errorf("%s: %w", what, err)
		}
		return id, nil
	}
	var people []Role
	for _, c := range d.People {
		role, who := strings.TrimSpace(c.Role), strings.TrimSpace(c.Person)
		if role == "" && who == "" {
			continue
		}
		if !slices.Contains(WritingRoles, role) {
			return fmt.Errorf("role %q: want one of %s", c.Role, strings.Join(WritingRoles, ", "))
		}
		if who == "" {
			return fmt.Errorf("a part needs its person: choose who the %s is", role)
		}
		id, err := person("the "+role, who)
		if err != nil {
			return err
		}
		people = append(people, Role{Person: id, Role: role})
	}
	var written *Made
	if value := strings.TrimSpace(d.Written.Value); value != "" {
		if !isISOPrefix(value) {
			return fmt.Errorf("written %q: want an ISO 8601 prefix such as 1974, 1974-06 or 1974-06-14", d.Written.Value)
		}
		source := d.Written.Source
		if source == "" {
			source = "family"
		}
		if !slices.Contains(WrittenSources, source) {
			return fmt.Errorf("source %q, want %s", d.Written.Source, strings.Join(WrittenSources, " or "))
		}
		written = &Made{Value: value, Approximate: d.Written.Approximate, Source: source}
	}
	var hands []Hand
	for _, c := range d.Transmission {
		h := Hand{Part: strings.TrimSpace(c.Part), Date: strings.TrimSpace(c.Date), Note: oneLine(c.Note)}
		who, name := strings.TrimSpace(c.Person), oneLine(c.Name)
		if h.Part == "" && who == "" && name == "" && h.Date == "" && h.Note == "" {
			continue
		}
		switch {
		case !slices.Contains(TransmissionParts, h.Part):
			return fmt.Errorf("a hand's part %q: want one of %s", c.Part, strings.Join(TransmissionParts, ", "))
		case who != "" && name != "":
			return fmt.Errorf("who %s it: a person picked or a name typed, one or the other, not both", h.Part)
		case who == "" && name == "":
			return fmt.Errorf("a hand needs a name: pick the person who %s it, or type their name", h.Part)
		case h.Date != "" && !isISOPrefix(h.Date):
			return fmt.Errorf("%s on %q: want an ISO 8601 prefix such as 1998, 1998-06 or 1998-06-14", h.Part, c.Date)
		}
		if who != "" {
			id, err := person("who "+h.Part+" it", who)
			if err != nil {
				return err
			}
			h.By = By{Person: id}
		} else {
			if _, ok := readsAsID(name); ok {
				return fmt.Errorf("the name %q reads as a person's id: pick the person, or write the name as it is written, %q", name, strings.ToUpper(name[:1])+name[1:])
			}
			h.By = By{Name: name}
		}
		hands = append(hands, h)
	}
	at := now.UTC()
	l.Writing.People, l.Writing.Written, l.Writing.Transmission, l.Writing.Described = people, written, hands, &at
	return writeYAML(l.path("scan.yaml"), l)
}

// written reads the import's record afresh: a step sees what the
// operator has said since the scan was opened.
func (l *Scan) written() (*Written, error) {
	fresh, err := l.batch.Scan(l.N)
	if err != nil {
		return nil, err
	}
	if fresh.Writing == nil {
		return nil, fmt.Errorf("scan %s is not a writing import", l.Key())
	}
	l.Writing, l.Description = fresh.Writing, fresh.Description
	return l.Writing, nil
}

// reserveWritingID gives the writing its id, once, homed with the batch's person.
func (l *Scan) reserveWritingID(svc archive.Service) error {
	w, err := l.written()
	if err != nil || w.ID != nil {
		return err
	}
	id, err := reserve(svc, l.root, l.batch.Person)
	if err != nil {
		return err
	}
	w.ID = &id
	return writeYAML(l.path("scan.yaml"), l)
}

// startWriting starts the writing's workflow under the import's run.
func (l *Scan) startWriting(ctx context.Context, store wf.Store) error {
	w, err := l.written()
	if err != nil {
		return err
	}
	if w.ID == nil {
		return fmt.Errorf("writing import %s: the writing has no id yet", l.Key())
	}
	run, err := store.FindRunByKey(ctx, WritingImportWorkflow, l.Key())
	if err != nil || run == nil {
		return fmt.Errorf("writing import %s: no run to start its writing under: %v", l.Key(), err)
	}
	date := wf.NewBusinessDate(l.Created.Year(), l.Created.Month(), l.Created.Day())
	_, err = wf.NewRunner(store, nil).StartChild(ctx, run.ID, WritingWorkflow, w.ID.String(), date)
	return err
}

// writingImportStages are the writing import's workflow steps, in order.
func (l *Scan) writingImportStages(env Env) []wf.Stage {
	return []wf.Stage{
		{Name: "import", Run: func(context.Context) error {
			_, err := os.Stat(l.path("scan.yaml"))
			return err
		}},
		{Name: "read", Until: func(context.Context) (bool, error) {
			w, err := l.written()
			return err == nil && w.Read != nil, err
		}},
		{Name: "describe",
			Until: func(context.Context) (bool, error) {
				w, err := l.written()
				return err == nil && w.Described != nil, err
			},
			Run: func(context.Context) error { return l.reserveWritingID(env.Archive) },
		},
		{Name: "writing", Run: func(ctx context.Context) error { return l.startWriting(ctx, env.Store) }},
	}
}

// writingStages are the writing's workflow steps: bagged, deposited and
// receipted. It has no renditions to make.
func (l *Scan) writingStages(id pid.ID, env Env) []wf.Stage {
	return []wf.Stage{
		// An archived writing is never altered, so nothing before packaging can be redone after it.
		{Name: "package", Run: func(context.Context) error { return l.packWriting(id, env.Now) }, Final: true},
		{Name: "deposit", Run: func(context.Context) error {
			return l.depositItem(id, env, func(bool) error { return nil })
		}},
		{Name: "receipt", Run: func(context.Context) error {
			if err := l.receipt(id, env); err != nil {
				return err
			}
			d, err := DepositOf(l.root, id)
			if err != nil {
				return err
			}
			if d != nil && d.Status != "duplicate" && d.Kind != "writing" {
				return fmt.Errorf("writing %s: the archive holds it as %q, not as a writing", id, d.Kind)
			}
			return nil
		}},
	}
}

// writingRecord is writing.yaml for the writing id.
func (l *Scan) writingRecord(id pid.ID) WritingRecord {
	b, w := l.batch, l.Writing
	return WritingRecord{ID: id, Batch: l.Batch, Writing: l.N, Title: w.Title,
		Home: Person{b.Person, b.PersonName}, Provenance: b.Provenance, Credit: b.Credit,
		People: w.People, Written: w.Written, Transmission: w.Transmission, Received: w.Received}
}

// packWriting bags the writing: its djot and writing.yaml, and nothing
// else. The file received is then let go: the import's copy of it and of
// the djot are cleared once the bag holds the djot.
func (l *Scan) packWriting(id pid.ID, now func() time.Time) error {
	if _, err := l.written(); err != nil {
		return err
	}
	if !bag.Complete(l.bags(), id) {
		rec := l.writingRecord(id)
		ph, err := bag.Create(l.bags(), id)
		if err != nil {
			return err
		}
		if err := ph.Add("writing.dj", copyFrom(l.writingDjot())); err != nil {
			return err
		}
		if err := ph.Add("writing.yaml", func(w io.Writer) error { return encodeYAML(w, rec) }); err != nil {
			return err
		}
		if err := ph.Close(bag.Info{Batch: l.Batch, Scan: l.N, Description: l.Description, Bagged: now()}); err != nil {
			return err
		}
	}
	return os.RemoveAll(l.path("writing"))
}

// FindWriting is a writing's record and its djot: from its bag, or
// before it is bagged from its import.
func FindWriting(root string, id pid.ID) (WritingRecord, []byte, error) {
	var rec WritingRecord
	data := filepath.Join(root, "bags", id.String(), "data")
	if err := readYAML(filepath.Join(data, "writing.yaml"), &rec); err == nil {
		text, err := os.ReadFile(filepath.Join(data, "writing.dj"))
		return rec, text, err
	}
	l, err := WritingImportOf(root, id)
	if err != nil {
		return WritingRecord{}, nil, err
	}
	text, err := l.WritingText()
	return l.writingRecord(id), text, err
}

// WritingImportOf is the import the writing id came in by, found among
// the batches' scans.
func WritingImportOf(root string, id pid.ID) (*Scan, error) {
	found, _ := filepath.Glob(filepath.Join(root, "batches", "*", "scans", "*", "scan.yaml"))
	for _, path := range found {
		var l Scan
		if readYAML(path, &l) != nil || l.Writing == nil || l.Writing.ID == nil || *l.Writing.ID != id {
			continue
		}
		return OpenScan(root, l.Key())
	}
	return nil, fmt.Errorf("writing %s: %w", id, os.ErrNotExist)
}

// WritingStanding is where one writing's own run stands.
func WritingStanding(ctx context.Context, store wf.Store, id pid.ID) (Standing, error) {
	return standingOf(ctx, store, WritingWorkflow, id.String())
}
