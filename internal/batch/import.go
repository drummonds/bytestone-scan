package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/page"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// Import is what the operator brings in: one received file and what is
// known of its capture. PPI stands over what the file declares; left 0,
// the file must say.
type Import struct {
	File     string // the name as sent
	Data     io.Reader
	Sender   string
	Received string // the date received, 2006-01-02; today when blank
	Device   string // what captured the page, if known
	PPI      int
}

// Received is a page's own record: the file as sent and what the operator
// knows of its capture. An import keeps it in scan.yaml; each side cut
// from the page carries it, so a photo.yaml says by itself that its
// master is a copy.
type Received struct {
	File     string `yaml:"file"`
	Format   string `yaml:"format"`
	PPI      int    `yaml:"ppi"`
	Depth    int    `yaml:"depth"`
	Sender   string `yaml:"sender"`
	Received string `yaml:"received"`
	Device   string `yaml:"device,omitempty"`
}

// PageRecord is page.yaml: the record of a page kept as an item of its
// own, holding several prints.
type PageRecord struct {
	ID          pid.ID   `yaml:"id"`
	Batch       string   `yaml:"batch"`
	Import      int      `yaml:"import"`
	Description string   `yaml:"description"`
	Home        Person   `yaml:"home"`
	Provenance  string   `yaml:"provenance,omitempty"`
	Credit      string   `yaml:"credit"`
	File        string   `yaml:"file"`
	Format      string   `yaml:"format"`
	PPI         int      `yaml:"ppi"`
	Depth       int      `yaml:"depth"`
	Sender      string   `yaml:"sender"`
	Received    string   `yaml:"received"`
	Device      string   `yaml:"device,omitempty"`
	Photos      []pid.ID `yaml:"photos"`
}

