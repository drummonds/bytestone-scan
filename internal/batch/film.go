package batch

import (
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/film"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// FilmFormats are the files a film import takes, by extension.
var FilmFormats = []string{".m4v", ".mp4", ".mov", ".avi", ".mkv"}

// FilmRoles are the parts a film names a person in.
var FilmRoles = []string{"director", "producer", "camera", "cast", "narrator", "subject"}

// MadeSources are where a film's made date may come from: the film
// itself, its titles or what is said in it, or a relative's word.
var MadeSources = []string{"family", "film"}

// FilmImport is what the operator brings in: one film as received.
type FilmImport struct {
	File string // the name as sent
	Data io.Reader
}

// Transfer is how a film's file came to be: who digitised it, when, from
// what medium. The film's provenance, as a page's sender is.
type Transfer struct {
	By   string `yaml:"by"`
	Date string `yaml:"date"`
	From string `yaml:"from,omitempty"`
	Note string `yaml:"note,omitempty"`
}

// Poster is the still that stands for a film: the picture attached in
// the file, or the frame at a time, hh:mm:ss.
type Poster struct {
	Attached bool   `yaml:"attached,omitempty"`
	Frame    string `yaml:"frame,omitempty"`
}

// Role is one person a film names and their part in it.
type Role struct {
	Person pid.ID `yaml:"person"`
	Role   string `yaml:"role"`
}

// Made is when a film was made: one claim, shaped as a photo's are.
type Made struct {
	Value       string `yaml:"value"`
	Approximate bool   `yaml:"approximate,omitempty"`
	Source      string `yaml:"source"`
	Read        string `yaml:"read"`
}

// Filmed is a film import's own record, kept in scan.yaml: the file as
// sent and what was read off it, then what the operator confirms and
// says of it, and the film's id once reserved.
type Filmed struct {
	File      string     `yaml:"file"`
	Probe     film.Probe `yaml:"probe"`
	Title     string     `yaml:"title,omitempty"`
	Transfer  Transfer   `yaml:"transfer,omitempty"`
	Poster    Poster     `yaml:"poster,omitempty"`
	Confirmed *time.Time `yaml:"confirmed,omitempty"`
	People    []Role     `yaml:"people,omitempty"`
	Made      *Made      `yaml:"made,omitempty"`
	Described *time.Time `yaml:"described,omitempty"`
	ID        *pid.ID    `yaml:"id,omitempty"`
}

// FilmDetails are what the operator confirms of a film import.
type FilmDetails struct {
	Title    string
	Transfer Transfer
	Poster   Poster
}

// RoleChoice is one part as the operator gives it: the person's id as
// picked, blank when none was.
type RoleChoice struct {
	Person string
	Role   string
}

// MadeClaim is when the film was made, as the operator says it: an ISO
// 8601 prefix, blank when not known; Source family or film, family when
// blank.
type MadeClaim struct {
	Value       string
	Approximate bool
	Source      string
}

// FilmDescription is what the operator says of a film: who it names and
// in what part, when it was made, and, to change it, the poster. Nothing
// known is an answer.
type FilmDescription struct {
	People []RoleChoice
	Made   MadeClaim
	Poster *Poster
}

// FilmFile is what was read off a film's file, as film.yaml keeps it.
type FilmFile struct {
	Name      string         `yaml:"name"`
	Duration  string         `yaml:"duration"`
	Container string         `yaml:"container"`
	Video     string         `yaml:"video"`
	Audio     string         `yaml:"audio,omitempty"`
	FrameRate float64        `yaml:"frame_rate"`
	Size      int64          `yaml:"size"`
	Chapters  []film.Chapter `yaml:"chapters,omitempty"`
}

// FilmRecord is film.yaml: the record of a film as an item (life-weaver
// story 032). Its home, provenance and credit are its batch's.
type FilmRecord struct {
	ID         pid.ID   `yaml:"id"`
	Batch      string   `yaml:"batch"`
	Film       int      `yaml:"film"`
	Title      string   `yaml:"title"`
	Home       Person   `yaml:"home"`
	Provenance string   `yaml:"provenance,omitempty"`
	Credit     string   `yaml:"credit"`
	Transfer   Transfer `yaml:"transfer"`
	Poster     Poster   `yaml:"poster"`
	People     []Role   `yaml:"people,omitempty"`
	Made       *Made    `yaml:"made,omitempty"`
	File       FilmFile `yaml:"file"`
}

// NewFilm adds a film import to the batch, numbered among its scans: the
// file is written to staging as it arrives, never held whole in memory,
// probed, and kept as sent under the import. What its tags say of the
// title and the transfer is offered for the operator to confirm; nothing
// is taken from them of when the film was made. What is not a film is
// refused and nothing of it kept.
func (b *Batch) NewFilm(ctx context.Context, in FilmImport, now time.Time) (*Scan, error) {
	if in.File == "" || in.Data == nil {
		return nil, errors.New("nothing to import: choose a file")
	}
	if b.Finished() {
		return nil, fmt.Errorf("batch %s: scanning is finished", b.ID)
	}
	if err := film.Available(); err != nil {
		return nil, err
	}
	name := in.File
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	ext := strings.ToLower(filepath.Ext(name))
	if strings.HasPrefix(name, ".") || !slices.Contains(FilmFormats, ext) {
		return nil, fmt.Errorf("file name %q: a film is one of %s", in.File, strings.Join(FilmFormats, ", "))
	}
	tmp, err := os.CreateTemp(b.Dir(), "film-*.upload")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(tmp.Name())
		}
	}()
	if _, err := io.Copy(tmp, in.Data); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	p, err := film.Read(ctx, tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	l, err := b.newScan(p.Tags.Title, now)
	if err != nil {
		return nil, err
	}
	l.Film = &Filmed{File: name, Probe: p, Title: oneLine(p.Tags.Title),
		Transfer: Transfer{By: oneLine(p.Tags.Studio), Date: p.Tags.Created}, Poster: Poster{Attached: len(p.Attached) > 0}}
	if err := os.Mkdir(l.path("film"), 0o755); err != nil {
		return nil, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), l.filmUpload()); err != nil {
		return nil, err
	}
	keep = true
	return l, writeYAML(l.path("scan.yaml"), l)
}

