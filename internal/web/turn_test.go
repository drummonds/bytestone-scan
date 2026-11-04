package web

import (
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
