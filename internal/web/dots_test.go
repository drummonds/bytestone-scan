package web

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// #135: the batch page draws its run as a row of dots and a row per scan
// with a dot per photo; the scan page draws its own run and its photos;
// a finished scan whose photo failed says which and offers a retry.
func TestProgressDotsOnTheBatchAndScanPages(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000},
		{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000},
	}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"Shoebox"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	// the batch's own run, and the scan with no photos yet as one dot
	page(t, h, "/batches/20261020-1",
		`<a class="dots" href="workflows/instances/`,
		`title="describe: completed"`,
		`title="scans: waiting"`,
		`title="close: not run"`,
		`<a class="dots" href="`+loc[1:]+`" title="waiting at confirm">`)
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	describeAll(t, a, h, loc)
	l, _ := batch.OpenScan(a.Staging, "20261020-1/1")
	_, ids, _ := l.Found()
	// the scan's run, then a dot per photo, on the scan page and the batch page
	page(t, h, loc,
		`<a class="dots" href="workflows/instances/`,
		`title="confirm: completed"`,
		`<a href="photos/`+ids[0].String()+`" title="photo `+ids[0].String()+`: waiting"`,
		`<a href="photos/`+ids[1].String()+`" title="photo `+ids[1].String()+`: waiting"`)
	page(t, h, "/batches/20261020-1",
		`<a href="photos/`+ids[1].String()+`" title="photo `+ids[1].String()+`: waiting"`)
	// a photo whose deposit fails is a red dot, and the scan page says so
	stale := ids[0]
	if err := os.WriteFile(filepath.Join(l.Renditions(stale), "recipe.yaml"), []byte("version: 1\nmaster: 0000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	act(t, a, h, loc+"/accept", nil)
	red := `<a href="photos/` + stale.String() + `" title="photo ` + stale.String() + `: failed"><span class="dot" style="display:inline-block;width:.75em;height:.75em;margin-right:.2em;border-radius:50%;border:1px solid #7a7a7a;background:#f14668"></span></a>`
	page(t, h, "/batches/20261020-1", red, `title="scans: waiting"`)
	page(t, h, loc, red,
		`<div class="notification is-danger">Photo `+stale.String()+` failed at deposit: `,
		`<form action="`+loc[1:]+`/retry" method="post" style="display:inline"><button class="button is-primary" type="submit">Retry</button></form>`)
	act(t, a, h, loc+"/retry", nil)
	page(t, h, loc, "Done: 2 photos held by the archive")
}