// IsFilm reports whether the scan is a film import.
func (l *Scan) IsFilm() bool { return l.Film != nil }

// filmName is the film's name in its bag: film, with the extension the
// file came with.
func (l *Scan) filmName() string { return "film" + strings.ToLower(filepath.Ext(l.Film.File)) }

// filmUpload is where the import keeps the file as sent, until it is bagged.
func (l *Scan) filmUpload() string { return l.path(filepath.Join("film", l.filmName())) }

// filmFile is where the film's file is: in its bag once packaged, in the
// import before.
func (l *Scan) filmFile() string {
	if id := l.Film.ID; id != nil && bag.Complete(l.bags(), *id) {
		return filepath.Join(l.bags(), id.String(), "data", l.filmName())
	}
	return l.filmUpload()
}

// PosterPreview is the path of the poster as chosen, a JPEG, once the
// film is confirmed.
func (l *Scan) PosterPreview() string { return l.path("poster.jpg") }

// filmPackaged reports whether the film is bagged: final.
func (l *Scan) filmPackaged() bool {
	return l.Film.ID != nil && bag.Complete(l.bags(), *l.Film.ID)
}

// setPoster checks the poster against the file and keeps the still it
// gives as the preview.
func (l *Scan) setPoster(ctx context.Context, p Poster) (Poster, error) {
	p.Frame = strings.TrimSpace(p.Frame)
	probe := l.Film.Probe
	switch {
	case p.Attached && len(probe.Attached) == 0:
		return p, errors.New("the poster: the file has no picture attached; give the time of a frame")
	case p.Attached:
		p.Frame = ""
	case p.Frame == "":
		return p, fmt.Errorf("the poster: give the time of a frame, hh:mm:ss within the film, which runs %s", probe.Duration)
	}
	img, err := film.Still(ctx, l.filmFile(), probe, p.Frame)
	if err != nil {
		return p, err
	}
	return p, writeAtomic(l.PosterPreview(), func(w io.Writer) error { return jpeg.Encode(w, img, &jpeg.Options{Quality: 85}) })
}

