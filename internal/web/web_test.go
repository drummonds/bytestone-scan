package web

import (
	"context"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
)

// scanned is a staging root holding one finished batch of two prints.
func scanned(t *testing.T) (*App, *batch.Batch) {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 4, 5, 0, time.UTC)
	b, err := batch.Describe(root, register.Register{Dir: "testdata/people"},
		batch.Description{Text: "SS Bonita & crew", Person: 10, ScanPPI: 600}, now)
	if err != nil {
		t.Fatal(err)
	}
	store := wf.NewMemStore()
	runner := wf.NewRunner(store, nil)
	s := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000},
		{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 60000},
	}}
	clock := func() time.Time { return now }
	if _, err := b.Run(context.Background(), runner, s, clock); err != nil {
		t.Fatal(err)
	}
	b.Confirm()
	b.Run(context.Background(), runner, s, clock)
	b.Accept()
	if rec, err := b.Run(context.Background(), runner, s, clock); err != nil || rec.Status != wf.StatusCompleted {
		t.Fatalf("batch: %+v, %v", rec, err)
	}
	return &App{Staging: root, Store: store}, b
}

func get(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	return w
}

func page(t *testing.T, h http.Handler, url string, want ...string) {
	t.Helper()
	w := get(t, h, url)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", url, w.Code, w.Body)
	}
	for _, s := range want {
		if !strings.Contains(w.Body.String(), s) {
			t.Errorf("GET %s lacks %q", url, s)
		}
	}
}

func TestBatchesAreTheWorkQueue(t *testing.T) {
	app, b := scanned(t)
	h := app.Handler()
	if w := get(t, h, "/"); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/workflows/" {
		t.Errorf("GET / = %d to %q, want a redirect to the work queue", w.Code, w.Header().Get("Location"))
	}
	page(t, h, "/workflows/", b.ID)
}

func TestBatchPage(t *testing.T) {
	app, b := scanned(t)
	page(t, app.Handler(), "/batches/"+b.ID,
		"SS Bonita &amp; crew", "Ada Example", "600 ppi",
		"batches/"+b.ID+"/prints.png", `href="photos/0"`, `href="photos/1"`,
		"Preview, 75 ppi")
	w := get(t, app.Handler(), "/batches/"+b.ID+"/prints.png")
	if _, err := png.Decode(w.Body); err != nil {
		t.Errorf("prints.png: %v", err)
	}
	if w := get(t, app.Handler(), "/batches/20990101-1"); w.Code != http.StatusNotFound {
		t.Errorf("unknown batch: %d", w.Code)
	}
}

func TestQualityPage(t *testing.T) {
	app, b := scanned(t)
	h := app.Handler()
	page(t, h, "/photos/1",
		`src="photos/1/fit.jpg"`, `<input type="image" name="at"`, "<svg", // the master, clickable, and its histogram
		"600 ppi", b.ID, "highlights",
		"Master, 600 ppi,", "shown scaled to fit",
	)
	page(t, h, "/photos/1?at.x=50&at.y=40", `src="photos/1/crop.png?`, "1:1, master pixels")
	page(t, h, "/photos/1", "Corners at 1:1", `src="photos/1/crop.png?x=0&amp;y=0&amp;size=256"`)
	if n := strings.Count(get(t, h, "/photos/1").Body.String(), "&amp;size=256\""); n != 4 {
		t.Errorf("%d corner crops, want 4", n)
	}
	corner, err := png.Decode(get(t, h, "/photos/1/crop.png?x=99999&y=99999&size=256").Body)
	if err != nil || corner.Bounds().Dx() != 256 {
		t.Errorf("corner crop: %v, %v", corner.Bounds(), err)
	}

	fit, err := jpeg.Decode(get(t, h, "/photos/0/fit.jpg").Body)
	if err != nil || fit.Bounds().Dx() > FitWidth {
		t.Fatalf("fit.jpg: %v, %v", fit.Bounds(), err)
	}
	crop, err := png.Decode(get(t, h, "/photos/0/crop.png?x=100&y=100").Body)
	if err != nil || crop.Bounds().Dx() != CropSize {
		t.Errorf("crop.png: %v, %v", crop.Bounds(), err)
	}
	if w := get(t, h, "/photos/zz"); w.Code != http.StatusNotFound {
		t.Errorf("unknown photo: %d", w.Code)
	}
}
