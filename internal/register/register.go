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
	"strings"

	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Register is a directory of person records, <id>.yaml.
type Register struct{ Dir string }

// Person is one person in the register.
type Person struct {
	ID   pid.ID
	Name string
}

// Person is the name of the person with id.
func (r Register) Person(id pid.ID) (string, error) {
	name, err := readName(filepath.Join(r.Dir, id.String()+".yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("person %s is not in the register %s", id, r.Dir)
	}
	if err != nil {
		return "", fmt.Errorf("person %s: %w", id, err)
	}
	return name, nil
}

// People is everyone in the register, sorted by name. A record that does
// not parse or has no name is left out, with a warning naming its file.
func (r Register) People() ([]Person, []string, error) {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return nil, nil, err
	}
	var people []Person
	var warnings []string
	for _, e := range entries {
		base, ok := strings.CutSuffix(e.Name(), ".yaml")
		id, err := pid.Parse(base)
		if !ok || err != nil || id.String() != base || e.IsDir() {
			continue
		}
		name, err := readName(filepath.Join(r.Dir, e.Name()))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s left out: %v", e.Name(), err))
			continue
		}
		people = append(people, Person{id, name})
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
