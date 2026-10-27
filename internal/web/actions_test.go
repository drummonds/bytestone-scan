package web

import (
	"context"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

func live(t *testing.T, s *batchtest.Scanner) *App {
	t.Helper()
	return &App{
		Staging: t.TempDir(),
		Store:   wf.NewMemStore(),
		Scanner: s,
		Archive: archivetest.Example(),
		Now:     func() time.Time { return time.Date(2026, 10, 20, 15, 4, 5, 0, time.UTC) },
	}
}

func post(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// act posts an action and waits for the step it starts to finish.
func act(t *testing.T, a *App, h http.Handler, path string, form url.Values) string {
	t.Helper()
	w := post(t, h, path, form)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST %s: %d %s", path, w.Code, w.Body)
	}
	a.wait()
	return w.Header().Get("Location")
}

func TestABatchFromStartToTheArchive(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	page(t, h, "/batches/new", `name="title"`, `name="person"`, `name="ppi"`, `name="provenance"`, `name="credit"`)

	loc := act(t, a, h, "/batches", url.Values{"title": {"SS Bonita"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	if loc != "/batches/20261020-1/scans/1" {
		t.Fatalf("new batch went to %q", loc)
	}
	page(t, h, loc, "Confirm", "Rescan the preview", "Preview, 150 ppi")

	s.Prints = append(s.Prints, batchtest.Print{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000})
	act(t, a, h, loc+"/preview", nil)
	if s.Previews != 2 {
		t.Errorf("previews %d, want 2", s.Previews)
	}
	page(t, h, loc, "<td>2</td>") // the second print is in the table

	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	page(t, h, loc, "Accept", "Rescan at", `href="photos/0"`, `href="photos/1"`)
	page(t, h, "/photos/1", "Master, 600 ppi,") // the quality page works before packaging

	act(t, a, h, loc+"/rescan", url.Values{"ppi": {"1200"}})
	if s.LastPPI != 1200 || s.Details != 3 {
		t.Errorf("rescan: last ppi %d after %d passes, want 1200 after 3", s.LastPPI, s.Details)
	}
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)

	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	if !bag.Complete(filepath.Join(a.Staging, "bags"), 0) || !bag.Complete(filepath.Join(a.Staging, "bags"), 1) {
		t.Error("photos not archived after accept")
	}
	page(t, h, loc, "Done")
}

func TestRetryAfterAFailure(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}, FailDetails: 1}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"SS Bonita"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	page(t, h, loc, "USB disconnected", "Retry")
	act(t, a, h, loc+"/retry", nil)
	page(t, h, loc, "Preview the backs")
}

func TestNewBatchRefusals(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	for _, f := range []url.Values{
		{"title": {""}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}},
		{"title": {"x"}, "person": {"c"}, "ppi": {"600"}, "credit": {"© Example"}},
		{"title": {"x"}, "person": {""}, "ppi": {"600"}, "credit": {"© Example"}},
		{"title": {"x"}, "person": {"a"}, "ppi": {"300"}, "credit": {"© Example"}},
		{"title": {"x"}, "person": {"a"}, "ppi": {"600"}, "credit": {" "}},
	} {
		if w := post(t, h, "/batches", f); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("new batch %v: %d, want 422", f, w.Code)
		}
	}
}

// slow holds the preview until released, so a step can be caught running:
// started is closed once the preview has been asked for.
type slow struct {
	*batchtest.Scanner
	release, started chan struct{}
	once             *sync.Once
}

func hold(s *batchtest.Scanner) slow {
	return slow{s, make(chan struct{}), make(chan struct{}), new(sync.Once)}
}

func (s slow) Preview(ctx context.Context) (image.Image, error) {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return s.Scanner.Preview(ctx)
}

func (s slow) Detail(ctx context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	return s.Scanner.Detail(ctx, area, ppi)
}