// ConfirmFilm records the film's title, its transfer and its poster. A
// film has a title, and the transfer, who made it and when, is its
// provenance; the poster is the picture attached or a frame within the
// film. Refused once the film is described and bagged.
func (l *Scan) ConfirmFilm(ctx context.Context, d FilmDetails) error {
	if !l.IsFilm() {
		return fmt.Errorf("scan %s is not a film import", l.Key())
	}
	if l.filmPackaged() {
		return fmt.Errorf("film %s is packaged; its record is no longer the import's to change", l.Film.ID)
	}
	title := oneLine(d.Title)
	tr := Transfer{By: oneLine(d.Transfer.By), Date: strings.TrimSpace(d.Transfer.Date), From: oneLine(d.Transfer.From), Note: oneLine(d.Transfer.Note)}
	switch {
	case title == "":
		return errors.New("a film has a title")
	case tr.By == "" || tr.Date == "":
		return errors.New("the transfer is the film's provenance: say who made it and when")
	case !isISOPrefix(tr.Date):
		return fmt.Errorf("the transfer's date %q: want an ISO 8601 prefix such as 2016, 2016-03 or 2016-03-07", d.Transfer.Date)
	}
	poster, err := l.setPoster(ctx, d.Poster)
	if err != nil {
		return err
	}
	at := time.Now().UTC()
	l.Film.Title, l.Film.Transfer, l.Film.Poster, l.Film.Confirmed = title, tr, poster, &at
	l.Description = title
	return writeYAML(l.path("scan.yaml"), l)
}

func isISOPrefix(date string) bool {
	m := isoPrefix.FindStringSubmatch(date)
	return m != nil && (m[2] == "" || (m[2] >= "01" && m[2] <= "12")) && (m[3] == "" || (m[3] >= "01" && m[3] <= "31"))
}

// DescribeFilm records who the film names and in what part, each a
// person of the register, and when it was made; nothing known is an
// answer. The poster may be changed with it. Refused before the film is
// confirmed and once it is bagged.
func (l *Scan) DescribeFilm(ctx context.Context, svc archive.Service, d FilmDescription, now time.Time) error {
	switch {
	case !l.IsFilm():
		return fmt.Errorf("scan %s is not a film import", l.Key())
	case l.Film.Confirmed == nil:
		return fmt.Errorf("film import %s: confirm its title, transfer and poster first", l.Key())
	case l.filmPackaged():
		return fmt.Errorf("film %s is packaged; its record is no longer the import's to change", l.Film.ID)
	}
	var people []Role
	for _, c := range d.People {
		role, who := strings.TrimSpace(c.Role), strings.TrimSpace(c.Person)
		if role == "" && who == "" {
			continue
		}
		if !slices.Contains(FilmRoles, role) {
			return fmt.Errorf("role %q: want one of %s", c.Role, strings.Join(FilmRoles, ", "))
		}
		if who == "" {
			return fmt.Errorf("a part needs its person: choose who the %s is", role)
		}
		id, err := pid.Parse(who)
		if err != nil {
			return fmt.Errorf("the %s: %w", role, err)
		}
		if _, err := svc.Person(id); err != nil {
			return fmt.Errorf("the %s: %w", role, err)
		}
		people = append(people, Role{Person: id, Role: role})
	}
	var made *Made
	if value := strings.TrimSpace(d.Made.Value); value != "" {
		if !isISOPrefix(value) {
			return fmt.Errorf("made %q: want an ISO 8601 prefix such as 1970, 1970-06 or 1970-06-14", d.Made.Value)
		}
		source := d.Made.Source
		if source == "" {
			source = "family"
		}
		if !slices.Contains(MadeSources, source) {
			return fmt.Errorf("source %q, want %s", d.Made.Source, strings.Join(MadeSources, " or "))
		}
		made = &Made{Value: value, Approximate: d.Made.Approximate, Source: source, Read: "transcribed"}
	}
	poster := l.Film.Poster
	if d.Poster != nil && *d.Poster != poster {
		p, err := l.setPoster(ctx, *d.Poster)
		if err != nil {
			return err
		}
		poster = p
	}
	at := now.UTC()
	l.Film.People, l.Film.Made, l.Film.Poster, l.Film.Described = people, made, poster, &at
	return writeYAML(l.path("scan.yaml"), l)
}

