package web

import (
	"net/url"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// Story 097: the queue lists batches alone; a batch's scans and photos
// are reached through its instance page, or by asking the filter for
// their workflow.
func TestTheQueueListsBatchesAlone(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"a test"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil) // the scan's run and its photo's exist now
	row := func(key string) string { return "<strong>" + key + "</strong>" }
	page(t, h, "/workflows/", row("20261020-1"))
	if w := get(t, h, "/workflows/"); strings.Contains(w.Body.String(), row("20261020-1/1")) || strings.Contains(w.Body.String(), "scan_photo</code>") {
		t.Errorf("the queue lists a scan or a photo:\n%s", w.Body)
	}
	page(t, h, "/workflows/?workflow=scan", row("20261020-1/1"))
	page(t, h, "/workflows/?workflow=scan_photo", "scan_photo</code>")
}
