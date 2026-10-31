package archive

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// service is a stand-in archive service: life-weaver's GET /people and
// GET /people/{id}, over a fixed register.
func service(t *testing.T, people string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /people", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(people))
	})
	mux.HandleFunc("GET /people/{id}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("id") {
		case "a":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"a","name":"Ada Example","short":"Ada","bio":"long"}`))
		case "b":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.Error(w, "no person "+r.PathValue("id"), http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"v0.14.0","instance":{"name":"h3","purpose":"scale","slot":1}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// #99: Instance is which archive the service serves, as /version says
// it; a service from before instances were reported says nothing, which
// is no error.
func TestInstanceIsWhatVersionSays(t *testing.T) {
	c := Client{URL: service(t, three).URL}
	in, err := c.Instance()
	if err != nil || in != (Instance{Name: "h3", Purpose: "scale", Slot: 1}) {
		t.Fatalf("instance %+v, %v", in, err)
	}
	if in.Production() {
		t.Error("a scale instance is not production")
	}
	if !(Instance{Name: "bytestone", Purpose: "production"}).Production() {
		t.Error("a production instance is")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"version":"v0.13.0"}`)) })
	old := httptest.NewServer(mux)
	t.Cleanup(old.Close)
	if in, err := (Client{URL: old.URL}).Instance(); err != nil || in != (Instance{}) {
		t.Errorf("an older service: %+v, %v, want an unnamed instance and no error", in, err)
	}
}

const three = `[{"id":"a","name":"Ada Example","short":"Ada","born":{"date":"1894"}},{"id":"e","name":"Aaron Example"},{"id":"f","name":"Flat Example"}]`

func TestPersonInTheRegister(t *testing.T) {
	r := Client{URL: service(t, three).URL}
	name, err := r.Person(10) // "a"
	if err != nil || name != "Ada Example" {
		t.Errorf("Person(a) = %q, %v, want Ada Example", name, err)
	}
}

func TestPersonRefusals(t *testing.T) {
	srv := service(t, three)
	r := Client{URL: srv.URL}
	for _, tc := range []struct {
		id   pid.ID
		want string
	}{
		{12, "not in the register"}, // "c": no record
		{11, "boom"},                // the service fails
	} {
		_, err := r.Person(tc.id)
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), srv.URL) {
			t.Errorf("Person(%s): %v, want an error mentioning %q and the service", tc.id, err, tc.want)
		}
	}
}

func TestPeopleAreListedByName(t *testing.T) {
	people, err := Client{URL: service(t, three).URL}.People()
	if err != nil {
		t.Fatal(err)
	}
	want := []Person{{14, "Aaron Example"}, {10, "Ada Example"}, {15, "Flat Example"}}
	if !reflect.DeepEqual(people, want) {
		t.Errorf("People() = %v, want %v", people, want)
	}
}

func TestNoPeopleIsAnError(t *testing.T) {
	srv := service(t, `[]`)
	if _, err := (Client{URL: srv.URL}).People(); err == nil || !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("People() of an empty register: %v, want an error naming %s", err, srv.URL)
	}
}

func TestServiceNotRunningSaysHowToStartIt(t *testing.T) {
	srv := service(t, three)
	srv.Close()
	r := Client{URL: srv.URL}
	for _, err := range []error{
		func() error { _, err := r.People(); return err }(),
		func() error { _, err := r.Person(10); return err }(),
	} {
		if err == nil || !strings.Contains(err.Error(), srv.URL) || !strings.Contains(err.Error(), "life-weaver serve") {
			t.Errorf("service down: %v, want an error naming %s and life-weaver serve", err, srv.URL)
		}
	}
}

func TestStringIsTheServiceURL(t *testing.T) {
	if s := (Client{URL: "http://localhost:8321"}).String(); s != "http://localhost:8321" {
		t.Errorf("String() = %q", s)
	}
}