// filmed reads the import's record afresh: a step sees what the operator
// has said since the scan was opened.
func (l *Scan) filmed() (*Filmed, error) {
	fresh, err := l.batch.Scan(l.N)
	if err != nil {
		return nil, err
	}
	if fresh.Film == nil {
		return nil, fmt.Errorf("scan %s is not a film import", l.Key())
	}
	l.Film, l.Description = fresh.Film, fresh.Description
	return l.Film, nil
}

// reserveFilmID gives the film its id, once, homed with the batch's person.
func (l *Scan) reserveFilmID(svc archive.Service) error {
	f, err := l.filmed()
	if err != nil || f.ID != nil {
		return err
	}
	id, err := svc.Reserve(l.batch.Person)
	if err != nil {
		return err
	}
	f.ID = &id
	return writeYAML(l.path("scan.yaml"), l)
}

// startFilm starts the film's workflow under the import's run.
func (l *Scan) startFilm(ctx context.Context, store wf.Store) error {
	f, err := l.filmed()
	if err != nil {
		return err
	}
	if f.ID == nil {
		return fmt.Errorf("film import %s: the film has no id yet", l.Key())
	}
	run, err := store.FindRunByKey(ctx, FilmImportWorkflow, l.Key())
	if err != nil || run == nil {
		return fmt.Errorf("film import %s: no run to start its film under: %v", l.Key(), err)
	}
	date := wf.NewBusinessDate(l.Created.Year(), l.Created.Month(), l.Created.Day())
	_, err = wf.NewRunner(store, nil).StartChild(ctx, run.ID, FilmWorkflow, f.ID.String(), date)
	return err
}

// filmImportStages are the film import's workflow steps, in order.
func (l *Scan) filmImportStages(env Env) []wf.Stage {
	return []wf.Stage{
		{Name: "import", Run: func(context.Context) error {
			_, err := os.Stat(l.path("scan.yaml"))
			return err
		}},
		{Name: "confirm", Until: func(context.Context) (bool, error) {
			f, err := l.filmed()
			return err == nil && f.Confirmed != nil, err
		}},
		{Name: "describe",
			Until: func(context.Context) (bool, error) {
				f, err := l.filmed()
				return err == nil && f.Described != nil, err
			},
			Run: func(context.Context) error { return l.reserveFilmID(env.Archive) },
		},
		{Name: "film", Run: func(ctx context.Context) error { return l.startFilm(ctx, env.Store) }},
	}
}

// filmStages are the film's workflow steps: bagged with its renditions,
// deposited and receipted.
func (l *Scan) filmStages(id pid.ID, env Env) []wf.Stage {
	return []wf.Stage{
		// An archived film is never altered, so nothing before packaging can be redone after it.
		{Name: "package", Run: func(ctx context.Context) error { return l.packFilm(ctx, id, env.Now) }, Final: true},
		{Name: "deposit", Run: func(ctx context.Context) error {
			return l.depositItem(id, env, func(again bool) error { return l.developFilm(ctx, id, again) })
		}},
		{Name: "receipt", Run: func(context.Context) error {
			if err := l.receipt(id, env); err != nil {
				return err
			}
			d, err := DepositOf(l.root, id)
			if err != nil {
				return err
			}
			if d != nil && d.Status != "duplicate" && d.Kind != "film" {
				return fmt.Errorf("film %s: the archive holds it as %q, not as a film", id, d.Kind)
			}
			return nil
		}},
	}
}

