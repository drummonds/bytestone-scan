package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// #78: a print's back is declared blank on the scan page, per print, at
// the backs preview or at the quality check; that photo goes on without a
// back while the others' backs are scanned, and its page says so.
func TestABlankBackPerPrint(t *testing.T) {
	s := &batchtest.Scanner{
		Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}},
		Backs:  []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 50000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 50000}},
	}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"one blank"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil) // the backs preview: each back found, waiting to be confirmed
	page(t, h, loc, "found 40 × 30 mm", `action="`+loc[1:]+`/blank-back"`, `name="photo" value="1"`, ">Blank<")
	if w := post(t, h, loc+"/blank-back", url.Values{"photo": {"1"}, "blank": {"1"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("blank back: %d %s", w.Code, w.Body)
	}
	a.wait()
	page(t, h, loc, "<td>blank", ">Not blank<")
	act(t, a, h, loc+"/confirm-backs", nil)
	page(t, h, loc, "<td>blank</td>", "40 × 30 mm", ">Blank<")
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "Not blank") {
		t.Error("the backs scanned, a blank back still offered as not blank")
	}
	if _, err := os.Stat(filepath.Join(a.Staging, "batches", "20261020-1", "scans", "1", "masters", "1.back.tif")); err == nil {
		t.Error("a back master for the blank back")
	}
	page(t, h, "/photos/1", "blank")
	if body := get(t, h, "/photos/1").Body.String(); strings.Contains(body, `name="back"`) {
		t.Error("the describe form asks what a blank back says")
	}

	// at the quality check, a back already scanned is discarded
	if w := post(t, h, loc+"/blank-back", url.Values{"photo": {"0"}, "blank": {"1"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("blank back at the quality check: %d %s", w.Code, w.Body)
	}
	a.wait()
	if _, err := os.Stat(filepath.Join(a.Staging, "batches", "20261020-1", "scans", "1", "masters", "0.back.tif")); err == nil {
		t.Error("the discarded back master is still there")
	}
	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "Done")
	if _, err := os.Stat(filepath.Join(a.Staging, "bags", "0", "data", "back.tif")); err == nil {
		t.Error("bag 0 has a back.tif")
	}
}
