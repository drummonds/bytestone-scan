// Package archivetest is a stand-in archive service for tests of batches
// and what is built on them: a fixed register of people and an in-memory
// store of items that follows life-weaver's decision table for a deposit
// (docs/archive.md there, *Depositing an item*).
package archivetest

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/life-weaver/writing"
)

// Fake is the service over Register, handing out item ids from Next.
type Fake struct {
	Register []archive.Person
	Next     pid.ID
	// RejectAs makes the deposit of an item a duplicate of another, for
	// tests of what the scanner does with one.
	RejectAs map[pid.ID]pid.ID
	// Down makes every call fail, as a service not running does.
	Down bool
	// Serving is the instance the fake says it serves; Example's is a scratch one.
	Serving archive.Instance
	// NoFixity makes the fake an archive from before life-weaver#87: the
	// fixity call is not there, so it answers as for an unknown item.
	NoFixity bool
	// Rotten are files the fake reports as failing their fixity check,
	// for tests of a master that has decayed at the archive.
	Rotten map[pid.ID][]string
	// Renditions are the rendition files each held item's deposit carried,
	// relative to renditions/: <recipe>/display.jpg and so on.
	Renditions map[pid.ID][]string

	mu    sync.Mutex
	items map[pid.ID]*item
}

type item struct {
	home    pid.ID
	payload map[string]string // once held
}

// New is a fake over people, item ids counting from 0.
func New(people ...archive.Person) *Fake {
	return &Fake{Register: people, RejectAs: map[pid.ID]pid.ID{}, Rotten: map[pid.ID][]string{}, Renditions: map[pid.ID][]string{}, items: map[pid.ID]*item{}}
}

// Example is a fake over two people, Ada (a) and Aaron (e) Example.
func Example() *Fake {
	f := New(archive.Person{ID: 10, Name: "Ada Example"}, archive.Person{ID: 14, Name: "Aaron Example"})
	f.Serving = archive.Instance{Name: "example", Purpose: "scratch", Slot: 1}
	return f
}

func (f *Fake) Instance() (archive.Instance, error) {
	if f.Down {
		return archive.Instance{}, errDown
	}
	return f.Serving, nil
}

func (f *Fake) String() string {
	return fmt.Sprintf("a stand-in archive service of %d people", len(f.Register))
}

var errDown = errors.New("the archive service is not answering (a stand-in, down)")