// filmRecord is film.yaml for the film id.
func (l *Scan) filmRecord(id pid.ID) FilmRecord {
	b, f := l.batch, l.Film
	p := f.Probe
	return FilmRecord{ID: id, Batch: l.Batch, Film: l.N, Title: f.Title,
		Home: Person{b.Person, b.PersonName}, Provenance: b.Provenance, Credit: b.Credit,
		Transfer: f.Transfer, Poster: f.Poster, People: f.People, Made: f.Made,
		File: FilmFile{Name: f.File, Duration: p.Duration, Container: p.Container, Video: p.Video, Audio: p.Audio,
			FrameRate: p.FrameRate, Size: p.Size, Chapters: p.Chapters}}
}

// packFilm bags the film: the file as sent, under the extension it came
// with, and film.yaml; then its renditions. The upload is cleared from
// the import once the bag holds the file.
func (l *Scan) packFilm(ctx context.Context, id pid.ID, now func() time.Time) error {
	if _, err := l.filmed(); err != nil {
		return err
	}
	if !bag.Complete(l.bags(), id) {
		rec := l.filmRecord(id)
		ph, err := bag.Create(l.bags(), id)
		if err != nil {
			return err
		}
		if err := ph.Add(l.filmName(), copyFrom(l.filmUpload())); err != nil {
			return err
		}
		if err := ph.Add("film.yaml", func(w io.Writer) error { return encodeYAML(w, rec) }); err != nil {
			return err
		}
		if err := ph.Close(bag.Info{Batch: l.Batch, Scan: l.N, Description: l.Description, Bagged: now()}); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(l.path("film")); err != nil {
		return err
	}
	return l.developFilm(ctx, id, false)
}

// developFilm sees the film's renditions are there at the current recipe,
// making them when they are not, or again: the poster's display, thumb
// and tiny with a recipe naming the file, and the play copy beside them.
func (l *Scan) developFilm(ctx context.Context, id pid.ID, again bool) error {
	dir := l.Renditions(id)
	if again {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	play := filepath.Join(dir, "play.mp4")
	if _, err := rendition.Load(dir); err == nil {
		if _, err := os.Stat(play); err == nil {
			return nil
		}
	}
	if _, err := l.filmed(); err != nil {
		return err
	}
	manifest, err := bag.Manifest(filepath.Join(l.bags(), id.String()))
	if err != nil {
		return err
	}
	src := l.filmFile()
	img, err := film.Still(ctx, src, l.Film.Probe, l.Film.Poster.Frame)
	if err != nil {
		return fmt.Errorf("film %s: %w", id, err)
	}
	r, imgs := rendition.Whole(img)
	r.Master = manifest["data/"+l.filmName()]
	if err := rendition.Save(dir, r, imgs); err != nil {
		return fmt.Errorf("film %s: %w", id, err)
	}
	if err := film.PlayCopy(ctx, src, play, l.Film.Probe); err != nil {
		return fmt.Errorf("film %s: %w", id, err)
	}
	return nil
}

// FindFilm is a film's record and the directory its files are in: its
// bag, or before it is bagged its import.
func FindFilm(root string, id pid.ID) (FilmRecord, string, error) {
	var rec FilmRecord
	data := filepath.Join(root, "bags", id.String(), "data")
	if err := readYAML(filepath.Join(data, "film.yaml"), &rec); err == nil {
		return rec, data, nil
	}
	l, err := FilmImportOf(root, id)
	if err != nil {
		return FilmRecord{}, "", err
	}
	return l.filmRecord(id), l.path("film"), nil
}

// FilmImportOf is the import the film id came in by, found among the
// batches' scans.
func FilmImportOf(root string, id pid.ID) (*Scan, error) {
	found, _ := filepath.Glob(filepath.Join(root, "batches", "*", "scans", "*", "scan.yaml"))
	for _, path := range found {
		var l Scan
		if readYAML(path, &l) != nil || l.Film == nil || l.Film.ID == nil || *l.Film.ID != id {
			continue
		}
		return OpenScan(root, l.Key())
	}
	return nil, fmt.Errorf("film %s: %w", id, os.ErrNotExist)
}

// FilmStanding is where one film's own run stands.
func FilmStanding(ctx context.Context, store wf.Store, id pid.ID) (Standing, error) {
	return standingOf(ctx, store, FilmWorkflow, id.String())
}
