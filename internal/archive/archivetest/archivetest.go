// Package archivetest is a stand-in archive service for tests of batches and
// what is built on them: a fixed list of people, no archive service.
package archivetest

import (
	"fmt"
	"sort"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// List is a register of these people and no others.
type List []archive.Person

// Example is the people the fixtures used to hold: Ada (a) and Aaron (e).
var Example = List{{ID: 10, Name: "Ada Example"}, {ID: 14, Name: "Aaron Example"}}

// People is the list sorted by name; an empty list is an error, as an
// empty register is.
func (l List) People() ([]archive.Person, error) {
	if len(l) == 0 {
		return nil, fmt.Errorf("no people in the register %s", l)
	}
	people := append([]archive.Person(nil), l...)
	sort.SliceStable(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	return people, nil
}

// Person is the name of the person with id.
func (l List) Person(id pid.ID) (string, error) {
	for _, p := range l {
		if p.ID == id {
			return p.Name, nil
		}
	}
	return "", fmt.Errorf("person %s is not in the register %s", id, l)
}

// String names the register as About shows it.
func (l List) String() string { return fmt.Sprintf("a fixed list of %d people", len(l)) }