func TestOneStepAtATime(t *testing.T) {
	sc := hold(&batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}})
	a := live(t, sc.Scanner)
	a.Scanner = sc
	h := a.Handler()
	w := post(t, h, "/batches", url.Values{"title": {"first"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	loc := w.Header().Get("Location")
	page(t, h, loc, "Working")

	if w := post(t, h, "/batches", url.Values{"title": {"second"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "busy") {
		t.Errorf("second batch while scanning: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(a.Staging, "batches", "20261020-2")); !os.IsNotExist(err) {
		t.Error("a refused batch was still recorded")
	}
	close(sc.release)
	a.wait()
	page(t, h, loc, "Confirm")
}

// A scan left waiting can be cancelled from the queue, one at a time.
func TestCancelAScanFromTheQueue(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"abandoned"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	runs, _ := a.Store.ListRuns(context.Background(), wf.RunFilter{Types: []wf.WorkflowType{batch.ScanWorkflow}})
	if len(runs) != 1 || runs[0].Status != wf.StatusWaiting {
		t.Fatalf("runs %+v, want one scan waiting at confirm", runs)
	}
	action := `action="workflows/instances/` + runs[0].ID + `/cancel"`
	page(t, h, "/workflows/", action)
	if w := post(t, h, "/workflows/instances/"+runs[0].ID+"/cancel", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}
	if run, _ := a.Store.GetRun(context.Background(), runs[0].ID); run.Status != wf.StatusCancelled {
		t.Errorf("status %s, want cancelled", run.Status)
	}
	if strings.Contains(get(t, h, "/workflows/").Body.String(), action) {
		t.Error("a cancelled scan is still offered to cancel")
	}
}

func TestNewBatchSaysTheScanIsAveragedDownToTheMaster(t *testing.T) {
	h := live(t, &batchtest.Scanner{}).Handler()
	page(t, h, "/batches/new",
		"averaged down to each master’s ppi: 600, or 1200 for prints smaller than 6 × 4 inches",
		"lowers noise", "the master is the same size")
}

func TestNewBatchChoosesTheHomePersonByName(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	a.Archive = archivetest.New(archive.Person{ID: 11, Name: "Zak Example"}, archive.Person{ID: 10, Name: "Ada Example"})
	h := a.Handler()
	body := get(t, h, "/batches/new").Body.String()
	ada := strings.Index(body, `<option value="a">Ada Example (a)</option>`)
	zak := strings.Index(body, `<option value="b">Zak Example (b)</option>`)
	if ada < 0 || zak < ada {
		t.Errorf("want Ada (a) then Zak (b) offered as home person:\n%s", body)
	}
	if w := post(t, h, "/batches", url.Values{"title": {"x"}, "person": {""}, "ppi": {"600"}, "credit": {"© Example"}}); !strings.Contains(w.Body.String(), "choose the home person") {
		t.Errorf("no person chosen: want it asked for:\n%s", w.Body)
	}
	// a refused batch offers the people again, with its choice kept
	w := post(t, h, "/batches", url.Values{"title": {""}, "person": {"b"}, "ppi": {"600"}, "credit": {"© Example"}})
	if !strings.Contains(w.Body.String(), `<option value="b" selected>Zak Example (b)</option>`) {
		t.Errorf("refused form does not keep the person chosen:\n%s", w.Body)
	}
}

// Story 014: provenance and credit are typed once per shoebox, not per
// scan of the bed.
func TestNewBatchIsPrefilledFromTheLatestBatch(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	body := get(t, h, "/batches/new").Body.String()
	if !strings.Contains(body, `name="provenance" value=""`) || !strings.Contains(body, `name="credit" value=""`) {
		t.Errorf("first batch: want blank provenance and credit:\n%s", body)
	}
	act(t, a, h, "/batches", url.Values{"title": {"one"}, "person": {"a"}, "ppi": {"600"},
		"provenance": {"Aunt Jean's shoebox"}, "credit": {"Drummond family collection, CC BY 4.0"}})
	page(t, h, "/batches/new", `name="provenance" value="Aunt Jean&#39;s shoebox"`, `name="credit" value="Drummond family collection, CC BY 4.0"`)
	// a refused form keeps what was typed, not the latest batch's
	w := post(t, h, "/batches", url.Values{"title": {""}, "person": {"a"}, "ppi": {"600"}, "provenance": {"box two"}, "credit": {"© Other"}})
	if !strings.Contains(w.Body.String(), `name="provenance" value="box two"`) || !strings.Contains(w.Body.String(), `name="credit" value="© Other"`) {
		t.Errorf("refused form does not keep what was typed:\n%s", w.Body)
	}
	page(t, h, "/batches/20261020-1", "Provenance", "Aunt Jean&#39;s shoebox", "Credit", "Drummond family collection, CC BY 4.0")
}

func TestNewBatchWithNoPeopleIsRefused(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	a.Archive = archivetest.New()
	w := get(t, a.Handler(), "/batches/new")
	if body := w.Body.String(); !strings.Contains(body, a.Archive.String()) || strings.Contains(body, "<form") {
		t.Errorf("new batch with no people: want the register named and no form:\n%s", body)
	}
}

// #37: a working batch's page shows the step running and how long it
// should take, redrawing only that, and says so from the batches before.
func TestAWorkingBatchShowsItsProgressWithoutRedrawing(t *testing.T) {
	sc := hold(&batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}})
	a := live(t, sc.Scanner)
	a.Scanner = sc
	h := a.Handler()
	loc := post(t, h, "/batches", url.Values{"title": {"first"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}}).Header().Get("Location")
	<-sc.started
	body := get(t, h, loc).Body.String()
	if strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("the page redraws itself")
	}
	for _, s := range []string{`id="progress"`, "Preview", loc[1:] + "/progress"} {
		if !strings.Contains(body, s) {
			t.Errorf("working page lacks %q:\n%s", s, body)
		}
	}
	page(t, h, loc+"/progress", "Preview", "<progress")
	if strings.Contains(get(t, h, loc+"/progress").Body.String(), "of about") {
		t.Error("first preview ever: nothing to expect from")
	}
	close(sc.release)
	a.wait()
	if w := get(t, h, loc+"/progress"); w.Code != http.StatusNoContent {
		t.Errorf("idle progress: %d, want 204", w.Code)
	}
	// the next batch's preview is expected to take about as long as this one's
	sc2 := hold(sc.Scanner)
	a.Scanner = sc2
	loc2 := post(t, h, "/batches", url.Values{"title": {"second"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}}).Header().Get("Location")
	<-sc2.started
	page(t, h, loc2+"/progress", "Preview", "of about")
	close(sc2.release)
	a.wait()
}

// #36: the usual rights line is typed without hunting for the sign.
func TestNewBatchHasButtonsThatInsertTheCreditTerms(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	page(t, a.Handler(), "/batches/new", `data-insert="©"`, `data-insert="CC BY 4.0"`, `name="credit"`)
}

// #38: a batch is a sitting of scans. New batch starts its first scan; the
// batch page lists the scans; another scan is described from the batch's
// title unless the operator says otherwise; scanning is finished once
// every scan is through; and each photo names its batch and scan.
func TestABatchOfScans(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	one := act(t, a, h, "/batches", url.Values{"title": {"Shoebox"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	if one != "/batches/20261020-1/scans/1" {
		t.Fatalf("new batch went to %s", one)
	}
	page(t, h, one, "Scan 1", "Shoebox", "Confirm", `href="batches/20261020-1"`)
	page(t, h, "/batches/20261020-1", "Shoebox", `href="batches/20261020-1/scans/1"`, "Do one more scan", "Finish scanning")
	if w := post(t, h, "/batches/20261020-1/finish", nil); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "scan 1 is not through") {
		t.Errorf("finished with a scan in hand: %d %s", w.Code, w.Body)
	}

	page(t, h, "/batches/20261020-1/scans/new", `name="description" value="Shoebox"`)
	two := act(t, a, h, "/batches/20261020-1/scans", url.Values{"description": {"the second envelope"}})
	if two != "/batches/20261020-1/scans/2" {
		t.Fatalf("new scan went to %s", two)
	}
	page(t, h, "/batches/20261020-1", "the second envelope", `href="batches/20261020-1/scans/2"`)

	for _, loc := range []string{one, two} {
		act(t, a, h, loc+"/confirm", nil)
		act(t, a, h, loc+"/backs", nil)
		act(t, a, h, loc+"/confirm-backs", nil)
		describeAll(t, a, h, loc)
		act(t, a, h, loc+"/accept", nil)
	}
	if loc := act(t, a, h, "/batches/20261020-1/finish", nil); loc != "/batches/20261020-1" {
		t.Errorf("finish went to %s", loc)
	}
	page(t, h, "/batches/20261020-1", "Closed")
	if w := get(t, h, "/batches/20261020-1/scans/new"); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("a finished batch offered another scan: %d", w.Code)
	}
	page(t, h, "/photos/1", "the second envelope", `href="batches/20261020-1"`, `href="batches/20261020-1/scans/2"`)
}

// Story 039: after the fronts the scan asks for the prints to be turned
// over, scans the backs where the fronts lay, shows where each back lay
// and flags one that differs; the photo page shows the back; the backs
// can be scanned again.
func TestTheBacksOfAScan(t *testing.T) {
	s := &batchtest.Scanner{
		Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}},
		Backs:  []batchtest.Print{{X0: 22, Y0: 20, X1: 62, Y1: 50, Value: 50000}, {X0: 90, Y0: 25, X1: 115, Y1: 60, Value: 50000}},
	}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"two sided"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	page(t, h, loc, "Turn each print over", "Preview the backs")
	if w := post(t, h, loc+"/accept", nil); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "backs") {
		t.Errorf("accepted before the backs: %d %s", w.Code, w.Body)
	}
	// #77: previewed and found, the backs wait to be confirmed before they are scanned
	act(t, a, h, loc+"/backs", nil)
	if s.Details != 1 {
		t.Errorf("%d detail passes after the backs preview, want 1: the backs are not scanned until confirmed", s.Details)
	}
	page(t, h, loc, "Check that each back lies where its front did", ">Scan the backs ›<", ">Preview the backs again<", ">‹ Back to the prints<", loc[1:]+"/backs.png", "was the print turned over in place?")
	if w := post(t, h, loc+"/accept", nil); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("accepted before the backs were confirmed: %d %s", w.Code, w.Body)
	}
	act(t, a, h, loc+"/preview-backs", nil)
	if s.Previews != 3 || s.Details != 1 {
		t.Errorf("previews %d, details %d after previewing the backs again; want 3 and 1", s.Previews, s.Details)
	}
	act(t, a, h, loc+"/confirm-backs", nil)
	if s.LastPPI != 300 || s.Details != 2 {
		t.Errorf("backs: last ppi %d after %d passes, want 300 after 2", s.LastPPI, s.Details)
	}
	page(t, h, loc, "Accept, then describe 2 photos ›", "Rescan the backs", loc[1:]+"/backs.png", "was the print turned over in place?")
	if w := get(t, h, loc+"/backs.png"); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("backs.png: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	page(t, h, "/photos/1", "Back", "photos/1/renditions/2/back/display.jpg", "was the print turned over in place?", "300 ppi")
	if w := get(t, h, "/photos/1/renditions/2/back/display.jpg"); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("back rendition: %d %s", w.Code, w.Header().Get("Content-Type"))
	}

	act(t, a, h, loc+"/rescan-backs", nil) // previews again, the prints still turned
	page(t, h, loc, "Check that each back lies where its front did", "Scan the backs")
	act(t, a, h, loc+"/confirm-backs", nil)
	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "Done")
	page(t, h, "/photos/1", "Back", "photos/1/renditions/2/back/display.jpg")
	if s.Details != 3 {
		t.Errorf("%d passes, want 3", s.Details)
	}
}

