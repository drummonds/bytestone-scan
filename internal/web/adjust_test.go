package web

import (
	"image/png"
	"math"
	"net/http"
	"net/url"
	"regexp"
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
	loc := act(t, a, h, "/batches", url.Values{"description": {"two"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
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
	loc := act(t, a, h, "/batches", url.Values{"description": {"one"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	b, _ := batch.Open(a.Staging, strings.TrimPrefix(loc, "/batches/"))
	prints, _, _ := b.Found()
	before := prints[0].Region

	w := post(t, h, loc+"/adjust", url.Values{"move": {"1 right 3"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("adjust: %d %s", w.Code, w.Body)
	}
	prints, _, _ = b.Found()
	if got, want := prints[0].Region.Bounds().X1, before.Bounds().X1+3; math.Abs(got-want) > 1e-9 {
		t.Errorf("right edge at %.2f, want %.2f", got, want)
	}
	if w := post(t, h, loc+"/adjust", url.Values{"move": {"1 sideways 3"}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("a bad move: %d", w.Code)
	}

	act(t, a, h, loc+"/confirm", nil)
	if body := get(t, h, loc).Body.String(); strings.Contains(body, `name="move"`) {
		t.Error("arrows offered after confirming")
	}
}

func TestArrowsOnTheQualityPageRescan(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 90, Y0: 25, X1: 125, Y1: 60, Value: 40000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"description": {"one"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	page(t, h, "/photos/0", `action="`+loc[1:]+`/reshape"`, arrow(1, "right 0.5"), arrow(1, "left -0.5"))

	b, _ := batch.Open(a.Staging, strings.TrimPrefix(loc, "/batches/"))
	prints, _, _ := b.Found()
	before := prints[0].Region
	act(t, a, h, loc+"/reshape", url.Values{"move": {"1 right -0.5"}})
	if s.Details != 2 {
		t.Errorf("%d detail passes, want a rescan", s.Details)
	}
	prints, _, _ = b.Found()
	if got, want := prints[0].Region.Bounds().X1, before.Bounds().X1-0.5; math.Abs(got-want) > 1e-9 {
		t.Errorf("right edge %.2f, want %.2f", got, want)
	}
	page(t, h, loc, "Accept")

	act(t, a, h, loc+"/accept", nil)
	if strings.Contains(get(t, h, "/photos/0").Body.String(), `name="move"`) {
		t.Error("arrows on an archived photo")
	}
}

// Story 004: a turned print's arrows move its own edges; one turned too far
// to deskew has none, and asks to be straightened.
func TestArrowsOnTurnedPrints(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 20, Y0: 30, X1: 120, Y1: 100, Angle: 5, Value: 20000},
		{X0: 20, Y0: 150, X1: 120, Y1: 220, Angle: 12, Value: 40000},
	}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"description": {"two"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	body := get(t, h, loc).Body.String()
	for _, present := range []string{"top 0.5", "right 3", "bottom -0.5", "left -3"} {
		if !strings.Contains(body, arrow(1, present)) {
			t.Errorf("print 1, turned 5°, lacks the arrow %q", present)
		}
	}
	if !regexp.MustCompile(`Print 1: 100\.\d × 70\.\d mm`).MatchString(body) {
		t.Error("print 1's size is not its own, square to it")
	}
	if !strings.Contains(body, "turned back 5.0° so the print sits square") {
		t.Error("print 1's close-up does not say it is turned")
	}
	if strings.Contains(body, `value="2 `) {
		t.Error("print 2, turned 12°, has arrows")
	}
	for _, want := range []string{"Print 2 is turned 12", "straighten it on the bed and rescan the preview"} {
		if !strings.Contains(body, want) {
			t.Errorf("print 2 lacks %q", want)
		}
	}
}

func TestSplitAndRemoveRegions(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{
		{X0: 10, Y0: 10, X1: 70, Y1: 70, Value: 20000},
		{X0: 73, Y0: 10, X1: 133, Y1: 70, Value: 20000},
		{X0: 10, Y0: 100, X1: 70, Y1: 150, Value: 20000},
	}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"description": {"three"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	body := get(t, h, loc).Body.String()
	if strings.Count(body, `action="`+loc[1:]+`/split"`) != 1 {
		t.Error("want a split button on the merged region only")
	}
	if strings.Count(body, `action="`+loc[1:]+`/remove"`) != 2 {
		t.Error("want a remove button on each region")
	}
	b, _ := batch.Open(a.Staging, strings.TrimPrefix(loc, "/batches/"))

	if w := post(t, h, loc+"/split", url.Values{"print": {"1"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("split: %d %s", w.Code, w.Body)
	}
	if prints, _, _ := b.Found(); len(prints) != 3 {
		t.Errorf("%d regions after the split, want 3", len(prints))
	}
	if w := post(t, h, loc+"/remove", url.Values{"print": {"3"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("remove: %d %s", w.Code, w.Body)
	}
	if prints, _, _ := b.Found(); len(prints) != 2 {
		t.Errorf("%d regions after the removal, want 2", len(prints))
	}
	if w := post(t, h, loc+"/split", url.Values{"print": {"1"}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("splitting a single print: %d", w.Code)
	}
}
