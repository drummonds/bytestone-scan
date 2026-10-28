package web

import (
	"net/url"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
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

// Story 113: a batch, scan or photo is reached straight from the Queue,
// its own page preferred over the generic instance page, which stays a
// step beside as Instance; each of those pages links back to its instance.
func TestTheQueuePrefersTheBatchPageAndEachPageLinksToItsInstance(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"a test"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	instance := func(typ wf.WorkflowType, key string) string {
		run, err := a.Store.FindRunByKey(t.Context(), typ, key)
		if err != nil || run == nil {
			t.Fatalf("no %s run for %s: %v", typ, key, err)
		}
		return `href="workflows/instances/` + run.ID + `"`
	}
	page(t, h, "/workflows/", `<a href="batches/20261020-1"><strong>20261020-1</strong></a>`, instance(batch.Workflow, "20261020-1")+`>Instance</a>`)
	page(t, h, "/workflows/?workflow=scan", `<a href="batches/20261020-1/scans/1"><strong>20261020-1/1</strong></a>`)
	page(t, h, "/batches/20261020-1", instance(batch.Workflow, "20261020-1")+`>Instance</a>`)
	page(t, h, "/batches/20261020-1/scans/1", instance(batch.ScanWorkflow, "20261020-1/1")+`>Instance</a>`)
	ids := get(t, h, "/workflows/?workflow=scan_photo").Body.String()
	_, after, _ := strings.Cut(ids, `<a href="photos/`)
	id, _, _ := strings.Cut(after, `"`)
	page(t, h, "/photos/"+id, instance(batch.PhotoWorkflow, id)+`>Instance</a>`)
}
