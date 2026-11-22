package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/site"
)

// Story 191, Home now: Verify and the photo's page show and link the
// home the archive gives, and say what the item was deposited under when
// that was another; an item whose record changed since is verified.
func TestThePagesShowTheHomeNow(t *testing.T) {
	a, l := scanned(t)
	svc := a.Archive.(*archivetest.Fake)
	uk := fakeSite(t, "i.bytestone.uk", "/b/items/0/", "/b/items/1/")
	a.Sites = []site.Site{uk}
	h := a.Handler()
	verify := "/batches/" + l.Batch + "/scans/1/verify"

	w := get(t, h, verify)
	if strings.Contains(w.Body.String(), "deposited under") {
		t.Error("an item homed as it was sent says what it was deposited under")
	}
	page(t, h, "/photos/0", "Ada Example (a)")

	svc.Renumber(10, 11)
	page(t, h, verify, "<td>verified</td>", "<td>b, <em>deposited under a</em></td>",
		`<a href="`+uk.URL+`/b/items/0/">i.bytestone.uk</a> <span class="tag is-success">published</span>`,
		`is-success">Verified: 2 photos held by the archive, every master as receipted; the record of 2 has changed at the archive since the deposit.`)
	page(t, h, "/photos/0", "Ada Example (b), <em>deposited under a</em>")

	svc.Down = true
	page(t, h, "/photos/0", "Ada Example (a), as sent: the archive could not be asked")
}

// Story 191: an open batch whose person the register does not hold
// refuses a new scan, import, feed or film import, reserving nothing,
// and offers Re-home: the person picker.
func TestABatchWhosePersonIsGoneOffersRehome(t *testing.T) {
	a, l := scanned(t)
	svc := a.Archive.(*archivetest.Fake)
	h := a.Handler()
	at := "/batches/" + l.Batch

	if w := get(t, h, at); strings.Contains(w.Body.String(), "Re-home") {
		t.Error("Re-home offered on a batch whose person the register holds")
	}
	if w := get(t, h, at+"/rehome"); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("the Re-home form of a batch whose person the register holds: %d", w.Code)
	}

	svc.Renumber(10, 11)
	next := svc.Next
	page(t, h, at, "person a is not in the register", `href="batches/`+l.Batch+`/rehome"`)
	refused := func(what string, code int, body string) {
		t.Helper()
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, "person a is not in the register") || !strings.Contains(body, `href="batches/`+l.Batch+`/rehome"`) {
			t.Errorf("%s: %d, want refused, person a not in the register, Re-home offered", what, code)
		}
	}
	for _, form := range []string{"/scans/new", "/imports/new", "/feeds/new", "/films/new"} {
		w := get(t, h, at+form)
		refused("GET "+form, w.Code, w.Body.String())
	}
	for _, to := range []string{"/scans", "/feeds"} {
		w := post(t, h, at+to, url.Values{"description": {"more"}})
		refused("POST "+to, w.Code, w.Body.String())
	}
	w := upload(t, h, at+"/imports", url.Values{"description": {"a page"}, "sender": {"Cousin Mary"}}, map[string][]byte{"page.png": pageOf(t, 100, 80, [4]float64{10, 10, 60, 50})})
	refused("POST /imports", w.Code, w.Body.String())
	w = uploadFilm(t, h, at+"/films", "film.mkv", []byte("a film"))
	refused("POST /films", w.Code, w.Body.String())
	if scans, _ := l.Sitting().Scans(); len(scans) != 1 || svc.Next != next {
		t.Errorf("something was begun: %d scans, the archive's next id %s, was %s", len(scans), svc.Next, next)
	}

	page(t, h, at+"/rehome", "Re-home batch "+l.Batch, `<option value="b">Ada Example (b)</option>`)
	for _, person := range []string{"", "0", "c"} {
		if w := post(t, h, at+"/rehome", url.Values{"person": {person}}); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("re-homed with %q: %d, want refused", person, w.Code)
		}
	}
	if w := post(t, h, at+"/rehome", url.Values{"person": {"b"}}); w.Code != http.StatusSeeOther || w.Header().Get("Location") != at {
		t.Fatalf("re-home: %d %s", w.Code, w.Body)
	}
	page(t, h, at, "Ada Example (b)")
	if w := get(t, h, at); strings.Contains(w.Body.String(), "Re-home") {
		t.Error("Re-home still offered once the batch is re-homed")
	}
	page(t, h, at+"/scans/new", "one more scan")
}

// Story 191, No id: 0 is not offered or taken as a home, and a batch
// from before that names it is listed as naming an id that is gone.
func TestZeroIsNoHome(t *testing.T) {
	a, l := scanned(t)
	svc := a.Archive.(*archivetest.Fake)
	svc.Register = append(svc.Register, archive.Person{ID: 0, Name: "Nobody"})
	a.Now = func() time.Time { return time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC) }
	h := a.Handler()
	if w := get(t, h, "/batches/new"); strings.Contains(w.Body.String(), "Nobody") {
		t.Error("the person picker offers 0")
	}
	w := post(t, h, "/batches", url.Values{"title": {"a shoebox"}, "person": {"0"}, "credit": {"Drummond family collection"}, "ppi": {"600"}, "then": {"import"}})
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "0 is no id") {
		t.Errorf("a batch homed with 0: %d, want refused, 0 is no id", w.Code)
	}

	yaml := filepath.Join(a.Staging, "batches", l.Batch, "batch.yaml")
	raw, _ := os.ReadFile(yaml)
	os.WriteFile(yaml, []byte(strings.Replace(string(raw), "person: a\n", `person: "0"`+"\n", 1)), 0o644)
	if b, err := batch.Open(a.Staging, l.Batch); err != nil || b.Person != 0 {
		t.Fatalf("a batch.yaml naming person 0: %+v %v", b, err)
	}
	page(t, h, "/batches", "<td>0, no longer an id</td>")
	page(t, h, "/batches/"+l.Batch, "0, no longer an id", `href="batches/`+l.Batch+`/rehome"`)
}
