package web

import (
	"image/jpeg"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// Story 117: a batch feeds the sheets of letters through the feeder, the
// operator arranges them into letters, and each letter is archived.
func TestFeedingLetters(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	a.Feeder = &batchtest.Feeder{Sides: []uint8{40, 255, 90, 120}}
	h := a.Handler()
	// a new batch can start with a feed instead of a scan
	form := act(t, a, h, "/batches", url.Values{"title": {"Letters home"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"feed"}})
	if form != "/batches/20261020-1/feeds/new" {
		t.Fatalf("new batch for feeding went to %s", form)
	}
	page(t, h, form, `name="description" value="Letters home"`, "face down", "Feed the sheets")
	page(t, h, "/batches/20261020-1", "Scan letters")

	loc := act(t, a, h, "/batches/20261020-1/feeds", url.Values{"description": {"From Ada, 1951"}})
	if loc != "/batches/20261020-1/scans/1" {
		t.Fatalf("feed went to %s", loc)
	}
	page(t, h, "/batches/20261020-1", "Feed 1", `href="batches/20261020-1/scans/1"`)
	page(t, h, loc, "Feed 1", "From Ada, 1951", "600 ppi, 8 bits", "Fake Feeder", "Arrange into letters",
		`name="letter-1" value="1"`, `name="letter-2" value="1"`, `name="role-2"`, `sides/1"`, `sides/4"`, "Sheet 1", "Sheet 2")
	body := get(t, h, loc).Body.String()
	for _, s := range []string{"Confirm and scan", "Preview, 150 ppi", "Rescan", "Scanned at"} {
		if strings.Contains(body, s) {
			t.Errorf("a feed's page offers %q", s)
		}
	}
	if w := get(t, h, loc+"/sides/1"); w.Code != http.StatusOK {
		t.Errorf("side 1: %d", w.Code)
	} else if _, err := jpeg.Decode(w.Body); err != nil {
		t.Errorf("side 1: %v", err)
	}

	if loc2 := act(t, a, h, loc+"/arrange", url.Values{"letter-1": {"1"}, "role-1": {"sheet"}, "letter-2": {"1"}, "role-2": {"envelope"}}); loc2 != loc {
		t.Fatalf("arrange went to %s", loc2)
	}
	page(t, h, loc, `href="letters/0"`, "envelope", "Accept and archive")
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "Arrange into letters") {
		t.Error("the feed still offers arranging once the letters have their ids")
	}
	v := strconv.Itoa(rendition.Version)
	page(t, h, "/letters/0", "Letter 0", "From Ada, 1951", "Sheet 1, front", "Sheet 1, back", "Envelope, front", "Envelope, back",
		`letters/0/renditions/`+v+`/sheet-01-front/display.jpg`, `letters/0/renditions/`+v+`/sheet-02-back/display.jpg`,
		"Fake Feeder", "600 ppi", "Not yet deposited", `name="side" value="sheet-01-back"`, "Turn")
	if w := get(t, h, "/letters/0/renditions/"+v+"/sheet-02-back/display.jpg"); w.Code != http.StatusOK {
		t.Errorf("rendition: %d", w.Code)
	} else if _, err := jpeg.Decode(w.Body); err != nil {
		t.Errorf("rendition: %v", err)
	}
	if loc2 := act(t, a, h, "/letters/0/turn", url.Values{"side": {"sheet-01-back"}, "by": {"90"}}); loc2 != "/letters/0" {
		t.Errorf("turn went to %s", loc2)
	}
	if rec, _, err := batch.FindLetter(a.Staging, 0); err != nil || rec.Sheets[0].Back.Turn != 90 {
		t.Errorf("after the turn: %+v, %v", rec.Sheets, err)
	}
	page(t, h, "/letters/0", "90°")

	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "Done", "1 letter held")
	page(t, h, "/letters/0", "Held by the archive")
	if body := get(t, h, "/letters/0").Body.String(); strings.Contains(body, `name="by"`) {
		t.Error("an accepted letter still offers turns")
	}
	if w := post(t, h, "/letters/0/turn", url.Values{"side": {"sheet-01-back"}, "by": {"90"}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("turn after accept: %d", w.Code)
	}
	if loc := act(t, a, h, "/batches/20261020-1/finish", nil); loc != "/batches/20261020-1" {
		t.Errorf("finish went to %s", loc)
	}
	page(t, h, "/batches/20261020-1", "Closed", "1 item held")
}

// Story 117: what the feed refuses comes back as an error page.
func TestFeedRefusalsOnTheWeb(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"Letters"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"feed"}})
	// no feeder: the feed step fails and says so
	loc := act(t, a, h, "/batches/20261020-1/feeds", url.Values{})
	page(t, h, loc, "Failed", "feeder", "Retry")
	// a feeder, but a bad arrangement
	a.Feeder = &batchtest.Feeder{Sides: []uint8{1, 2, 3, 4}}
	act(t, a, h, loc+"/retry", nil)
	page(t, h, loc, "Arrange into letters")
	if w := post(t, h, loc+"/arrange", url.Values{"letter-1": {"1"}, "role-1": {"postcard"}, "letter-2": {"1"}, "role-2": {"sheet"}}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "postcard") {
		t.Errorf("bad role: %d %s", w.Code, w.Body)
	}
	if w := get(t, h, "/letters/9"); w.Code != http.StatusNotFound {
		t.Errorf("unknown letter: %d", w.Code)
	}
}
