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
	"strings"
	"sync"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register/registertest"
)

func live(t *testing.T, s *batchtest.Scanner) *App {
	t.Helper()
	return &App{
		Staging: t.TempDir(),
		Store:   wf.NewMemStore(),
		Scanner: s,
		People:  registertest.Example,
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
	page(t, h, "/batches/new", `name="description"`, `name="person"`, `name="ppi"`, `name="provenance"`, `name="credit"`)

	loc := act(t, a, h, "/batches", url.Values{"description": {"SS Bonita"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	if loc != "/batches/20261020-1" {
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
	page(t, h, loc, "Accept", "Rescan at", `href="photos/0"`, `href="photos/1"`)
	page(t, h, "/photos/1", "Master, 600 ppi,") // the quality page works before packaging

	act(t, a, h, loc+"/rescan", url.Values{"ppi": {"1200"}})
	if s.LastPPI != 1200 || s.Details != 2 {
		t.Errorf("rescan: last ppi %d after %d passes, want 1200 after 2", s.LastPPI, s.Details)
	}

	act(t, a, h, loc+"/accept", nil)
	if !archive.Complete(filepath.Join(a.Staging, "archive"), 0) || !archive.Complete(filepath.Join(a.Staging, "archive"), 1) {
		t.Error("photos not archived after accept")
	}
	page(t, h, loc, "Done")
}

func TestRetryAfterAFailure(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}, FailDetails: 1}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"description": {"SS Bonita"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	page(t, h, loc, "USB disconnected", "Retry")
	act(t, a, h, loc+"/retry", nil)
	page(t, h, loc, "Accept")
}

func TestNewBatchRefusals(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	for _, f := range []url.Values{
		{"description": {""}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}},
		{"description": {"x"}, "person": {"c"}, "ppi": {"600"}, "credit": {"© Example"}},
		{"description": {"x"}, "person": {""}, "ppi": {"600"}, "credit": {"© Example"}},
		{"description": {"x"}, "person": {"a"}, "ppi": {"300"}, "credit": {"© Example"}},
		{"description": {"x"}, "person": {"a"}, "ppi": {"600"}, "credit": {" "}},
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
	w := post(t, h, "/batches", url.Values{"description": {"first"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	loc := w.Header().Get("Location")
	page(t, h, loc, "Working")

	if w := post(t, h, "/batches", url.Values{"description": {"second"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "busy") {
		t.Errorf("second batch while scanning: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(a.Staging, "batches", "20261020-2")); !os.IsNotExist(err) {
		t.Error("a refused batch was still recorded")
	}
	close(sc.release)
	a.wait()
	page(t, h, loc, "Confirm")
}

// A batch left waiting can be cancelled from the queue, one at a time.
func TestCancelABatchFromTheQueue(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"description": {"abandoned"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	runs, _ := a.Store.ListRuns(context.Background(), wf.RunFilter{})
	if len(runs) != 1 || runs[0].Status != wf.StatusWaiting {
		t.Fatalf("runs %+v, want one batch waiting at confirm", runs)
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
		t.Error("a cancelled batch is still offered to cancel")
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
	a.People = registertest.List{{ID: 11, Name: "Zak Example"}, {ID: 10, Name: "Ada Example"}}
	h := a.Handler()
	body := get(t, h, "/batches/new").Body.String()
	ada := strings.Index(body, `<option value="a">Ada Example (a)</option>`)
	zak := strings.Index(body, `<option value="b">Zak Example (b)</option>`)
	if ada < 0 || zak < ada {
		t.Errorf("want Ada (a) then Zak (b) offered as home person:\n%s", body)
	}
	if w := post(t, h, "/batches", url.Values{"description": {"x"}, "person": {""}, "ppi": {"600"}, "credit": {"© Example"}}); !strings.Contains(w.Body.String(), "choose the home person") {
		t.Errorf("no person chosen: want it asked for:\n%s", w.Body)
	}
	// a refused batch offers the people again, with its choice kept
	w := post(t, h, "/batches", url.Values{"description": {""}, "person": {"b"}, "ppi": {"600"}, "credit": {"© Example"}})
	if !strings.Contains(w.Body.String(), `<option value="b" selected>Zak Example (b)</option>`) {
		t.Errorf("refused form does not keep the person chosen:\n%s", w.Body)
	}
}

// Story 014: provenance and credit are typed once per shoebox, not per
// load of the bed.
func TestNewBatchIsPrefilledFromTheLatestBatch(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	body := get(t, h, "/batches/new").Body.String()
	if !strings.Contains(body, `name="provenance" value=""`) || !strings.Contains(body, `name="credit" value=""`) {
		t.Errorf("first batch: want blank provenance and credit:\n%s", body)
	}
	act(t, a, h, "/batches", url.Values{"description": {"one"}, "person": {"a"}, "ppi": {"600"},
		"provenance": {"Aunt Jean's shoebox"}, "credit": {"Drummond family collection, CC BY 4.0"}})
	page(t, h, "/batches/new", `name="provenance" value="Aunt Jean&#39;s shoebox"`, `name="credit" value="Drummond family collection, CC BY 4.0"`)
	// a refused form keeps what was typed, not the latest batch's
	w := post(t, h, "/batches", url.Values{"description": {""}, "person": {"a"}, "ppi": {"600"}, "provenance": {"box two"}, "credit": {"© Other"}})
	if !strings.Contains(w.Body.String(), `name="provenance" value="box two"`) || !strings.Contains(w.Body.String(), `name="credit" value="© Other"`) {
		t.Errorf("refused form does not keep what was typed:\n%s", w.Body)
	}
	page(t, h, "/batches/20261020-1", "Provenance", "Aunt Jean&#39;s shoebox", "Credit", "Drummond family collection, CC BY 4.0")
}

func TestNewBatchWithNoPeopleIsRefused(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	a.People = registertest.List{}
	w := get(t, a.Handler(), "/batches/new")
	if body := w.Body.String(); !strings.Contains(body, a.People.String()) || strings.Contains(body, "<form") {
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
	loc := post(t, h, "/batches", url.Values{"description": {"first"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}}).Header().Get("Location")
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
	loc2 := post(t, h, "/batches", url.Values{"description": {"second"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}}).Header().Get("Location")
	<-sc2.started
	page(t, h, loc2+"/progress", "Preview", "of about")
	close(sc2.release)
	a.wait()
}
