package archive

import (
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
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

const three = `[{"id":"a","name":"Ada Example","short":"Ada","born":{"date":"1894"}},{"id":"e","name":"Aaron Example"},{"id":"f","name":"Flat Example"}]`

func TestPersonInTheRegister(t *testing.T) {
	r := Archive{URL: service(t, three).URL}
	name, err := r.Person(10) // "a"
	if err != nil || name != "Ada Example" {
		t.Errorf("Person(a) = %q, %v, want Ada Example", name, err)
	}
}

func TestPersonRefusals(t *testing.T) {
	srv := service(t, three)
	r := Archive{URL: srv.URL}
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
	people, err := Archive{URL: service(t, three).URL}.People()
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
	if _, err := (Archive{URL: srv.URL}).People(); err == nil || !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("People() of an empty register: %v, want an error naming %s", err, srv.URL)
	}
}

func TestServiceNotRunningSaysHowToStartIt(t *testing.T) {
	srv := service(t, three)
	srv.Close()
	r := Archive{URL: srv.URL}
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
	if s := (Archive{URL: "http://localhost:8321"}).String(); s != "http://localhost:8321" {
		t.Errorf("String() = %q", s)
	}
}
