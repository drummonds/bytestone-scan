package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// A scanner that is looked for, as the 4990 and the FF-680W are: err is
// what the look finds wrong, nil when it is connected.
type lookedBed struct {
	*batchtest.Scanner
	err error
}

func (s *lookedBed) Find(context.Context) (string, error) { return "epson2:libusb:001:042", s.err }

type lookedFeeder struct {
	*batchtest.Feeder
	err error
}

func (f *lookedFeeder) Find(context.Context) (string, error) { return "epsonds:libusb:001:012", f.err }

var (
	no4990   = errors.New("the Epson 4990 is not connected: no epson2 USB scanner in scanimage -L")
	noFF680W = errors.New("the Epson FF-680W is not connected: no epsonds scanner in scanimage -L")
	noList   = errors.New("the scanners could not be listed: exec: \"scanimage\": executable file not found in $PATH")
)

// looked is an app whose scanners are both looked for.
func looked(t *testing.T) (*App, *lookedBed, *lookedFeeder) {
	t.Helper()
	bed := &lookedBed{Scanner: &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}}}
	feeder := &lookedFeeder{Feeder: &batchtest.Feeder{Sides: []uint8{40, 255}}}
	a := live(t, bed.Scanner)
	a.Scanner, a.Feeder = bed, feeder
	return a, bed, feeder
}

func batchesRecorded(t *testing.T, a *App) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(a.Staging, "batches"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return len(entries)
}

func newBatch(then string) url.Values {
	f := url.Values{"title": {"SS Bonita"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "provenance": {"Aunt Jean"}}
	if then != "" {
		f.Set("then", then)
	}
	return f
}

// #185: no batch is recorded for a scanner that is not connected; the
// form comes back with what was typed and the reason.
func TestNewBatchIsNotRecordedWithoutItsScanner(t *testing.T) {
	for _, c := range []struct {
		name, then  string
		bed, feeder error
		says        string
	}{
		{"scan the preview, no 4990", "", no4990, nil, "the Epson 4990 is not connected"},
		{"scan letters, no FF-680W", "feed", nil, noFF680W, "the Epson FF-680W is not connected"},
		{"scan the preview, the scanners not listed", "", noList, noList, "the scanners could not be listed"},
		{"scan letters, the scanners not listed", "feed", noList, noList, "the scanners could not be listed"},
	} {
		a, bed, feeder := looked(t)
		bed.err, feeder.err = c.bed, c.feeder
		h := a.Handler()
		w := post(t, h, "/batches", newBatch(c.then))
		body := w.Body.String()
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d, want 422", c.name, w.Code)
		}
		for _, want := range []string{c.says, `name="title" value="SS Bonita"`, `value="Aunt Jean"`, "New batch"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the form back lacks %q", c.name, want)
			}
		}
		if n := batchesRecorded(t, a); n != 0 {
			t.Errorf("%s: %d batches recorded, want none", c.name, n)
		}
		if strings.Contains(get(t, h, "/workflows/").Body.String(), "20261020-1") {
			t.Errorf("%s: the Queue has a batch", c.name)
		}
		// plugged in, and asked again
		bed.err, feeder.err = nil, nil
		if w := post(t, h, "/batches", newBatch(c.then)); w.Code != http.StatusSeeOther {
			t.Errorf("%s: once connected %d %s, want it begun", c.name, w.Code, w.Body)
		}
		a.wait()
	}
}

// A scanner is looked for only by what needs it.
func TestNewBatchNeedsOnlyTheScannerItUses(t *testing.T) {
	for _, c := range []struct {
		name, then  string
		bed, feeder error
		to          string
	}{
		{"scan the preview, no FF-680W", "", nil, noFF680W, "/batches/20261020-1/scans/1"},
		{"scan letters, no 4990", "feed", no4990, nil, "/batches/20261020-1/feeds/new"},
		{"import pages, neither", "import", no4990, noFF680W, "/batches/20261020-1/imports/new"},
		{"import a film, neither", "film", no4990, noFF680W, "/batches/20261020-1/films/new"},
	} {
		a, bed, feeder := looked(t)
		bed.err, feeder.err = c.bed, c.feeder
		if loc := act(t, a, a.Handler(), "/batches", newBatch(c.then)); loc != c.to {
			t.Errorf("%s: went to %s, want %s", c.name, loc, c.to)
		}
	}
}

// #185: nor is a scan or a feed recorded for a batch.
func TestAScanOrAFeedIsNotRecordedWithoutItsScanner(t *testing.T) {
	a, bed, feeder := looked(t)
	h := a.Handler()
	act(t, a, h, "/batches", newBatch("import")) // a batch with no scan yet
	bed.err, feeder.err = no4990, noFF680W

	w := post(t, h, "/batches/20261020-1/scans", url.Values{"description": {"The second shoebox"}})
	if body := w.Body.String(); w.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "the Epson 4990 is not connected") || !strings.Contains(body, `value="The second shoebox"`) {
		t.Errorf("one more scan with no 4990: %d %s", w.Code, body)
	}
	w = post(t, h, "/batches/20261020-1/feeds", url.Values{"description": {"From Ada, 1951"}})
	if body := w.Body.String(); w.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "the Epson FF-680W is not connected") || !strings.Contains(body, `value="From Ada, 1951"`) {
		t.Errorf("scan letters with no FF-680W: %d %s", w.Code, body)
	}
	body := get(t, h, "/batches/20261020-1").Body.String()
	for _, s := range []string{"Scan 1", "Feed 1", "failed"} {
		if strings.Contains(body, s) {
			t.Errorf("the batch's page has %q: something was recorded", s)
		}
	}
	// the batch is as it would be with nothing asked: nothing scanned,
	// and no failed scan for Finish scanning to wait on
	if w := post(t, h, "/batches/20261020-1/finish", nil); !strings.Contains(w.Body.String(), "nothing has been scanned") {
		t.Errorf("finish: %d %s", w.Code, w.Body)
	}
}

func TestAScanAndAFeedAreBegunOnceTheirScannersAreConnected(t *testing.T) {
	a, bed, feeder := looked(t)
	h := a.Handler()
	act(t, a, h, "/batches", newBatch("import"))
	bed.err, feeder.err = no4990, noFF680W
	post(t, h, "/batches/20261020-1/scans", url.Values{"description": {"x"}})
	post(t, h, "/batches/20261020-1/feeds", url.Values{"description": {"x"}})
	feeder.err = nil
	if loc := act(t, a, h, "/batches/20261020-1/feeds", url.Values{"description": {"From Ada, 1951"}}); loc != "/batches/20261020-1/scans/1" {
		t.Errorf("the feed went to %s", loc)
	}
	page(t, h, "/batches/20261020-1", "Feed 1")
}

// #185: a batch's page says what each kind is captured at, and its
// workflow is not named for photographs alone.
func TestTheBatchPageSaysWhatPrintsAndLettersAreCapturedAt(t *testing.T) {
	a, _, _ := looked(t)
	h := a.Handler()
	act(t, a, h, "/batches", newBatch("feed"))
	page(t, h, "/batches/20261020-1",
		"<th>Prints scanned at</th><td>600 ppi</td>",
		"<th>Letters fed at</th><td>600 ppi, 8 bits</td>",
		">Archive a batch</a>")
	if body := get(t, h, "/batches/20261020-1").Body.String(); strings.Contains(body, "<th>Scanned at</th>") || strings.Contains(body, "Archive photographs") {
		t.Error("the batch's page still says Scanned at, or Archive photographs")
	}
	page(t, h, "/workflows/", "Archive a batch")
}
