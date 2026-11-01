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

// #132: the batch page shows the batch's steps as the workflow instance
// has them, links to that instance, and stands each scan where its photos
// stand, so that what holds Finish scanning back is in plain sight, with
// a thumbnail of each photo and the batch's folder in staging.
func TestTheBatchPageShowsItsStepsAndGates(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000},
		{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000},
	}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"Shoebox"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	page(t, h, "/batches/20261020-1",
		`<td>describe</td><td><span class="tag is-success">completed</span></td>`,
		`<td>scans</td><td><span class="tag is-info">waiting</span></td>`,
		`<td>close</td><td><span class="tag is-light">not run</span></td>`,
		`>Workflow instance</a>`,
		`<td>waiting at confirm</td>`,
		`<code>`+filepath.Join(a.Staging, "batches", "20261020-1")+`</code>`)
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	describeAll(t, a, h, loc)
	l, _ := batch.OpenScan(a.Staging, "20261020-1/1")
	_, ids, _ := l.Found()
	stale := ids[0]
	if err := os.WriteFile(filepath.Join(l.Renditions(stale), "recipe.yaml"), []byte("version: 1\nmaster: 0000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	act(t, a, h, loc+"/accept", nil)
	failed := "photo " + stale.String() + " failed at deposit: "
	page(t, h, "/batches/20261020-1",
		`<td>`+failed,
		`Finish scanning waits for scan 1: `+failed,
		`<a href="photos/`+ids[1].String()+`"><img src="photos/`+ids[1].String()+`/renditions/2/tiny.jpg"`)
	if w := post(t, h, "/batches/20261020-1/finish", nil); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), failed) {
		t.Errorf("finish: %d %s, want the photo named", w.Code, w.Body)
	}
	page(t, h, "/photos/"+stale.String(), "Not deposited: failed at deposit: ")
}
