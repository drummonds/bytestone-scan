package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// #80: Keep for diagnosis is on the scan page at every state, with a note
// and the choice to include the masters; what is kept is listed on the
// Batches page beside the stale data, with a Discard, and on the scan's
// own page.
func TestKeepForDiagnosis(t *testing.T) {
	a := live(t, &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}})
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"gone wrong"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil) // at check quality: the masters are the scan's
	l, err := batch.OpenScan(a.Staging, scanKey(loc))
	if err != nil {
		t.Fatal(err)
	}
	page(t, h, loc, `action="`+scanURL(l.Key())+`/keep"`, `name="note"`, `name="masters"`, "Keep for diagnosis")
	if w := post(t, h, loc+"/keep", url.Values{"note": {"the right print was cut"}}); w.Code != http.StatusSeeOther || w.Header().Get("Location") != loc {
		t.Fatalf("keep: %d %s %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	list, err := batch.ListDiagnoses(a.Staging)
	if err != nil || len(list) != 1 || list[0].Note != "the right print was cut" || list[0].Scan != l.Key() || list[0].State != "waiting at check quality" || list[0].Version == "" {
		t.Fatalf("kept %+v, %v", list, err)
	}
	name := list[0].Name
	if tifs, _ := filepath.Glob(filepath.Join(a.Staging, "diagnosis", name, "masters", "*.tif")); len(tifs) != 0 {
		t.Errorf("masters kept without being asked for: %v", tifs)
	}
	page(t, h, loc, "Kept for diagnosis", name, "the right print was cut")
	page(t, h, "/batches", "Kept for diagnosis", name, "the right print was cut", `href="`+scanURL(l.Key())+`"`, `action="diagnosis/discard"`, `value="`+name+`"`)

	if w := post(t, h, loc+"/keep", url.Values{"note": {"with masters"}, "masters": {"on"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("keep with masters: %d %s", w.Code, w.Body)
	}
	list, _ = batch.ListDiagnoses(a.Staging)
	if len(list) != 2 || !list[0].Masters {
		t.Fatalf("kept with masters: %+v", list)
	}
	if tifs, _ := filepath.Glob(filepath.Join(a.Staging, "diagnosis", list[0].Name, "masters", "*.tif")); len(tifs) == 0 {
		t.Error("masters asked for, none kept")
	}

	if w := post(t, h, "/diagnosis/discard", url.Values{"name": {name}}); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/batches" {
		t.Fatalf("discard: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(a.Staging, "diagnosis", name)); !os.IsNotExist(err) {
		t.Error("discarded, still there")
	}
	if w := post(t, h, "/diagnosis/discard", url.Values{"name": {"../batches"}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("discarding outside diagnosis/: %d", w.Code)
	}
	if body := get(t, h, "/batches").Body.String(); strings.Contains(body, `value="`+name+`"`) {
		t.Error("the discarded diagnosis is still listed")
	}
}

// While a step of the scan runs, Keep waits for it: the files are being
// written.
func TestKeepWaitsForTheStep(t *testing.T) {
	sc := hold(&batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}})
	a := live(t, sc.Scanner)
	a.Scanner = sc
	h := a.Handler()
	w := post(t, h, "/batches", url.Values{"title": {"held"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	loc := w.Header().Get("Location")
	<-sc.started
	if w := post(t, h, loc+"/keep", url.Values{"note": {"stuck"}}); w.Code != http.StatusConflict {
		t.Errorf("keep during a step: %d %s", w.Code, w.Body)
	}
	close(sc.release)
	a.wait()
	if w := post(t, h, loc+"/keep", url.Values{"note": {"after"}}); w.Code != http.StatusSeeOther {
		t.Errorf("keep after the step: %d %s", w.Code, w.Body)
	}
	if list, _ := batch.ListDiagnoses(a.Staging); len(list) != 1 || list[0].State == "" {
		t.Errorf("kept %+v", list)
	}
}
