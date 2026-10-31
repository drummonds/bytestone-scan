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
	"sync"
	"time"

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
	// Instance is which archive the service serves: production, or a
	// scratch or scale instance. A service from before instances were
	// reported gives an unnamed one.
	Instance() (Instance, error)
	// String names the service, for About.
	String() string
}

// Instance is an archive as life-weaver's archive.yaml names it: one tree
// among several that may be served at once.
type Instance struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"` // production, scratch or scale
	// Slot is the instance's place among the ten that may serve at once:
	// production is 0, the others 1 to 9. Its service is at BasePort+Slot,
	// and a scanner run for it takes its own port and staging from it.
	Slot int `json:"slot"`
}

// BasePort is slot 0's port, production's: life-weaver serve's default.
// Slots is how many instances may serve at once, on the ports following.
const (
	BasePort = 8321
	Slots    = 10
)

// Serving is an instance found answering at a URL.
type Serving struct {
	URL      string
	Instance Instance
}

// Addresses are where instances serve on this machine: localhost, at
// BasePort+slot for every slot.
func Addresses() []string {
	urls := make([]string, Slots)
	for i := range urls {
		urls[i] = fmt.Sprintf("http://localhost:%d", BasePort+i)
	}
	return urls
}

// Discover asks each URL which instance serves there, all at once, and
// returns those that answer, in URL order.
func Discover(urls []string) []Serving {
	found := make([]*Serving, len(urls))
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Go(func() {
			if in, err := (Client{URL: url, Timeout: time.Second}).Instance(); err == nil {
				found[i] = &Serving{URL: url, Instance: in}
			}
		})
	}
	wg.Wait()
	var out []Serving
	for _, f := range found {
		if f != nil {
			out = append(out, *f)
		}
	}
	return out
}

// Choose picks the instance the scanner was started for among those
// found: the one named, or production when none was named. When it is
// not serving, the refusal says what was found and how to start it.
func Choose(name string, found []Serving) (Serving, error) {
	var names []string
	for _, f := range found {
		if (name == "" && f.Instance.Production()) || (name != "" && f.Instance.Name == name) {
			return f, nil
		}
		names = append(names, f.Instance.String()+" on "+f.URL)
	}
	what := "none"
	if len(names) > 0 {
		what = strings.Join(names, ", ")
	}
	if name == "" {
		return Serving{}, fmt.Errorf("no production archive is serving on localhost:%d-%d (found: %s): start it with life-weaver serve, or task serve in life-weaver, or run the scanner for another with -instance", BasePort, BasePort+Slots-1, what)
	}
	return Serving{}, fmt.Errorf("archive instance %s is not serving on localhost:%d-%d (found: %s): start it with life-weaver serve -archive <its tree>", name, BasePort, BasePort+Slots-1, what)
}

// Production says whether the instance is the real archive.
func (in Instance) Production() bool { return in.Purpose == "production" }

// String is the instance as the operator reads it.
func (in Instance) String() string {
	if in.Name == "" {
		return "an archive that does not say which instance it is"
	}
	if in.Production() {
		return in.Name
	}
	return in.Name + " (" + in.Purpose + ")"
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
type Client struct {
	URL     string
	Timeout time.Duration // per call; 0 is no limit
}

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
func (c Client) Instance() (Instance, error) {
	var v struct {
		Instance Instance `json:"instance"`
	}
	if err := c.call("GET", "/version", nil, &v); err != nil {
		return Instance{}, err
	}
	return v.Instance, nil
}

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
		return r, fmt.Errorf("item %s at %s: %w", item, c.URL, ErrUnknown)
	}
	if err != nil {
		return r, fmt.Errorf("item %s: %w", item, err)
	}
	return r, nil
}

var errNotFound = errors.New("not found")

// ErrUnknown is the archive's answer for an item id it has never given
// out, or has lost: Receipt wraps it.
var ErrUnknown = errors.New("not known to the archive")

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
	resp, err := (&http.Client{Timeout: c.Timeout}).Do(req)
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
