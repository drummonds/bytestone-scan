// Package register reads life-weaver's register of people, the records
// behind i.bytestone.uk: who a batch's home person can be.
package register

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Register is the archive's people tree: a directory per person
// (pid.Dir) holding person.yaml.
type Register struct{ Dir string }

// recordFile is the record's name in the person's directory.
const recordFile = "person.yaml"

// Person is one person in the register.
type Person struct {
	ID   pid.ID
	Name string
}

// Person is the name of the person with id.
func (r Register) Person(id pid.ID) (string, error) {
	name, err := readName(filepath.Join(r.Dir, filepath.FromSlash(id.Dir()), recordFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("person %s is not in the register %s", id, r.Dir)
	}
	if err != nil {
		return "", fmt.Errorf("person %s: %w", id, err)
	}
	return name, nil
}

// People is everyone in the register, sorted by name. A record that does
// not parse or has no name is left out, with a warning naming its
// directory; a person.yaml in a directory that is not a person's is ignored.
func (r Register) People() ([]Person, []string, error) {
	if _, err := os.Stat(r.Dir); err != nil {
		return nil, nil, err
	}
	var people []Person
	var warnings []string
	err := filepath.WalkDir(r.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != recordFile {
			return err
		}
		rel, err := filepath.Rel(r.Dir, filepath.Dir(path))
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		id, err := pid.ParseDir(rel)
		if err != nil {
			return nil
		}
		name, err := readName(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s left out: %v", rel, err))
			return nil
		}
		people = append(people, Person{id, name})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(people) == 0 {
		return nil, warnings, fmt.Errorf("no people in the register %s", r.Dir)
	}
	sort.SliceStable(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	return people, warnings, nil
}

func readName(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var rec struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(b, &rec); err != nil {
		return "", err
	}
	if rec.Name == "" {
		return "", errors.New("record has no name")
	}
	return rec.Name, nil
}
