package batch

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// ErrNoID is the refusal of 0 as a person's id: never offered, chosen or
// written; read from an older file, it means none was given or the id is
// gone (#191, life-weaver#309).
var ErrNoID = errors.New("0 is no id")

// ErrRehome is in the refusal of anything an open batch cannot do until
// it is re-homed: its person is not one the register holds, or not the
// one the archive homes its items with.
var ErrRehome = errors.New("Re-home the batch")

// Homed is nil when the batch may begin a scan, import, feed or film
// import: its person is one the register holds. A person who is gone,
// or 0, is refused with ErrRehome and nothing is reserved. A register
// that cannot be asked refuses nothing here: the step that reserves
// says so.
func (b *Batch) Homed(svc archive.Service) error {
	if b.Person == 0 {
		return fmt.Errorf("batch %s names person 0, and %w: %w", b.ID, ErrNoID, ErrRehome)
	}
	if _, err := svc.Person(b.Person); errors.Is(err, archive.ErrNotRegistered) {
		return fmt.Errorf("batch %s: %w: %w", b.ID, err, ErrRehome)
	}
	return nil
}

// Rehome gives an open batch its home person again, when the register
// no longer holds the one it names: batch.yaml names the person chosen,
// and each item not yet deposited has its record written again with that
// home, in its bag if packaged, its id the same. What is deposited is
// left as sent. A finished batch is not re-homed: nothing more will be
// deposited from it; nor one whose person the register holds.
func (b *Batch) Rehome(svc archive.Service, person pid.ID) error {
	if b.Finished() {
		return fmt.Errorf("batch %s: scanning is finished, so nothing more will be deposited from it and it keeps the home it had", b.ID)
	}
	if b.Person != 0 {
		_, err := svc.Person(b.Person)
		if err == nil {
			return fmt.Errorf("batch %s: person %s is in the register, and the batch keeps its home", b.ID, b.Person)
		}
		if !errors.Is(err, archive.ErrNotRegistered) {
			return err
		}
	}
	if person == 0 {
		return fmt.Errorf("the home person: %w", ErrNoID)
	}
	name, err := svc.Person(person)
	if err != nil {
		return err
	}
	home := Person{person, name}
	scans, err := b.Scans()
	if err != nil {
		return err
	}
	for _, l := range scans {
		ids, err := l.items()
		if errors.Is(err, os.ErrNotExist) {
			continue // nothing found yet: no item
		}
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := l.rehome(id, home); err != nil {
				return fmt.Errorf("item %s: %w", id, err)
			}
		}
	}
	// last, so that a re-home cut short is offered again
	again := *b
	again.Person, again.PersonName = person, name
	if err := writeYAML(b.path("batch.yaml"), &again); err != nil {
		return err
	}
	b.Person, b.PersonName = person, name
	return nil
}

// rehome writes the item's record again with the home, wherever it is
// before its deposit: in its bag, or among the scan's masters. A page's
// and a film's are made from the batch when they are packaged.
func (l *Scan) rehome(id pid.ID, home Person) error {
	if d, err := DepositOf(l.root, id); err != nil || d != nil {
		return err // deposited: as sent
	}
	if bag.Complete(l.bags(), id) {
		dir := filepath.Join(l.bags(), id.String())
		name, err := recordOf(dir)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if raw, err = rehomed(raw, home); err != nil {
			return err
		}
		return bag.Replace(l.bags(), id, filepath.Base(name), raw)
	}
	for _, path := range []string{l.masterPath(id, "yaml"), filepath.Join(l.letterDir(id), "letter.yaml")} {
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			return err
		}
		if raw, err = rehomed(raw, home); err != nil {
			return err
		}
		if err := writeAtomic(path, func(w io.Writer) error { _, err := w.Write(raw); return err }); err != nil {
			return err
		}
	}
	return nil
}

// recordOf is the bag's record, relative to the bag: data/photo.yaml or
// another kind's.
func recordOf(dir string) (string, error) {
	for _, name := range records {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err == nil {
			return filepath.FromSlash(name), nil
		}
	}
	return "", fmt.Errorf("the bag %s holds no record", dir)
}

// bagHome is the home the bag's record names.
func bagHome(dir string) (pid.ID, error) {
	name, err := recordOf(dir)
	if err != nil {
		return 0, err
	}
	var rec struct {
		Home Person `yaml:"home"`
	}
	if err := readYAML(filepath.Join(dir, name), &rec); err != nil {
		return 0, err
	}
	return rec.Home.ID, nil
}

// rehomed is the record with its home replaced, everything else as it
// was written.
func rehomed(raw []byte, home Person) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("the record is not a mapping")
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != "home" {
			continue
		}
		if err := m.Content[i+1].Encode(home); err != nil {
			return nil, err
		}
		var out bytes.Buffer
		if err := encodeYAML(&out, &doc); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	return nil, errors.New("the record names no home")
}