// Prints with blank backs: the operator skips the backs and the scan goes
// on to the quality check without a back.
func TestSkippingTheBacks(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"blank backs"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	page(t, h, loc, "Preview the backs", "No backs")
	act(t, a, h, loc+"/no-backs", nil)
	page(t, h, loc, "Accept, then describe 1 photo ›", "Rescan the backs", "<td>none</td>")
	if body := get(t, h, "/photos/0").Body.String(); strings.Contains(body, "renditions/2/back/") {
		t.Error("a back shown for a photo without one")
	}
	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "Done")
	if s.Details != 1 || s.Previews != 1 {
		t.Errorf("%d passes, %d previews; want 1 and 1", s.Details, s.Previews)
	}
}

// Story 040: the photo page takes the photo's description while the scan
// is being checked: the date taken, the photographer and what the back
// says. A photo not described waits after accept, and the scan page says
// so; once archived the description is shown and fixed.
func TestDescribingAPhoto(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"Rothesay"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	page(t, h, "/photos/0", `name="date"`, `name="approximate"`, `name="photographer"`, `name="back"`, `name="source"`, "Nothing known")
	if w := post(t, h, "/photos/0/describe", url.Values{"date": {"July 1952"}}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "1952-07") {
		t.Errorf("a date that is not an ISO 8601 prefix: %d %s", w.Code, w.Body)
	}
	// #87: Save goes on to the scan's next photo still to be described
	if loc := act(t, a, h, "/photos/0/describe", url.Values{"date": {"1952-07"}, "approximate": {"on"}, "photographer": {"Jerome Ltd"}, "back": {"Mum &amp; Dad"}, "source": {"back"}}); loc != "/photos/1" {
		t.Errorf("describe went to %s, want the next photo", loc)
	}
	page(t, h, "/photos/0", `value="1952-07"`, "checked", `value="Jerome Ltd"`, "Mum &amp;amp; Dad")

	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "1 photo waiting to be described", `href="photos/1"`)
	page(t, h, "/photos/0", "1952-07", "Jerome Ltd", "approximate")
	if body := get(t, h, "/photos/0").Body.String(); strings.Contains(body, `name="date"`) {
		t.Error("an archived photo's description is still a form")
	}
	// #87: the last photo described, Save goes back to the batch
	if to := act(t, a, h, "/photos/1/describe", url.Values{"skip": {"1"}}); to != strings.TrimSuffix(loc, "/scans/1") {
		t.Errorf("describing the last photo went to %s, want the batch", to)
	}
	page(t, h, loc, "Done")
	page(t, h, "/photos/1", "Nothing known")
}