// NewImport adds the next import to the batch, numbered among its scans:
// the received file kept as sent under the import, decoded once to see
// that it is an image and what it declares. Its description is the
// batch's title when blank. A finished batch takes no more.
func (b *Batch) NewImport(in Import, description string, now time.Time) (*Scan, error) {
	if b.Finished() {
		return nil, fmt.Errorf("batch %s: scanning is finished", b.ID)
	}
	if in.File == "" || in.Data == nil {
		return nil, errors.New("nothing to import: choose a file")
	}
	name := in.File
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return nil, fmt.Errorf("file name %q", in.File)
	}
	raw, err := io.ReadAll(in.Data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	p, err := page.Decode(name, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	r := Received{File: name, Format: p.Format, PPI: in.PPI, Depth: p.Depth, Sender: oneLine(in.Sender), Device: oneLine(in.Device)}
	if r.PPI == 0 {
		r.PPI = p.PPI
	}
	if r.PPI <= 0 {
		return nil, fmt.Errorf("%s: no resolution recorded in the file; give the ppi it was captured at", name)
	}
	if r.Sender == "" {
		return nil, errors.New("a page must say who sent it")
	}
	r.Received = strings.TrimSpace(in.Received)
	if r.Received == "" {
		r.Received = now.Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", r.Received); err != nil {
		return nil, fmt.Errorf("received %q: want the date, as 2006-01-02", in.Received)
	}
	l, err := b.newScan(description, now)
	if err != nil {
		return nil, err
	}
	l.Page = &r
	if err := os.Mkdir(l.path("page"), 0o755); err != nil {
		return nil, err
	}
	if err := writeAtomic(l.pagePath(), func(w io.Writer) error { _, err := w.Write(raw); return err }); err != nil {
		return nil, err
	}
	return l, writeYAML(l.path("scan.yaml"), l)
}

// Import reports whether the scan is an import: its bed a received page.
func (l *Scan) Import() bool { return l.Page != nil }

// pagePath is where the import keeps the file as sent.
func (l *Scan) pagePath() string { return l.path(filepath.Join("page", l.Page.File)) }

// readPage decodes the file as sent.
func (l *Scan) readPage() ([]byte, *page.Page, error) {
	raw, err := os.ReadFile(l.pagePath())
	if err != nil {
		return nil, nil, err
	}
	p, err := page.Decode(l.Page.File, bytes.NewReader(raw))
	return raw, p, err
}

// pageScanner is the page as a scanner: its preview the page at
// PreviewPPI, its detail pass the page's own pixels at its own ppi.
func (l *Scan) pageScanner() (Scanner, error) {
	_, p, err := l.readPage()
	if err != nil {
		return nil, err
	}
	return &page.Scanner{Page: p, PPI: l.Page.PPI, PreviewPPI: PreviewPPI, Device: l.Page.Device}, nil
}

// scanner is what the scan's passes run on: the page for an import, else
// the scanner given.
func (l *Scan) scanner(env Env) (Scanner, error) {
	if l.Import() {
		return l.pageScanner()
	}
	return env.Scanner, nil
}

// PageItem is the id of the page kept as an item of its own, once the
// import's prints are confirmed and there are several; nil for one.
func (l *Scan) PageItem() (*pid.ID, error) {
	var f found
	if err := readYAML(l.path("prints.yaml"), &f); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return f.Page, nil
}

// pageStages are the import's steps for the page item, after the scan is
// accepted and before its photos are packaged: the page bagged with its
// record and its renditions made, deposited, and receipted. With no page
// item (one print) each does nothing.
func (l *Scan) pageStages(env Env) []wf.Stage {
	withPage := func(step func(id pid.ID) error) func(context.Context) error {
		return func(context.Context) error {
			id, err := l.PageItem()
			if err != nil || id == nil {
				return err
			}
			return step(*id)
		}
	}
	return []wf.Stage{
		{Name: "page", Run: withPage(func(id pid.ID) error { return l.packPage(id, env.Now) }), Final: true},
		{Name: "deposit page", Run: withPage(func(id pid.ID) error {
			return l.depositItem(id, env, func(again bool) error {
				if again {
					if err := os.RemoveAll(l.Renditions(id)); err != nil {
						return err
					}
				}
				return l.developPage(id)
			})
		})},
		{Name: "page receipt", Run: withPage(func(id pid.ID) error { return l.receipt(id, env) })},
	}
}

// pageHeld reports whether the photos may be packaged: the page item, if
// there is one, is deposited, held or found a duplicate.
func (l *Scan) pageHeld() (bool, error) {
	id, err := l.PageItem()
	if err != nil || id == nil {
		return err == nil, err
	}
	d, err := DepositOf(l.root, *id)
	return d != nil, err
}

// pageRecord is page.yaml for the page item id.
func (l *Scan) pageRecord(id pid.ID) (PageRecord, error) {
	_, ids, err := l.Found()
	if err != nil {
		return PageRecord{}, err
	}
	b, p := l.batch, l.Page
	return PageRecord{ID: id, Batch: l.Batch, Import: l.N, Description: l.Description,
		Home: Person{b.Person, b.PersonName}, Provenance: b.Provenance, Credit: b.Credit,
		File: p.File, Format: p.Format, PPI: p.PPI, Depth: p.Depth, Sender: p.Sender, Received: p.Received, Device: p.Device,
		Photos: ids}, nil
}

// packPage bags the page item: the file as sent and page.yaml; then its
// renditions.
func (l *Scan) packPage(id pid.ID, now func() time.Time) error {
	if !bag.Complete(l.bags(), id) {
		rec, err := l.pageRecord(id)
		if err != nil {
			return err
		}
		ph, err := bag.Create(l.bags(), id)
		if err != nil {
			return err
		}
		if err := ph.Add(l.Page.File, copyFrom(l.pagePath())); err != nil {
			return err
		}
		if err := ph.Add("page.yaml", func(w io.Writer) error { return encodeYAML(w, rec) }); err != nil {
			return err
		}
		if err := ph.Close(bag.Info{Batch: l.Batch, Scan: l.N, Description: l.Description, Bagged: now()}); err != nil {
			return err
		}
	}
	return l.developPage(id)
}

// developPage makes the page item's renditions at the current recipe,
// unless they are there: the file as sent, fit to the screen.
func (l *Scan) developPage(id pid.ID) error {
	dir := l.Renditions(id)
	if _, err := rendition.Load(dir); err == nil {
		return nil
	}
	raw, p, err := l.readPage()
	if err != nil {
		return err
	}
	r, imgs := rendition.Whole(p.Image)
	sum := sha256.Sum256(raw)
	r.Master = hex.EncodeToString(sum[:])
	return rendition.Save(dir, r, imgs)
}

// cutFrom makes the photo's record name page as the one it was cut from,
// when the archive held the page already as another item: photo.yaml and
// the master's XMP are rewritten, before packaging, and the front's
// renditions with them, since their recipe names the master's checksum.
func (l *Scan) cutFrom(id, page pid.ID) error {
	var photo Photo
	if err := readYAML(l.masterPath(id, "yaml"), &photo); err != nil {
		return err
	}
	if photo.Front.CutFrom != nil && *photo.Front.CutFrom == page {
		return nil
	}
	photo.Front.CutFrom = &page
	tif, err := os.ReadFile(l.masterPath(id, "front"))
	if err != nil {
		return err
	}
	img, err := master.Decode(bytes.NewReader(tif))
	if err != nil {
		return err
	}
	if err := writeAtomic(l.masterPath(id, "front"), func(w io.Writer) error {
		return master.Encode(w, img, master.Options{PPI: photo.Front.MasterPPI, Depth: photo.Front.Depth, XMP: xmp(photo, "front")})
	}); err != nil {
		return err
	}
	if err := writeYAML(l.masterPath(id, "yaml"), photo); err != nil {
		return err
	}
	return os.RemoveAll(l.Renditions(id))
}
