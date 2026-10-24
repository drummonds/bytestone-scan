// Package archive is the client of life-weaver's archive service, the
// keeper of the bytestone archive: the register of people behind
// i.bytestone.uk (life-weaver#23: GET /people and GET /people/{id}), and
// the items in each person's collection (life-weaver#33: an item id is
// reserved under its home person, its bag deposited, a receipt read).
// The scanner never reads or writes the archive's tree.
package archive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Service is the archive service as the scanner uses it.
type Service interface {
	// People is everyone in the register, sorted by name; none is an error.
	People() ([]Person, error)
	// Person is the name of the person with id.
	Person(id pid.ID) (string, error)
	// Reserve takes the next item id, homed with the person.
	Reserve(home pid.ID) (pid.ID, error)
	// Deposit hands the item's deposit, a tar of bag/ and
	// renditions/<recipe>/, to the archive and returns its receipt: held,
	// with the checksums of the bag it holds. A deposit the archive will
	// not take is a *Refusal; anything else is a failure to try again.
	Deposit(item pid.ID, deposit io.Reader) (Receipt, error)
	// Receipt is the archive's record of the item: reserved, or held.
	Receipt(item pid.ID) (Receipt, error)
	// String names the service, for About.
	String() string
}

// Person is one person in the register.
type Person struct {
	ID   pid.ID
	Name string
}

// Status is where an item stands with the archive.
type Status string

const (
	// Reserved: the id is given out for a home person; no bag is held yet.
	Reserved Status = "reserved"
	// Held: the bag is in the archive, every checksum verified.
	Held Status = "held"
)

// Receipt is the archive's answer for an item.
type Receipt struct {
	Item   pid.ID `json:"item"`
	Home   pid.ID `json:"home"`
	Status Status `json:"status"`
	// Payload is the held bag's manifest: path within the bag → sha256.
	Payload map[string]string `json:"payload,omitempty"`
}

// Refusal is why the archive will not hold a bag: "duplicate", a picture
// in it being held already as part of DuplicateOf (homed with Home);
// "conflict", the id holding a bag with other checksums; or "renditions",
// the deposit lacking File, a rendition, or its recipe naming a master
// that is not in the bag.
type Refusal struct {
	Status      string `json:"status"`
	DuplicateOf pid.ID `json:"duplicate_of,omitempty"`
	Home        pid.ID `json:"home,omitempty"`
	File        string `json:"file,omitempty"`
}

func (r *Refusal) Error() string {
	switch r.Status {
	case "duplicate":
		return fmt.Sprintf("refused as a duplicate: %s is held already as part of item %s (home %s)", r.File, r.DuplicateOf, r.Home)
	case "renditions":
		return fmt.Sprintf("refused for its renditions: %s is missing or not this bag's", r.File)
	}
	return "refused: the item holds a bag with other contents"
}

// Client is the archive service at URL.
type Client struct{ URL string }

// entry is what the service lists a person as, the fields the scanner uses.
type entry struct {
	ID   pid.ID `json:"id"`
	Name string `json:"name"`
}

func (c Client) String() string { return c.URL }

// People asks the service for every entry.
func (c Client) People() ([]Person, error) {
	var entries []entry
	if err := c.call(http.MethodGet, "/people", nil, &entries); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no people in the register at %s", c.URL)
	}
	people := make([]Person, 0, len(entries))
	for _, e := range entries {
		people = append(people, Person{e.ID, e.Name})
	}
	sort.SliceStable(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	return people, nil
}

// Person asks the service for the person's record.
func (c Client) Person(id pid.ID) (string, error) {
	var e entry
	err := c.call(http.MethodGet, "/people/"+id.String(), nil, &e)
	if errors.Is(err, errNotFound) {
		return "", fmt.Errorf("person %s is not in the register at %s", id, c.URL)
	}
	if err != nil {
		return "", fmt.Errorf("person %s: %w", id, err)
	}
	return e.Name, nil
}

// Reserve asks the service for the next item id under the person.
func (c Client) Reserve(home pid.ID) (pid.ID, error) {
	var r Receipt
	err := c.call(http.MethodPost, "/people/"+home.String()+"/items", nil, &r)
	if errors.Is(err, errNotFound) {
		return 0, fmt.Errorf("person %s is not in the register at %s", home, c.URL)
	}
	if err != nil {
		return 0, fmt.Errorf("reserving an item for %s: %w", home, err)
	}
	return r.Item, nil
}

// Deposit puts the photo's deposit to the service.
func (c Client) Deposit(item pid.ID, deposit io.Reader) (Receipt, error) {
	var r Receipt
	err := c.call(http.MethodPut, "/items/"+item.String(), deposit, &r)
	if errors.Is(err, errNotFound) {
		return r, fmt.Errorf("item %s is not reserved at %s", item, c.URL)
	}
	if err != nil {
		return r, fmt.Errorf("depositing item %s: %w", item, err)
	}
	return r, nil
}

// Receipt asks the service where the item stands.
func (c Client) Receipt(item pid.ID) (Receipt, error) {
	var r Receipt
	err := c.call(http.MethodGet, "/items/"+item.String(), nil, &r)
	if errors.Is(err, errNotFound) {
		return r, fmt.Errorf("item %s is not known at %s", item, c.URL)
	}
	if err != nil {
		return r, fmt.Errorf("item %s: %w", item, err)
	}
	return r, nil
}

var errNotFound = errors.New("not found")

// call sends body (if any) to path with method and decodes the JSON the
// service answers into v. A 409 is returned as the *Refusal in its body.
func (c Client) call(method, path string, body io.Reader, v any) error {
	req, err := http.NewRequest(method, strings.TrimRight(c.URL, "/")+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-tar")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("the archive service at %s is not answering (start it with life-weaver serve, or task serve in life-weaver): %w", c.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("the archive service at %s: %s: %w", c.URL, path, err)
		}
		return nil
	case http.StatusNotFound:
		return errNotFound
	case http.StatusConflict, http.StatusBadRequest:
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var r Refusal
		if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&r); err != nil || r.Status == "" || (resp.StatusCode == http.StatusBadRequest && r.Status != "renditions") {
			return fmt.Errorf("the archive service at %s: %s: %s", c.URL, resp.Status, strings.TrimSpace(string(raw)))
		}
		return &r
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	return fmt.Errorf("the archive service at %s: %s: %s", c.URL, resp.Status, strings.TrimSpace(string(raw)))
}
