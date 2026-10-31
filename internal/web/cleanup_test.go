package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// #116: the Tools menu's Clean up lists the batches newest first, each
// row with Delete below, which deletes every closed or purged batch below
// that row after a confirm page saying what goes, what is kept and which
// orphans go with them; the orphans go on their own too.
func TestCleanUpDeletesBelowARow(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	day := 0
	a.Now = func() time.Time { return time.Date(2026, 10, 20+day, 15, 4, 5, 0, time.UTC) }
	closed := func() string {
		loc := act(t, a, h, "/batches", url.Values{"title": {"done"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
		act(t, a, h, loc+"/confirm", nil)
		act(t, a, h, loc+"/backs", nil)
		act(t, a, h, loc+"/confirm-backs", nil)
		describeAll(t, a, h, loc)
		act(t, a, h, loc+"/accept", nil)
		id := loc[len("/batches/") : len("/batches/")+len("20261020-1")]
		act(t, a, h, "/batches/"+id+"/finish", nil)
		day++
		return id
	}
	open := func() string {
		loc := act(t, a, h, "/batches", url.Values{"title": {"going"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
		act(t, a, h, loc+"/confirm", nil)
		day++
		return loc[len("/batches/") : len("/batches/")+len("20261020-1")]
	}
	first, kept, second, chosen, newest := closed(), open(), closed(), closed(), open()
	os.MkdirAll(filepath.Join(a.Staging, "bags", "42"), 0o755)

	page(t, h, "/about", `href="tools/cleanup"`, "Clean up")
	page(t, h, "/tools/cleanup", "Clean up", first, kept, second, chosen, newest,
		`href="tools/cleanup/below/`+chosen+`"`, `href="tools/cleanup/below/`+newest+`"`, "Delete below",
		"Orphans", "bags/42", `action="tools/cleanup/orphans"`)
	if w := get(t, h, "/tools/cleanup"); contains(w, `href="tools/cleanup/below/`+first+`"`) {
		t.Error("Delete below offered on the oldest row, with nothing below it")
	}
	page(t, h, "/tools/cleanup/below/"+chosen, "Delete below "+chosen, second, first, "2 batches", kept+"</a> stays open", "bags/42",
		`action="tools/cleanup/below/`+chosen+`"`, "Keep")
	if w := get(t, h, "/tools/cleanup/below/"+chosen); contains(w, ">"+chosen+"</a>") || contains(w, ">"+newest+"</a>") {
		t.Error("the confirm page lists the row chosen or one above it as going")
	}
	if w := post(t, h, "/tools/cleanup/below/"+chosen, nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/tools/cleanup" {
		t.Fatalf("delete below: %d %s to %q", w.Code, w.Body, w.Header().Get("Location"))
	}
	for _, gone := range []string{first, second} {
		if w := get(t, h, "/batches/"+gone); w.Code != http.StatusNotFound {
			t.Errorf("GET batch %s after clean up: %d", gone, w.Code)
		}
	}
	for _, stays := range []string{kept, chosen, newest} {
		if w := get(t, h, "/batches/"+stays); w.Code != http.StatusOK {
			t.Errorf("GET batch %s after clean up: %d", stays, w.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(a.Staging, "bags", "42")); !os.IsNotExist(err) {
		t.Errorf("the orphan survives: %v", err)
	}
	if w := get(t, h, "/tools/cleanup/below/20250101-1"); w.Code != http.StatusNotFound {
		t.Errorf("confirm for a batch that is not there: %d", w.Code)
	}

	// orphans on their own
	os.MkdirAll(filepath.Join(a.Staging, "renditions", "43"), 0o755)
	if w := post(t, h, "/tools/cleanup/orphans", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("remove orphans: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(a.Staging, "renditions", "43")); !os.IsNotExist(err) {
		t.Errorf("the orphan survives: %v", err)
	}
	if w := get(t, h, "/tools/cleanup"); contains(w, "Orphans") {
		t.Error("the Clean up page still has an Orphans section with none")
	}
}

func contains(w *httptest.ResponseRecorder, s string) bool {
	return strings.Contains(w.Body.String(), s)
}
