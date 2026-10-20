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
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
)

func live(t *testing.T, s *batchtest.Scanner) *App {
	t.Helper()
	return &App{
		Staging: t.TempDir(),
		Store:   wf.NewMemStore(),
		Scanner: s,
		People:  register.Register{Dir: "testdata/people"},
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

func TestABatchFromStartToBags(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}
	a := live(t, s)
	h := a.Handler()
	page(t, h, "/batches/new", `name="description"`, `name="person"`, `name="ppi"`)

	loc := act(t, a, h, "/batches", url.Values{"description": {"SS Bonita"}, "person": {"a"}, "ppi": {"600"}})
	if loc != "/batches/20261020-1" {
		t.Fatalf("new batch went to %q", loc)
	}
	page(t, h, loc, "Confirm", "Rescan the preview", "Preview, 75 ppi")

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
	if !bag.Complete(filepath.Join(a.Staging, "bags"), 0) || !bag.Complete(filepath.Join(a.Staging, "bags"), 1) {
		t.Error("bags not complete after accept")
	}
	page(t, h, loc, "Done")
}

func TestRetryAfterAFailure(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}, FailDetails: 1}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"description": {"SS Bonita"}, "person": {"a"}, "ppi": {"600"}})
	act(t, a, h, loc+"/confirm", nil)
	page(t, h, loc, "USB disconnected", "Retry")
	act(t, a, h, loc+"/retry", nil)
	page(t, h, loc, "Accept")
}

func TestNewBatchRefusals(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	for _, f := range []url.Values{
		{"description": {""}, "person": {"a"}, "ppi": {"600"}},
		{"description": {"x"}, "person": {"c"}, "ppi": {"600"}},
		{"description": {"x"}, "person": {"a"}, "ppi": {"300"}},
	} {
		if w := post(t, h, "/batches", f); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("new batch %v: %d, want 422", f, w.Code)
		}
	}
}

// slow holds the preview until released, so a step can be caught running.
type slow struct {
	*batchtest.Scanner
	release chan struct{}
}

func (s slow) Preview(ctx context.Context) (image.Image, error) {
	<-s.release
	return s.Scanner.Preview(ctx)
}

func (s slow) Detail(ctx context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	return s.Scanner.Detail(ctx, area, ppi)
}

func TestOneStepAtATime(t *testing.T) {
	sc := slow{&batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}, make(chan struct{})}
	a := live(t, sc.Scanner)
	a.Scanner = sc
	h := a.Handler()
	w := post(t, h, "/batches", url.Values{"description": {"first"}, "person": {"a"}, "ppi": {"600"}})
	loc := w.Header().Get("Location")
	page(t, h, loc, `http-equiv="refresh"`, "Working")

	if w := post(t, h, "/batches", url.Values{"description": {"second"}, "person": {"a"}, "ppi": {"600"}}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "busy") {
		t.Errorf("second batch while scanning: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(a.Staging, "batches", "20261020-2")); !os.IsNotExist(err) {
		t.Error("a refused batch was still recorded")
	}
	close(sc.release)
	a.wait()
	page(t, h, loc, "Confirm")
}