func (f *Fake) People() ([]archive.Person, error) {
	if f.Down {
		return nil, errDown
	}
	if len(f.Register) == 0 {
		return nil, fmt.Errorf("no people in the register at %s", f)
	}
	people := append([]archive.Person(nil), f.Register...)
	sort.SliceStable(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	return people, nil
}

func (f *Fake) Person(id pid.ID) (string, error) {
	if f.Down {
		return "", errDown
	}
	for _, p := range f.Register {
		if p.ID == id {
			return p.Name, nil
		}
	}
	return "", fmt.Errorf("person %s is %w (a stand-in)", id, archive.ErrNotRegistered)
}

func (f *Fake) Reserve(home pid.ID) (pid.ID, error) {
	if _, err := f.Person(home); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for f.items[f.Next] != nil {
		f.Next++
	}
	id := f.Next
	f.items[id] = &item{home: home}
	f.Next++
	return id, nil
}

func (f *Fake) Receipt(id pid.ID) (archive.Receipt, error) {
	if f.Down {
		return archive.Receipt{}, errDown
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	it := f.items[id]
	if it == nil {
		return archive.Receipt{}, fmt.Errorf("item %s (a stand-in): %w", id, archive.ErrUnknown)
	}
	return receipt(id, it), nil
}

// Fixity answers as the real service does: the item's manifest with
// every file sound, but for those Rotten says have decayed.
func (f *Fake) Fixity(id pid.ID) (archive.Fixity, error) {
	if f.Down {
		return archive.Fixity{}, errDown
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	it := f.items[id]
	if it == nil || f.NoFixity {
		return archive.Fixity{}, fmt.Errorf("fixity of item %s (a stand-in): %w", id, archive.ErrUnknown)
	}
	fx := archive.Fixity{Item: id, Home: it.home, Status: archive.Reserved}
	if it.payload == nil {
		return fx, nil
	}
	fx.Status, fx.Payload, fx.Checked = archive.Held, it.payload, len(it.payload)+3
	for _, file := range f.Rotten[id] {
		fx.Failures = append(fx.Failures, archive.Failure{File: file, Reason: "sha256 does not match the manifest"})
	}
	fx.Sound = len(fx.Failures) == 0
	return fx, nil
}

func receipt(id pid.ID, it *item) archive.Receipt {
	r := archive.Receipt{Item: id, Home: it.home, Status: archive.Reserved}
	if it.payload != nil {
		r.Status, r.Payload, r.Kind = archive.Held, it.payload, kindOf(it.payload)
	}
	return r
}

// Deposit takes the deposit as the real service would: it must be a tar
// of bag/, a bag of this item with its manifest right, and
// renditions/<recipe>/, the front's display.jpg, thumb.jpg, tiny.jpg and
// recipe.yaml, the back's under back/ when the bag has a back.tif, each
// recipe naming a master in the bag. Then life-weaver's deposit table: a
// bag held already is held again if its manifest is the same, a conflict
// if not; a deposit without its renditions is refused; a picture held
// under another item is a duplicate; otherwise it is held.
func (f *Fake) Deposit(id pid.ID, deposit io.Reader) (archive.Receipt, error) {
	if f.Down {
		return archive.Receipt{}, errDown
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	it := f.items[id]
	if it == nil {
		return archive.Receipt{}, fmt.Errorf("item %s is not reserved (a stand-in)", id)
	}
	payload, renditions, djot, err := readDeposit(id, deposit)
	if err != nil {
		return archive.Receipt{}, err
	}
	if it.payload != nil {
		if !same(it.payload, payload) {
			return archive.Receipt{}, &archive.Refusal{Status: "conflict"}
		}
		return receipt(id, it), nil
	}
	if _, ok := payload[writingRecord]; ok {
		return f.holdWriting(id, it, payload, djot)
	}
	if !known(payload) {
		return archive.Receipt{}, errors.New("bad bag: no record the archive knows, and no picture")
	}
	if missing := renditionsMissing(payload, renditions); missing != "" {
		return archive.Receipt{}, &archive.Refusal{Status: "renditions", File: missing}
	}
	if of, ok := f.RejectAs[id]; ok {
		home := it.home
		if o := f.items[of]; o != nil {
			home = o.home
		}
		return archive.Receipt{}, &archive.Refusal{Status: "duplicate", DuplicateOf: of, Home: home, File: "data/front.tif"}
	}
	for other, o := range f.items {
		for file, sum := range payload {
			if isPicture(file) && o.payload != nil && hasSum(o.payload, sum, false) {
				return archive.Receipt{}, &archive.Refusal{Status: "duplicate", DuplicateOf: other, Home: o.home, File: file}
			}
		}
	}
	it.payload = payload
	names := make([]string, 0, len(renditions))
	for name := range renditions {
		names = append(names, name)
	}
	sort.Strings(names)
	f.Renditions[id] = names
	return receipt(id, it), nil
}

const (
	writingRecord = "data/writing.yaml"
	writingText   = "data/writing.dj"
)

// holdWriting takes a writing as life-weaver does (its stories 288, 331,
// 336): the bag holds its djot and its record alone; the djot is checked
// as the site's renderer will read it; one the archive holds already is a
// duplicate; renditions sent with it are not kept.
func (f *Fake) holdWriting(id pid.ID, it *item, payload map[string]string, djot []byte) (archive.Receipt, error) {
	const alone = "bad bag: a writing holds its djot and its record alone: "
	if _, ok := payload[writingText]; !ok {
		return archive.Receipt{}, errors.New(alone + "no writing.dj")
	}
	for file := range payload {
		if file != writingRecord && file != writingText {
			return archive.Receipt{}, errors.New(alone + file + " is neither")
		}
	}
	if err := writing.Check(djot); err != nil {
		return archive.Receipt{}, fmt.Errorf("writing: %s: %w", writingText, err)
	}
	for other, o := range f.items {
		if o.payload != nil && o.payload[writingText] == payload[writingText] {
			return archive.Receipt{}, &archive.Refusal{Status: "duplicate", DuplicateOf: other, Home: o.home, File: writingText}
		}
	}
	it.payload = payload
	return receipt(id, it), nil
}

// known is whether the payload is of a kind the archive knows: it holds
// a record, or a picture, which with no record is a photo.
func known(payload map[string]string) bool {
	for file := range payload {
		if isPicture(file) || file == "data/photo.yaml" || kindOf(map[string]string{file: ""}) != "photo" {
			return true
		}
	}
	return false
}

// renditionsMissing is the first rendition file a deposit lacks, or whose
// recipe names a master not in the bag; "" when it has them all. The
// renditions are at one recipe, a set where sets sit (life-weaver story
// 035): the root, <recipe>/<file>, unless the bag is a letter's; the
// back's under back/ when the bag has a back master; and every directory
// the deposit has under the recipe, a letter's one per side. A film's set
// has play.mp4 beside the stills, and its recipe names the file, which is
// no picture (life-weaver story 032).
func renditionsMissing(payload, renditions map[string]string) string {
	files := []string{"display.jpg", "thumb.jpg", "tiny.jpg", "recipe.yaml"}
	_, isFilm := payload["data/film.yaml"]
	if isFilm {
		files = append(files, "play.mp4")
	}
	recipe := ""
	for name := range renditions {
		recipe, _, _ = strings.Cut(name, "/")
		break
	}
	if recipe == "" {
		return "renditions/"
	}
	sets := map[string]bool{}
	if _, letter := payload["data/letter.yaml"]; !letter {
		sets[""] = true
	}
	if _, ok := payload["data/back.tif"]; ok {
		sets["back/"] = true
	}
	for name := range renditions {
		if dir, _, ok := strings.Cut(strings.TrimPrefix(name, recipe+"/"), "/"); ok {
			sets[dir+"/"] = true
		}
	}
	names := make([]string, 0, len(sets))
	for set := range sets {
		names = append(names, set)
	}
	sort.Strings(names)
	for _, set := range names {
		for _, file := range files {
			name := recipe + "/" + set + file
			body, ok := renditions[name]
			if !ok {
				return "renditions/" + name
			}
			if file == "recipe.yaml" {
				_, master, _ := strings.Cut(body, "master:")
				master, _, _ = strings.Cut(master, "\n")
				if !hasSum(payload, strings.TrimSpace(master), isFilm) {
					return "renditions/" + name
				}
			}
		}
	}
	return ""
}

// kindOf is what a held bag is, read off the record it holds, as
// life-weaver reads it: a photo unless it holds another record.
func kindOf(payload map[string]string) string {
	for _, kind := range []string{"letter", "film", "book", "page", "writing"} {
		if _, ok := payload["data/"+kind+".yaml"]; ok {
			return kind
		}
	}
	return "photo"
}

func isPicture(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".tif", ".tiff", ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

// hasSum is whether the payload holds a picture with the checksum; any
// file but a record, when the bag is a film's.
func hasSum(payload map[string]string, sum string, anyFile bool) bool {
	for file, s := range payload {
		if (isPicture(file) || (anyFile && !strings.HasSuffix(file, ".yaml"))) && s == sum {
			return true
		}
	}
	return false
}

func same(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// readDeposit reads the tar, verifies the bag's manifest over its payload
// and that the bag is the item's, and returns the payload manifest and
// the rendition files, relative to renditions/, with their contents.
func readDeposit(id pid.ID, r io.Reader) (map[string]string, map[string]string, []byte, error) {
	sums, renditions := map[string]string{}, map[string]string{}
	var djot []byte
	var manifest, info string
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("bad bag: not a tar: %w", err)
		}
		name := path.Clean(h.Name)
		if strings.HasPrefix(name, "..") || path.IsAbs(name) {
			return nil, nil, nil, fmt.Errorf("bad bag: %s is outside the deposit", h.Name)
		}
		// read in a stream, as the service does: a film is not held in memory
		text := func() (string, error) {
			body, err := io.ReadAll(tr)
			return string(body), err
		}
		switch {
		case name == "bag/manifest-sha256.txt":
			manifest, err = text()
		case name == "bag/bag-info.txt":
			info, err = text()
		case strings.HasPrefix(name, "bag/data/"):
			h := sha256.New()
			var body io.Writer = h
			var text bytes.Buffer
			if name == "bag/"+writingText { // read, to be checked; small, where a film is not
				body = io.MultiWriter(h, &text)
			}
			_, err = io.Copy(body, tr)
			if name == "bag/"+writingText {
				djot = text.Bytes()
			}
			sums[strings.TrimPrefix(name, "bag/")] = hex.EncodeToString(h.Sum(nil))
		case strings.HasPrefix(name, "renditions/") && path.Base(name) == "recipe.yaml":
			renditions[strings.TrimPrefix(name, "renditions/")], err = text()
		case strings.HasPrefix(name, "renditions/"):
			_, err = io.Copy(io.Discard, tr)
			renditions[strings.TrimPrefix(name, "renditions/")] = ""
		}
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if manifest == "" {
		return nil, nil, nil, errors.New("bad bag: no manifest under bag/")
	}
	if !strings.Contains(info, "External-Identifier: "+id.String()+"\n") {
		return nil, nil, nil, fmt.Errorf("bad bag: not identified as item %s", id)
	}
	payload := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(manifest))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			return nil, nil, nil, errors.New("bad bag: a manifest line is not <sum> <path>")
		}
		if sums[fields[1]] != strings.ToLower(fields[0]) {
			return nil, nil, nil, fmt.Errorf("bad bag: %s does not match its manifest", fields[1])
		}
		payload[fields[1]] = strings.ToLower(fields[0])
	}
	if len(payload) == 0 || len(payload) != len(sums) {
		return nil, nil, nil, errors.New("bad bag: the manifest and the payload differ")
	}
	return payload, renditions, djot, nil
}

// Hold sets what the fake holds for id, for tests of verifying against
// an archive that has changed: payload is the manifest it now holds, nil
// leaving the id reserved with no bag.
func (f *Fake) Hold(id pid.ID, payload map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.items[id] == nil {
		f.items[id] = &item{}
	}
	f.items[id].payload = payload
}

// Forget drops the item altogether, as an archive that has lost it.
func (f *Fake) Forget(id pid.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.items, id)
}

// Renumber moves the person from one id to another, as life-weaver's
// story 313 did: the register names them by the new id, their items are
// homed with it, and each record held is rewritten, so its checksum is
// no longer the one receipted. The masters are as they were.
func (f *Fake) Renumber(from, to pid.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.Register {
		if f.Register[i].ID == from {
			f.Register[i].ID = to
		}
	}
	for _, it := range f.items {
		if it.home != from {
			continue
		}
		it.home = to
		if it.payload == nil {
			continue
		}
		rewritten := map[string]string{}
		for file, sum := range it.payload {
			if strings.HasSuffix(file, ".yaml") {
				h := sha256.Sum256([]byte(sum + " renumbered"))
				sum = hex.EncodeToString(h[:])
			}
			rewritten[file] = sum
		}
		it.payload = rewritten
	}
}