// Story 041: once accepted and described, each photo is bagged, deposited
// with the archive service and receipted; the photo page shows the
// archive's answer, and a duplicate the archive refused is shown as such.
func TestPhotosAreDepositedWithTheArchive(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}}
	a := live(t, s)
	fake := a.Archive.(*archivetest.Fake)
	fake.Next = 40
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"deposited"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	one, two := pid.ID(40), pid.ID(41)
	page(t, h, loc, `href="photos/`+one.String()+`"`, `href="photos/`+two.String()+`"`)
	page(t, h, "/photos/"+one.String(), "Not yet deposited")
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	fake.RejectAs[two] = 7
	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "Done", "1 photo held by the archive", "1 rejected as a duplicate", `href="photos/`+two.String()+`"`)
	page(t, h, "/photos/"+one.String(), "Held by the archive", "3 files")
	page(t, h, "/photos/"+two.String(), "Rejected by the archive", "duplicate of item <code>7</code>", "data/front.tif")
	if rec, _ := fake.Receipt(one); rec.Status != "held" {
		t.Errorf("the archive does not hold %s: %+v", one, rec)
	}
	page(t, h, "/about", "Archive service")
}

// Story 042: the batch page says when the batch closed and when its work
// files were purged.
func TestTheBatchPageSaysClosedAndPurged(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"closing"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	act(t, a, h, "/batches/20261020-1/finish", nil)
	page(t, h, "/batches/20261020-1", "Closed", "held by the archive", "30 days")
	p := &batch.Purger{Root: a.Staging, Now: func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }, Grace: 30 * 24 * time.Hour,
		Threshold: 1, Free: func(string) (uint64, error) { return 1 << 40, nil }}
	if purged, err := p.Sweep(); err != nil || len(purged) != 1 {
		t.Fatalf("sweep: %v %v", purged, err)
	}
	page(t, h, "/batches/20261020-1", "Purged")
	if w := get(t, h, loc); w.Code != http.StatusNotFound {
		t.Errorf("a purged scan's page: %d", w.Code)
	}
}

