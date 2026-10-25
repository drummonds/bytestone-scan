package web

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// Story 054: the batch page offers Delete, confirmed on its own page, for
// a batch that is not closed; it refuses while the archive holds a photo
// of the batch. The Batches page lists every batch and the stale data no
// readable batch claims, each deletable the same way.
func TestDeleteABatch(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"a test"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	page(t, h, "/batches/20261020-1", `href="batches/20261020-1/delete"`)
	page(t, h, "/batches", "20261020-1", "a test", "open")
	page(t, h, "/batches/20261020-1/delete", "Delete batch 20261020-1", `action="batches/20261020-1/delete"`, "Keep")
	if w := post(t, h, "/batches/20261020-1/delete", nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/batches" {
		t.Fatalf("delete: %d %s to %q", w.Code, w.Body, w.Header().Get("Location"))
	}
	for _, gone := range []string{"/batches/20261020-1", loc} {
		if w := get(t, h, gone); w.Code != http.StatusNotFound {
			t.Errorf("GET %s after deleting: %d", gone, w.Code)
		}
	}
	if r, _ := a.Store.FindRunByKey(context.Background(), batch.ScanWorkflow, scanKey(loc)); r == nil || r.Status != wf.StatusCancelled {
		t.Errorf("the scan's run: %+v, want cancelled", r)
	}

	// a batch with a photo held is refused, and told what to do instead
	loc = act(t, a, h, "/batches", url.Values{"title": {"kept"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	if !strings.HasPrefix(loc, "/batches/20261020-2/") {
		t.Fatalf("the next batch is %s, want 20261020-2: a deleted batch's id is not given out again", loc)
	}
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	if w := post(t, h, "/batches/20261020-2/delete", nil); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "finish scanning") {
		t.Errorf("deleting a batch with a photo held: %d %s", w.Code, w.Body)
	}
	page(t, h, "/batches/20261020-2", "kept")
	act(t, a, h, "/batches/20261020-2/finish", nil)
	w := get(t, h, "/batches/20261020-2")
	if !strings.Contains(w.Body.String(), "Closed") || strings.Contains(w.Body.String(), "/delete") {
		t.Errorf("a closed batch's page offers Delete, or is not closed: %s", w.Body)
	}
	page(t, h, "/batches", "20261020-2", "closed")

	// stale data: listed on the Batches page, deleted the same way
	old := filepath.Join(a.Staging, "batches", "20250101-1")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "prints.yaml"), []byte("prints: []\n"), 0o644)
	os.MkdirAll(filepath.Join(a.Staging, "bags", "42"), 0o755)
	page(t, h, "/batches", "Stale", "batches/20250101-1", "bags/42", `href="stale?path=bags%2F42"`)
	page(t, h, "/stale?path=bags%2F42", "Delete bags/42", `action="stale/delete"`, `value="bags/42"`)
	if w := post(t, h, "/stale/delete", url.Values{"path": {"bags/42"}}); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/batches" {
		t.Fatalf("delete stale: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(a.Staging, "bags", "42")); !os.IsNotExist(err) {
		t.Errorf("bags/42 survives: %v", err)
	}
	if w := post(t, h, "/stale/delete", url.Values{"path": {"batches/20261020-2"}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("deleting a live batch as stale: %d", w.Code)
	}
	if w := get(t, h, "/stale?path=batches%2F20261020-2"); w.Code != http.StatusNotFound {
		t.Errorf("confirming a live batch as stale: %d", w.Code)
	}
	page(t, h, "/batches/20261020-2", "kept")
}
