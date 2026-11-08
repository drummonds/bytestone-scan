package web

import (
	"image/png"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// Story 112: at the quality check each rendition has Turn buttons; a turn
// is kept on the side and the rendition made again, the caption saying
// so; none once the photo is accepted.
func TestTurnARenditionAtTheQualityCheck(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 80, Y1: 60, Value: 40000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"one"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	act(t, a, h, loc+"/confirm", nil)
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	page(t, h, "/photos/0", `action="photos/0/turn"`, `name="side" value="front"`, `name="side" value="back"`, `name="by" value="90"`, `name="by" value="-90"`, "Turn")
	if body := get(t, h, "/photos/0").Body.String(); strings.Contains(body, "then turned") {
		t.Error("a turn said before any was made")
	}

	if w := post(t, h, "/photos/0/turn", url.Values{"side": {"front"}, "by": {"90"}}); w.Code != 303 {
		t.Fatalf("turn: %d %s", w.Code, w.Body)
	}
	page(t, h, "/photos/0", "then turned 90° clockwise")
	if r, err := rendition.Load(a.renditionDir(0)); err != nil || r.Turn != 90 || r.Display.W >= r.Display.H {
		t.Errorf("front recipe %+v (%v), want turn 90 and a portrait display", r, err)
	}
	if w := post(t, h, "/photos/0/turn", url.Values{"side": {"back"}, "by": {"-90"}}); w.Code != 303 {
		t.Fatalf("turn the back: %d %s", w.Code, w.Body)
	}
	page(t, h, "/photos/0", "then turned 90° clockwise", "turned 90° anticlockwise")
	if r, err := rendition.Load(filepath.Join(a.renditionDir(0), "back")); err != nil || r.Turn != 270 {
		t.Errorf("back recipe %+v (%v), want turn 270", r, err)
	}
	if w := post(t, h, "/photos/0/turn", url.Values{"side": {"front"}, "by": {"45"}}); w.Code == 303 {
		t.Error("a turn of 45° accepted")
	}
	// the rendition is redrawn after a turn: the browser must not keep it
	if cc := get(t, h, "/photos/0/renditions/2/display.jpg").Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("display.jpg Cache-Control %q", cc)
	}

	describeAll(t, a, h, loc)
	act(t, a, h, loc+"/accept", nil)
	if strings.Contains(get(t, h, "/photos/0").Body.String(), `action="photos/0/turn"`) {
		t.Error("turn buttons on an accepted photo")
	}
	if w := post(t, h, "/photos/0/turn", url.Values{"side": {"front"}, "by": {"90"}}); w.Code == 303 {
		t.Error("a turn after accept accepted")
	}
}

// Story 114: at confirm each print's close-up has Turn buttons; the
// heading and the table say the turn; the buttons go with the arrows
// once the prints are confirmed; the front is captured with the turn.
func TestTurnAPrintAtConfirmFromTheEditor(t *testing.T) {
	s := &batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 80, Y1: 60, Value: 40000}}}
	a := live(t, s)
	h := a.Handler()
	loc := act(t, a, h, "/batches", url.Values{"title": {"one"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	page(t, h, loc, `action="`+loc[1:]+`/turn"`, `name="by" value="90"`, `name="by" value="-90"`, "Turn")
	if body := get(t, h, loc).Body.String(); strings.Contains(body, "turned 90°") {
		t.Error("a turn said before any was made")
	}
	w := post(t, h, loc+"/turn", url.Values{"print": {"1"}, "by": {"90"}})
	if w.Code != 303 || w.Header().Get("Location") != loc+"#print-1" {
		t.Fatalf("turn: %d %s %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	body := get(t, h, loc).Body.String()
	if strings.Count(body, "turned 90° clockwise") != 2 {
		t.Errorf("want the turn said in the table and the heading:\n%s", body)
	}
	// the close-up is shown turned, its arrows and rulers with it: the
	// print's left edge is now at the top, so its arrows come first and
	// point up and down, and the top ruler sets the top and bottom edges
	img, err := png.Decode(get(t, h, loc+"/regions/1.png").Body)
	if err != nil || img.Bounds().Dx() >= img.Bounds().Dy() {
		t.Errorf("close-up %v (%v) at turn 90, want it shown turned, portrait", img.Bounds(), err)
	}
	if l, tp := strings.Index(body, `title="left edge`), strings.Index(body, `title="top edge`); l < 0 || tp < 0 || l > tp {
		t.Errorf("left edge's arrows at %d, top edge's at %d: want the left edge's first, at the top", l, tp)
	}
	if !strings.Contains(body, `title="left edge out 0.5 mm">↑<`) || !strings.Contains(body, `title="top edge out 0.5 mm">→<`) {
		t.Error("want the left edge's out arrow pointing up and the top edge's pointing right")
	}
	if l, tp := strings.Index(body, `title="top edge to `), strings.Index(body, `title="left edge to `); l < 0 || tp < 0 || l > tp {
		t.Errorf("top ruler sets the top edge at %d, the left edge at %d: want the top and bottom edges on the top ruler", l, tp)
	}
	if cc := get(t, h, loc+"/regions/1.png").Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("close-up Cache-Control %q, want no-store: it is redrawn after a turn", cc)
	}
	if w := post(t, h, loc+"/turn", url.Values{"print": {"1"}, "by": {"45"}}); w.Code == 303 {
		t.Error("a turn of 45° accepted")
	}
	act(t, a, h, loc+"/confirm", nil)
	if body := get(t, h, loc).Body.String(); strings.Contains(body, `/turn"`) {
		t.Error("turn buttons on confirmed prints")
	}
	if w := post(t, h, loc+"/turn", url.Values{"print": {"1"}, "by": {"90"}}); w.Code == 303 {
		t.Error("a turn after confirm accepted")
	}
	act(t, a, h, loc+"/backs", nil)
	act(t, a, h, loc+"/confirm-backs", nil)
	page(t, h, "/photos/0", "then turned 90° clockwise")
	if r, err := rendition.Load(a.renditionDir(0)); err != nil || r.Turn != 90 {
		t.Errorf("front recipe %+v (%v), want turn 90 from confirm", r, err)
	}
}