// Story 057: the closed notice on the batch page counts the photos: how
// many the archive holds and how many it rejected as duplicates, each of
// those linked.
func TestTheClosedNoticeCountsThePhotos(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}}
	a := live(t, s)
	fake := a.Archive.(*archivetest.Fake)
	fake.Next = 40
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"counted"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	two := pid.ID(41)
	fake.RejectAs[two] = 7
	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	act(t, a, h, "/batches/20261020-1/finish", nil)
	page(t, h, "/batches/20261020-1", "Closed on", "1 photo held by the archive", "1 rejected as a duplicate", `href="photos/`+two.String()+`"`, "30 days")
}

// #67: a fresh batch form starts at 2400 ppi, the least noise, averaged
// 4 × 4 down to the 600 ppi master; what the operator picked stays
// picked when the form comes back with an error.
func TestNewBatchStartsAt2400PPI(t *testing.T) {
	h := live(t, &batchtest.Scanner{}).Handler()
	page(t, h, "/batches/new", `<option value="2400" selected>`)
	w := post(t, h, "/batches", url.Values{"title": {""}, "person": {"a"}, "ppi": {"1200"}, "credit": {"© Example"}})
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `<option value="1200" selected>`) {
		t.Errorf("form returned with an error: %d, 1200 ppi not kept:\n%s", w.Code, w.Body)
	}
}

