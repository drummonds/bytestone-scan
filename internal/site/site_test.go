package site

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// #143: a site answers whether it has an item's page, at
// /<home>/items/<id>/, in the ids' URL form.
func TestStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/a/items/k/" {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s := Site{Name: "i.bytestone.uk", URL: srv.URL}
	if got := s.Item(10, 19); got != srv.URL+"/a/items/k/" {
		t.Errorf("Item: %s", got)
	}
	if got := s.Status(10, 19); got != Published {
		t.Errorf("held item: %s, want published", got)
	}
	if got := s.Status(10, 20); got != NotYet {
		t.Errorf("item not built yet: %s, want not yet", got)
	}
	srv.Close()
	if got := s.Status(10, 19); got != Unreachable {
		t.Errorf("site down: %s, want unreachable", got)
	}
}

func TestDefaults(t *testing.T) {
	d := Defaults()
	if len(d) != 2 || d[0].URL != "https://i.bytestone.uk" || d[1].URL != "https://i.bytestone.org" {
		t.Errorf("defaults %+v", d)
	}
}
