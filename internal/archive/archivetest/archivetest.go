// Package archivetest is a stand-in archive service for tests of batches
// and what is built on them: a fixed register of people and an in-memory
// store of items that follows life-weaver's decision table for a deposit
// (docs/archive.md there, *Depositing an item*).
package archivetest

import (
	"archive/tar"
	"bufio"
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

	mu    sync.Mutex
	items map[pid.ID]*item
}

type item struct {
	home    pid.ID
	payload map[string]string // once held
}

// New is a fake over people, item ids counting from 0.
func New(people ...archive.Person) *Fake {
	return &Fake{Register: people, RejectAs: map[pid.ID]pid.ID{}, items: map[pid.ID]*item{}}
}

// Example is a fake over two people, Ada (a) and Aaron (e) Example.
func Example() *Fake {
	return New(archive.Person{ID: 10, Name: "Ada Example"}, archive.Person{ID: 14, Name: "Aaron Example"})
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
	return "", fmt.Errorf("person %s is not in the register (a stand-in)", id)
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
		return archive.Receipt{}, fmt.Errorf("item %s is not known (a stand-in)", id)
	}
	return receipt(id, it), nil
}

func receipt(id pid.ID, it *item) archive.Receipt {
	r := archive.Receipt{Item: id, Home: it.home, Status: archive.Reserved}
	if it.payload != nil {
		r.Status, r.Payload = archive.Held, it.payload
	}
	return r
}

// Deposit takes the bag as the real service would: it must be a tar of a
// whole bag whose manifest is right and whose External-Identifier is the
// item; the same bag twice is one item; other contents under a held id
// are a conflict; a picture held already under another item is a
// duplicate.
func (f *Fake) Deposit(id pid.ID, bag io.Reader) (archive.Receipt, error) {
	if f.Down {
		return archive.Receipt{}, errDown
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	it := f.items[id]
	if it == nil {
		return archive.Receipt{}, fmt.Errorf("item %s is not reserved (a stand-in)", id)
	}
	payload, err := readBag(id, bag)
	if err != nil {
		return archive.Receipt{}, err
	}
	if it.payload != nil {
		if !same(it.payload, payload) {
			return archive.Receipt{}, &archive.Refusal{Status: "conflict"}
		}
		return receipt(id, it), nil
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
			if isPicture(file) && o.payload != nil && hasSum(o.payload, sum) {
				return archive.Receipt{}, &archive.Refusal{Status: "duplicate", DuplicateOf: other, Home: o.home, File: file}
			}
		}
	}
	it.payload = payload
	return receipt(id, it), nil
}

func isPicture(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".tif", ".tiff", ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

func hasSum(payload map[string]string, sum string) bool {
	for file, s := range payload {
		if isPicture(file) && s == sum {
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

// readBag reads the tar, verifies the manifest over its payload and that
// the bag is the item's, and returns the payload manifest.
func readBag(id pid.ID, r io.Reader) (map[string]string, error) {
	sums := map[string]string{}
	var manifest, info string
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bad bag: not a tar: %w", err)
		}
		name := path.Clean(h.Name)
		if strings.HasPrefix(name, "..") || path.IsAbs(name) {
			return nil, fmt.Errorf("bad bag: %s is outside the bag", h.Name)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		switch {
		case name == "manifest-sha256.txt":
			manifest = string(body)
		case name == "bag-info.txt":
			info = string(body)
		case strings.HasPrefix(name, "data/"):
			sum := sha256.Sum256(body)
			sums[name] = hex.EncodeToString(sum[:])
		}
	}
	if manifest == "" {
		return nil, errors.New("bad bag: no manifest")
	}
	if !strings.Contains(info, "External-Identifier: "+id.String()+"\n") {
		return nil, fmt.Errorf("bad bag: not identified as item %s", id)
	}
	payload := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(manifest))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			return nil, errors.New("bad bag: a manifest line is not <sum> <path>")
		}
		if sums[fields[1]] != strings.ToLower(fields[0]) {
			return nil, fmt.Errorf("bad bag: %s does not match its manifest", fields[1])
		}
		payload[fields[1]] = strings.ToLower(fields[0])
	}
	if len(payload) == 0 || len(payload) != len(sums) {
		return nil, errors.New("bad bag: the manifest and the payload differ")
	}
	return payload, nil
}
