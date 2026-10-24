// Package register reads life-weaver's register of people, the records
// behind i.bytestone.uk: who a batch's home person can be. It is read
// through the archive service's API (life-weaver#23: GET /people and
// GET /people/{id}), never from the archive's tree.
package archive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Register is where people come from.
type Register interface {
	// People is everyone in the register, sorted by name; none is an error.
	People() ([]Person, error)
	// Person is the name of the person with id.
	Person(id pid.ID) (string, error)
	// String names the register, for About.
	String() string
}

// Person is one person in the register.
type Person struct {
	ID   pid.ID
	Name string
}

// Archive is the register as the archive service serves it at URL.
type Archive struct{ URL string }

// entry is what the service lists a person as, the fields the scanner uses.
type entry struct {
	ID   pid.ID `json:"id"`
	Name string `json:"name"`
}

func (a Archive) String() string { return a.URL }

// People asks the service for every entry.
func (a Archive) People() ([]Person, error) {
	var entries []entry
	if err := a.get("/people", &entries); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no people in the register at %s", a.URL)
	}
	people := make([]Person, 0, len(entries))
	for _, e := range entries {
		people = append(people, Person{e.ID, e.Name})
	}
	sort.SliceStable(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	return people, nil
}

// Person asks the service for the person's record.
func (a Archive) Person(id pid.ID) (string, error) {
	var e entry
	err := a.get("/people/"+id.String(), &e)
	if errors.Is(err, errNotFound) {
		return "", fmt.Errorf("person %s is not in the register at %s", id, a.URL)
	}
	if err != nil {
		return "", fmt.Errorf("person %s: %w", id, err)
	}
	return e.Name, nil
}

var errNotFound = errors.New("not found")

// get decodes the JSON the service answers at path into v.
func (a Archive) get(path string, v any) error {
	resp, err := http.Get(strings.TrimRight(a.URL, "/") + path)
	if err != nil {
		return fmt.Errorf("the archive service at %s is not answering (start it with life-weaver serve, or task serve in life-weaver): %w", a.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("the archive service at %s: %s: %s", a.URL, resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("the archive service at %s: %s: %w", a.URL, path, err)
	}
	return nil
}
