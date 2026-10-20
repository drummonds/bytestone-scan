package web

import (
	"image/png"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
)

func arrow(n int, move string) string {
	return `value="` + strings.TrimSpace(strings.Join([]string{itoa(n), move}, " ")) + `"`
}

func itoa(n int) string { return string(rune('0' + n)) }

func TestArrowsOnlyWhereARegionCanMove(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 0, Y0: 0, X1: 60, Y1: 50, Value: 20000},    // against the bed's corner
		{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000}, // in the open
	}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"description": {"two"}, "person": {"a"}, "ppi": {"600"}})
	body := get(t, h, loc).Body.String()

	for _, absent := range []string{"top 0.5", "top 3", "left 0.5", "left 3"} {
		if strings.Contains(body, arrow(1, absent)) {
			t.Errorf("print 1 against the corner has an arrow %q", absent)
		}
	}
	for _, present := range []string{"right 3", "right 0.5", "bottom 0.5", "bottom -0.5", "top -0.5", "left -3"} {
		if !strings.Contains(body, arrow(1, present)) {
			t.Errorf("print 1 lacks the arrow %q", present)
		}
	}
	for _, present := range []string{"top 0.5", "top 3", "left 0.5", "right -0.5"} {
		if !strings.Contains(body, arrow(2, present)) {
			t.Errorf("print 2 lacks the arrow %q", present)
		}
	}
	if !strings.Contains(body, loc[1:]+"/regions/1.png") {
		t.Error("no close-up of print 1")
	}
	if _, err := png.Decode(get(t, h, loc+"/regions/2.png").Body); err != nil {
		t.Errorf("close-up: %v", err)
	}
	if _, err := png.Decode(get(t, h, loc+"/prints.png").Body); err != nil {
		t.Errorf("annotated preview: %v", err)
	}
}

func TestAnArrowMovesTheEdge(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"description": {"one"}, "person": {"a"}, "ppi": {"600"}})
	b, _ := batch.Open(a.Staging, strings.TrimPrefix(loc, "/batches/"))
	prints, _, _ := b.Found()
	before := prints[0].Region

	w := post(t, h, loc+"/adjust", url.Values{"move": {"1 right 3"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("adjust: %d %s", w.Code, w.Body)
	}
	prints, _, _ = b.Found()
	if prints[0].Region.X1 != before.X1+3 {
		t.Errorf("right edge at %.2f, want %.2f", prints[0].Region.X1, before.X1+3)
	}
	if w := post(t, h, loc+"/adjust", url.Values{"move": {"1 sideways 3"}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("a bad move: %d", w.Code)
	}

	act(t, a, h, loc+"/confirm", nil)
	if body := get(t, h, loc).Body.String(); strings.Contains(body, `name="move"`) {
		t.Error("arrows offered after confirming")
	}
}