// #66: the describe form takes a caption first, what the picture shows;
// the photo page shows it under the picture as its title, the scan's
// description beneath. Blank means no caption and the page is as before.
func TestACaptionTitlesThePhoto(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"War photos"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	body := get(t, h, "/photos/0").Body.String()
	if c, d := strings.Index(body, `name="caption"`), strings.Index(body, `name="date"`); c < 0 || c > d {
		t.Errorf("the caption field is not first on the form: caption at %d, date at %d", c, d)
	}
	if strings.Contains(body, "photo-caption") {
		t.Error("a photo without a caption shows one")
	}
	act(t, a, h, "/photos/0/describe", url.Values{"caption": {"Air Raid Wardens"}, "skip": {"1"}})
	body = get(t, h, "/photos/0").Body.String()
	if c := strings.Index(body, `class="photo-caption`); c < 0 || strings.Index(body[c:], "Air Raid Wardens") < 0 || strings.Index(body[c:], "Air Raid Wardens") > strings.Index(body[c:], "War photos") {
		t.Errorf("the caption is not shown as the title with the description beneath:\n%s", body)
	}
	page(t, h, "/photos/0", `value="Air Raid Wardens"`)
	act(t, a, h, "/photos/0/describe", url.Values{"caption": {"  "}, "date": {"1952"}})
	if body := get(t, h, "/photos/0").Body.String(); strings.Contains(body, "photo-caption") {
		t.Error("a caption cleared to blank is still shown")
	}
}

// #76: each wait's buttons say which way they go. The one that moves the
// scan on ends in › and names what comes next; a step back starts with ‹;
// a rescan in place has neither. At check quality the forward step is the
// describing still to do, and only once every photo is described is it
// the archive.
func TestScanButtonsSayWhichWayTheyGo(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"which way"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	page(t, h, loc, ">Confirm and scan ›<", ">Rescan the preview<")
	act(t, a, h, loc+"/confirm", nil)
	page(t, h, loc, ">Preview the backs ›<", ">No backs ›<", ">‹ Back to the prints<")
	act(t, a, h, loc+"/backs", nil)
	page(t, h, loc, ">Scan the backs ›<", ">No backs ›<", ">Preview the backs again<", ">‹ Back to the prints<")
	act(t, a, h, loc+"/confirm-backs", nil)
	page(t, h, loc, ">Accept, then describe 2 photos ›<", ">Rescan the backs<", ">‹ Back to the prints<", "2 photos still to be described")
	act(t, a, h, "/photos/0/describe", url.Values{"skip": {"1"}})
	page(t, h, loc, ">Accept, then describe 1 photo ›<", "1 photo still to be described")
	act(t, a, h, "/photos/1/describe", url.Values{"skip": {"1"}})
	page(t, h, loc, ">Accept and archive ›<")
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "still to be described") {
		t.Error("every photo described, yet the notice says some are still to be")
	}
}

// #73: once a scan is accepted with photos still to describe, the notice
// carries a primary button to the first of them, in scan order, rather
// than leaving the operator to spot that the ids are links.
func TestTheAcceptedNoticeHasADescribeButton(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"describe next"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "2 photos waiting to be described", `href="photos/0"`, `href="photos/1"`, `class="button is-primary" href="photos/0">Describe photo 0 ›<`)
	act(t, a, h, "/photos/0/describe", url.Values{"skip": {"1"}})
	page(t, h, loc, "1 photo waiting to be described", `href="photos/1">Describe photo 1 ›<`)
	act(t, a, h, "/photos/1/describe", url.Values{"skip": {"1"}})
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "Describe photo") {
		t.Error("every photo described, yet a Describe button remains")
	}
}