// Story 041: the client reserves item ids, deposits bags and reads
// receipts over life-weaver's API, and tells a refusal from a failure.
func TestReserveDepositAndReceipt(t *testing.T) {
	mux := http.NewServeMux()
	var got []byte
	mux.HandleFunc("POST /people/{id}/items", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "a" {
			http.Error(w, "no person", http.StatusNotFound)
			return
		}
		w.Header().Set("Location", "/items/8")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"item":"8","home":"a","status":"reserved"}`))
	})
	mux.HandleFunc("PUT /items/{item}", func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		switch r.PathValue("item") {
		case "8":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"item":"8","home":"a","status":"held","payload":{"data/front.tif":"ab12"}}`))
		case "9":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"status":"duplicate","duplicate_of":"3","home":"a","file":"data/front.tif"}`))
		case "c":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"status":"conflict"}`))
		case "d":
			http.Error(w, "bad bag: manifest wrong", http.StatusBadRequest)
		case "e":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":"renditions","file":"renditions/1/display.jpg"}`))
		default:
			http.Error(w, "no item reserved", http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /items/{item}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("item") != "8" {
			http.Error(w, "no item", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"item":"8","home":"a","status":"held","payload":{"data/front.tif":"ab12"}}`))
	})
	mux.HandleFunc("GET /items/{item}/fixity", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("item") {
		case "8":
			_, _ = w.Write([]byte(`{"item":"8","home":"a","status":"held","payload":{"data/front.tif":"ab12"},"checked":4,"sound":true}`))
		case "9":
			_, _ = w.Write([]byte(`{"item":"9","home":"a","status":"held","checked":1,"failures":[{"file":"data/front.tif","reason":"sha256 does not match the manifest"}],"sound":false}`))
		default:
			http.Error(w, "no item", http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := Client{URL: srv.URL}

	id, err := c.Reserve(10)
	if err != nil || id != 8 {
		t.Fatalf("Reserve(a) = %s, %v", id, err)
	}
	if _, err := c.Reserve(12); err == nil || !strings.Contains(err.Error(), "not in the register") {
		t.Errorf("Reserve(c): %v", err)
	}
	rec, err := c.Deposit(8, strings.NewReader("the tar"))
	if err != nil || rec.Status != Held || rec.Payload["data/front.tif"] != "ab12" || string(got) != "the tar" {
		t.Errorf("Deposit(8) = %+v, %v; sent %q", rec, err, got)
	}
	_, err = c.Deposit(9, strings.NewReader("x"))
	var ref *Refusal
	if !errors.As(err, &ref) || ref.Status != "duplicate" || ref.DuplicateOf != 3 || ref.Home != 10 || ref.File != "data/front.tif" {
		t.Errorf("Deposit(9) = %v, want a duplicate refusal naming item 3", err)
	}
	if _, err := c.Deposit(12, strings.NewReader("x")); !errors.As(err, &ref) || ref.Status != "conflict" {
		t.Errorf("Deposit(c) = %v, want a conflict refusal", err)
	}
	if _, err := c.Deposit(13, strings.NewReader("x")); err == nil || errors.As(err, &ref) || !strings.Contains(err.Error(), "manifest wrong") {
		t.Errorf("Deposit(d) = %v, want the service's complaint, not a refusal", err)
	}
	// story 055: a deposit without its renditions, or with another bag's, is refused, not failed
	if _, err := c.Deposit(14, strings.NewReader("x")); !errors.As(err, &ref) || ref.Status != "renditions" || ref.File != "renditions/1/display.jpg" || !strings.Contains(err.Error(), "renditions") {
		t.Errorf("Deposit(e) = %v, want a renditions refusal naming the file", err)
	}
	if rec, err := c.Receipt(8); err != nil || rec.Status != Held || rec.Home != 10 {
		t.Errorf("Receipt(8) = %+v, %v", rec, err)
	}
	if _, err := c.Receipt(9); !errors.Is(err, ErrUnknown) {
		t.Errorf("Receipt(9) of an item never reserved: %v, want ErrUnknown", err)
	}
	// life-weaver#87: the checksums recomputed from the archive's disk
	if f, err := c.Fixity(8); err != nil || !f.Sound || f.Checked != 4 || f.Payload["data/front.tif"] != "ab12" {
		t.Errorf("Fixity(8) = %+v, %v, want sound", f, err)
	}
	if f, err := c.Fixity(9); err != nil || f.Sound || len(f.Failures) != 1 || f.Failures[0].String() != "data/front.tif: sha256 does not match the manifest" {
		t.Errorf("Fixity(9) = %+v, %v, want one failure", f, err)
	}
	if _, err := c.Fixity(12); !errors.Is(err, ErrUnknown) {
		t.Errorf("Fixity(c) of an item never reserved: %v, want ErrUnknown", err)
	}
	srv.Close()
	if _, err := c.Deposit(8, strings.NewReader("x")); err == nil || !strings.Contains(err.Error(), "life-weaver serve") {
		t.Errorf("service down: %v", err)
	}
}

// #101: Discover asks each address who serves there and keeps the
// answers; Choose picks the instance the scanner was started for, or
// production when it was started for none, and says what it found when
// that is not there.
func TestDiscoverAndChoose(t *testing.T) {
	h3 := service(t, three)
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"v0.14.0","instance":{"name":"bytestone","purpose":"production","slot":0}}`))
	})
	prod := httptest.NewServer(mux)
	t.Cleanup(prod.Close)
	found := Discover([]string{down.URL, h3.URL, prod.URL})
	if len(found) != 2 || found[0].Instance.Name != "h3" || found[0].URL != h3.URL || found[1].Instance.Name != "bytestone" {
		t.Fatalf("found %+v", found)
	}
	if got, err := Choose("", found); err != nil || got.URL != prod.URL {
		t.Errorf("production by default: %+v, %v", got, err)
	}
	if got, err := Choose("h3", found); err != nil || got.URL != h3.URL {
		t.Errorf("h3 by name: %+v, %v", got, err)
	}
	_, err := Choose("geni", found)
	if err == nil || !strings.Contains(err.Error(), "geni") || !strings.Contains(err.Error(), "h3 (scale)") || !strings.Contains(err.Error(), "life-weaver serve") {
		t.Errorf("an instance not serving: %v, want a refusal naming it, what was found and how to start it", err)
	}
	_, err = Choose("", found[:1])
	if err == nil || !strings.Contains(err.Error(), "production") || !strings.Contains(err.Error(), "h3 (scale)") {
		t.Errorf("no production serving: %v, want a refusal naming what was found", err)
	}
	_, err = Choose("", nil)
	if err == nil || !strings.Contains(err.Error(), "none") {
		t.Errorf("nothing serving: %v", err)
	}
}
