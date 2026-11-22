package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

// Story 181: each side has a blank mark at arrange, off unless the
// operator sets it; a side marked is left out of the letter, named on
// its page as blank with no picture and no turn, and counted on the
// feed's page.
func TestMarkingASideBlankAtArrange(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	a.Feeder = &batchtest.Feeder{Sides: []uint8{40, 255, 90, 120}}
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"Letters home"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"feed"}})
	loc := act(t, a, h, "/batches/20261020-1/feeds", url.Values{"description": {"From Ada, 1951"}})
	page(t, h, loc, `name="blank-1-front"`, `name="blank-1-back"`, `name="blank-2-front"`, `name="blank-2-back"`, "blank: leave out")
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "checked") {
		t.Error("a side is marked blank for the operator")
	}

	// every side of the one letter marked: refused, and still to arrange
	all := url.Values{"letter-1": {"1"}, "role-1": {"sheet"}, "letter-2": {"1"}, "role-2": {"envelope"},
		"blank-1-front": {"on"}, "blank-1-back": {"on"}, "blank-2-front": {"on"}, "blank-2-back": {"on"}}
	if w := post(t, h, loc+"/arrange", all); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "needs a side") {
		t.Errorf("every side blank: %d %s", w.Code, w.Body)
	}
	page(t, h, loc, "Arrange into letters")

	act(t, a, h, loc+"/arrange", url.Values{"letter-1": {"1"}, "role-1": {"sheet"}, "letter-2": {"1"}, "role-2": {"envelope"}, "blank-1-back": {"on"}})
	page(t, h, loc, `href="letters/0"`, "4, 2 sheets", "1 left out as blank", "Sheet 1's back", "Accept and archive")
	rec, _, err := batch.FindLetter(a.Staging, 0)
	if err != nil || !rec.Sheets[0].Back.Blank || rec.Sheets[0].Back.File != "" || rec.Sheets[0].Front.File == "" || rec.Sheets[1].Back.Blank {
		t.Fatalf("letter.yaml sheets %+v, %v", rec.Sheets, err)
	}
	page(t, h, "/letters/0", "Sheet 1, front", "Sheet 1, back", "Blank: left out of the letter", "Envelope, back", `name="side" value="sheet-01-front"`)
	body := get(t, h, "/letters/0").Body.String()
	for _, s := range []string{"sheet-01-back", "Not developed yet"} {
		if strings.Contains(body, s) {
			t.Errorf("the letter's page has %q for a side left out", s)
		}
	}

	act(t, a, h, loc+"/accept", nil)
	page(t, h, loc, "Done", "1 letter held")
	page(t, h, "/letters/0", "Held by the archive", "Blank: left out of the letter")
}