// #74: describing a photo needs no scanner. Once the photo's run has
// reached the accepted wait, or waits to be described, Save records the
// description whatever the scanner is doing, and the photo's run is
// ticked when the step running ends.
func TestDescribingAPhotoWhileTheScannerIsBusy(t *testing.T) {
	a := live(t, &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}})
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"first"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	act(t, a, h, "/photos/0/describe", url.Values{"skip": {"1"}})
	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "1 photo waiting to be described")

	sc := hold(&batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}})
	a.Scanner = sc
	if w := post(t, h, "/batches", url.Values{"title": {"second"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("second batch: %d %s", w.Code, w.Body)
	}
	<-sc.started
	if w := post(t, h, "/photos/1/describe", url.Values{"caption": {"the crew"}, "skip": {"1"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("describe while the scanner is busy with another scan: %d %s", w.Code, w.Body)
	}
	if p, _, err := batch.FindPhoto(a.Staging, 1); err != nil || p.Described == nil || p.Caption != "the crew" {
		t.Errorf("description not recorded: %+v, %v", p, err)
	}
	close(sc.release)
	a.wait()
	page(t, h, loc, "Done")
}

// holdBacks scans the fronts freely and holds the backs preview.
type holdBacks struct {
	*batchtest.Scanner
	previews         int
	started, release chan struct{}
}

func (s *holdBacks) Preview(ctx context.Context) (image.Image, error) {
	s.previews++
	if s.previews == 2 {
		close(s.started)
		<-s.release
	}
	return s.Scanner.Preview(ctx)
}

func (s *holdBacks) Detail(ctx context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	return s.Scanner.Detail(ctx, area, ppi)
}

// #74: a photo its scan is still scanning (its run short of the accepted
// wait while a step runs) cannot be described until the step ends, since
// the step writes the photo's record too.
func TestDescribingAPhotoStillBeingScannedWaits(t *testing.T) {
	sc := &holdBacks{Scanner: &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}, started: make(chan struct{}), release: make(chan struct{})}
	a := live(t, sc.Scanner)
	a.Scanner = sc
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"held backs"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	if w := post(t, h, loc+"/backs", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("backs: %d %s", w.Code, w.Body)
	}
	<-sc.started
	if w := post(t, h, "/photos/0/describe", url.Values{"skip": {"1"}}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "wait for scan") {
		t.Errorf("describe during the backs pass: %d %s", w.Code, w.Body)
	}
	close(sc.release)
	a.wait()
	act(t, a, h, "/photos/0/describe", url.Values{"skip": {"1"}})
}

// #86: seeing the backs on their preview is often what shows they are
// blank, so No backs is offered at the backs wait as well as before the
// preview, and takes the scan on to the quality check without a back.
func TestSkippingTheBacksAfterTheirPreview(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"blank after all"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	page(t, h, loc, ">Scan the backs ›<", ">No backs ›<", ">Preview the backs again<")
	act(t, a, h, loc+"/no-backs", nil)
	page(t, h, loc, "Accept, then describe 1 photo ›", "<td>none</td>")
	if body := get(t, h, "/photos/0").Body.String(); strings.Contains(body, "renditions/2/back/") {
		t.Error("a back shown for a photo without one")
	}
	if s.Details != 1 || s.Previews != 2 {
		t.Errorf("%d passes, %d previews; want 1 and 2", s.Details, s.Previews)
	}
}

// #85: adjusting the crop ends on the last print's box, so that box
// offers Confirm and scan beside Remove this region; the earlier boxes
// do not, and the state bar's button stays.
func TestLastPrintOffersConfirm(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}, {X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 30000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"confirm below"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	body := get(t, h, loc).Body.String()
	first, second := strings.Index(body, `id="print-1"`), strings.Index(body, `id="print-2"`)
	if first < 0 || second < first {
		t.Fatalf("no region editors in order:\n%s", body)
	}
	const confirm = ">Confirm and scan ›<"
	if strings.Count(body, confirm) != 2 {
		t.Errorf("want Confirm and scan at the top and on the last print, got %d:\n%s", strings.Count(body, confirm), body)
	}
	if strings.Contains(body[first:second], confirm) {
		t.Error("the first print's box offers Confirm and scan")
	}
	last := body[second:]
	c, remove := strings.Index(last, confirm), strings.LastIndex(last, ">Remove this region<") // the Remove below the close-up
	if c < 0 || remove < 0 || c > remove {
		t.Fatalf("the last print's box does not offer Confirm and scan before Remove this region: confirm at %d, remove at %d", c, remove)
	}
	if !strings.Contains(last[:c], `action="`+strings.TrimPrefix(loc, "/")+`/confirm"`) {
		t.Error("the last print's Confirm and scan does not post to confirm")
	}
}

// holdDetail scans the preview freely and holds a detail pass until its
// context ends, as scanimage does when it is killed.
type holdDetail struct {
	*batchtest.Scanner
	started chan struct{}
	once    sync.Once
}

func (s *holdDetail) Detail(ctx context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

// A step that is running can be stopped, as when the prints were
// confirmed before their regions were sized: the scanner's pass ends, the
// run says it was stopped, and the scan goes back to the prints or
// carries on. Stopping an idle scan is refused.
func TestStopARunningStepAndGoBackToThePrints(t *testing.T) {
	sc := &holdDetail{Scanner: &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}, started: make(chan struct{})}
	a := live(t, sc.Scanner)
	a.Scanner = sc
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"first"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	if w := post(t, h, loc+"/stop", nil); w.Code != http.StatusConflict {
		t.Errorf("stop with nothing running: %d, want 409", w.Code)
	}
	if w := post(t, h, loc+"/confirm", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("confirm: %d %s", w.Code, w.Body)
	}
	<-sc.started
	page(t, h, loc, "Working", "Fronts", loc[1:]+"/stop", "Stop")
	if w := post(t, h, loc+"/stop", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("stop: %d %s", w.Code, w.Body)
	}
	a.wait()
	page(t, h, loc, "Stopped", "Back to the prints", "Carry on")
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "Failed") || strings.Contains(body, "context canceled") {
		t.Errorf("a stop reads as a failure:\n%s", body)
	}
	act(t, a, h, loc+"/prints", nil)
	page(t, h, loc, "Confirm and scan")
}

// #105: a print can be rejected from the top of its box as well as the
// bottom, without scrolling past its close-up.
func TestRemoveIsOfferedAboveAndBelowTheCloseUp(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"reject"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	body := get(t, h, loc).Body.String()
	box := body[strings.Index(body, `id="print-1"`):]
	img := strings.Index(box, "regions/1.png")
	const remove = ">Remove this region<"
	if img < 0 || !strings.Contains(box[:img], remove) || !strings.Contains(box[img:], remove) {
		t.Errorf("want Remove this region above and below the close-up:\n%s", box)
	}
}

// #105: a close-up reduced to fit the screen spaces its ruler ticks to
// stay apart, so they do not pile into one another; the help says the
// spacing the rulers use.
func TestARulerOnAReducedCloseUpKeepsItsTicksApart(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 200, Y1: 280, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"large"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	body := get(t, h, loc).Body.String()
	box := body[strings.Index(body, `id="print-1"`):]
	m := regexp.MustCompile(`grid-template-columns:\d+px (\d+)px`).FindStringSubmatch(box)
	if m == nil {
		t.Fatalf("no ruled close-up:\n%s", box)
	}
	width, _ := strconv.Atoi(m[1])
	top := box[strings.Index(box, "grid-template-columns"):strings.Index(box, "regions/1.png")]
	left := strings.Index(top, `<div style="position:relative">`)
	top = top[left : strings.Index(top[left:], "</div>")+left]
	ticks := strings.Count(top, "|<br>")
	if most := width/minTickPx + 1; ticks > most {
		t.Errorf("%d ticks across %d px, want at most %d so they stay %d px apart", ticks, width, most, minTickPx)
	}
	if every := regexp.MustCompile(`every (\d+) mm`).FindStringSubmatch(box); every == nil || every[1] == "5" {
		t.Errorf("the help should say the wider tick spacing the rulers use, got %q", every)
	}
}
