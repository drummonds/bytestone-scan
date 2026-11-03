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

// #139: every page names the definition it is an instance of and shows
// its steps with their status, error and action: Back to a step the scan
// can go back to, Retry on a failed step; its parent above and its
// children below. A failure names the step once and the item not at all.
func TestStepsErrorsAndGoBacksOnEveryPage(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000},
		{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000},
	}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"Shoebox"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	redo := func(step string) string {
		return `<form action="` + loc[1:] + `/redo" method="post" style="display:inline"><input type="hidden" name="step" value="` + step + `"><button class="button is-small" type="submit">Back to ` + step + `</button></form>`
	}
	// the scan at confirm: its definition, its parent, a go-back to the
	// preview, nothing on the waiting step or on find prints
	page(t, h, loc,
		`Steps of <a href="workflows/definitions/scan">Scan the bed once</a> `,
		`<p>Parent: <a href="batches/20261020-1">batch 20261020-1</a> at scans <span class="tag is-info">waiting</span></p>`,
		`<td>preview</td><td><span class="tag is-success">completed</span></td><td></td><td>`+redo("preview")+`</td>`,
		`<td>find prints</td><td><span class="tag is-success">completed</span></td><td></td><td></td>`,
		`<td>confirm</td><td><span class="tag is-info">waiting</span></td><td></td><td></td>`)
	page(t, h, "/batches/20261020-1",
		`Steps of <a href="workflows/definitions/scan_batch">Archive photographs</a> `,
		`<td>scans</td><td><span class="tag is-info">waiting</span></td><td></td><td></td>`)
	// back to the preview from the table finds the print added since
	s.Prints = append(s.Prints, batchtest.Print{X0: 20, Y0: 70, X1: 60, Y1: 100, Value: 30000})
	act(t, a, h, loc+"/redo", url.Values{"step": {"preview"}})
	if s.Previews != 2 {
		t.Errorf("previews %d, want 2", s.Previews)
	}
	page(t, h, loc, "<td>3</td>")
	// only the five spots are go-backs
	for _, step := range []string{"describe", "find prints", "check quality", "", "no such step"} {
		if w := post(t, h, loc+"/redo", url.Values{"step": {step}}); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("redo from %q: %d, want 422", step, w.Code)
		}
	}
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	describeAll(t, a, h, loc)
	l, _ := batch.OpenScan(a.Staging, "20261020-1/1")
	_, ids, _ := l.Found()
	// the photo page: its definition, its parent the scan, its steps; the
	// scan page lists its photos with their step
	page(t, h, "/photos/"+ids[0].String(),
		`Steps of <a href="workflows/definitions/scan_photo">Archive one photo</a> `,
		`<p>Parent: <a href="`+loc[1:]+`">scan 20261020-1/1</a> at check quality <span class="tag is-info">waiting</span></p>`,
		`<td>develop front</td><td><span class="tag is-success">completed</span></td><td></td><td></td>`,
		`<td>describe</td><td><span class="tag is-light">not run</span></td><td></td><td></td>`,
		`<td>accepted</td><td><span class="tag is-info">waiting</span></td><td></td><td></td>`)
	page(t, h, loc,
		`<td><a href="photos/`+ids[1].String()+`">photo `+ids[1].String()+`</a></td><td>accepted</td><td><span class="tag is-info">waiting</span></td><td></td>`,
		redo("backs"), redo("backs preview"), redo("fronts"), redo("confirm"))
	// a photo refused at deposit: the step's error once on its row and in
	// the archive section, the item never, Retry on both; the scan is
	// packaged, so nothing before is redone
	stale := ids[0]
	if err := os.WriteFile(filepath.Join(l.Renditions(stale), "recipe.yaml"), []byte("version: 1\nmaster: 0000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	act(t, a, h, loc+"/accept", nil)
	retry := `<form action="photos/` + stale.String() + `/retry" method="post" style="display:inline"><button class="button is-small is-primary" type="submit">Retry</button></form>`
	page(t, h, "/photos/"+stale.String(),
		`<td>package</td><td><span class="tag is-success">completed</span></td><td></td><td>final</td>`,
		`<td>deposit</td><td><span class="tag is-danger">failed</span></td><td>refused for its renditions: the archive wants `,
		`Retry deposits again</td><td>`+retry+`</td>`,
		`<p class="has-text-danger">Not deposited: failed at deposit: refused for its renditions: the archive wants `,
		`<div class="buttons">`+retry+`</div>`)
	for _, noise := range []string{"deposit: item", "depositing item", "item " + stale.String() + ":"} {
		if body := get(t, h, "/photos/"+stale.String()).Body.String(); strings.Contains(body, noise) {
			t.Errorf("the photo page says %q", noise)
		}
	}
	page(t, h, loc,
		`<td><a href="photos/`+stale.String()+`">photo `+stale.String()+`</a></td><td>deposit</td><td><span class="tag is-danger">failed</span></td><td>refused for its renditions: the archive wants `,
		`<td>preview</td><td><span class="tag is-success">completed</span></td><td></td><td>archived and final</td>`)
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "Back to ") {
		t.Error("the scan page offers a go-back after a photo is packaged")
	}
	// retry from the photo page lands back on the photo, held
	if to := act(t, a, h, "/photos/"+stale.String()+"/retry", nil); to != "/photos/"+stale.String() {
		t.Errorf("retry went to %q", to)
	}
	page(t, h, "/photos/"+stale.String(), "Held by the archive")
}
