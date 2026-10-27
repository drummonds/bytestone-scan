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

// #88: at the backs wait each print's back is shown close up from the
// backs preview with a tick, on unless the back is declared blank; Scan
// the backs takes the ticks with it, and an unticked back is declared
// blank and skipped, as its Blank button would.
func TestATickPerBackOnTheBacksPreview(t *testing.T) {
	s := &batchtest.Scanner{
		Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}},
		Backs:  []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 50000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 50000}},
	}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"pick the backs"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "backs/1.png") {
		t.Error("back close-ups before the backs are previewed")
	}
	act(t, a, h, loc+"/backs", nil)
	page(t, h, loc, `id="scan-backs"`, `name="select" value="1"`,
		`src="`+loc[1:]+`/backs/1.png"`, `src="`+loc[1:]+`/backs/2.png"`,
		`form="scan-backs" name="scan" value="1" checked`, `form="scan-backs" name="scan" value="2" checked`, "Scan this back")
	if w := get(t, h, loc+"/backs/2.png"); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("back close-up: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	// a back declared blank starts unticked
	act(t, a, h, loc+"/blank-back", url.Values{"photo": {"1"}, "blank": {"1"}})
	if body := get(t, h, loc).Body.String(); strings.Contains(body, `name="scan" value="2" checked`) || !strings.Contains(body, `name="scan" value="2"`) {
		t.Error("a back declared blank is still ticked, or not offered")
	}
	// ticking it again and unticking the first, then scanning
	act(t, a, h, loc+"/confirm-backs", url.Values{"select": {"1"}, "scan": {"2"}})
	page(t, h, loc, "<td>blank</td>", "35 × 35 mm")
	masters := filepath.Join(a.Staging, "batches", "20261020-1", "scans", "1", "masters")
	if _, err := os.Stat(filepath.Join(masters, "0.back.tif")); err == nil {
		t.Error("a back master for the unticked back")
	}
	if _, err := os.Stat(filepath.Join(masters, "1.back.tif")); err != nil {
		t.Error("no back master for the ticked back")
	}
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "Scan this back") {
		t.Error("the backs scanned, the ticks are still offered")
	}
}
