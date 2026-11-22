package web

import (
	"image/jpeg"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// pagesShown are the pictures under Pages on the letter's page, in the
// order shown: each as its side and file, "sheet-01-front/page-01.jpg".
func pagesShown(t *testing.T, h http.Handler) string {
	t.Helper()
	body := get(t, h, "/letters/0").Body.String()
	_, pages, ok := strings.Cut(body, `id="pages"`)
	if !ok {
		t.Fatalf("the letter's page has no Pages:\n%s", body)
	}
	var shown []string
	for _, m := range regexp.MustCompile(`<a href="letters/0#(sheet-[^"]+)"><img src="letters/0/renditions/\d+/(sheet-[^/]+)/([^"]+)"`).FindAllStringSubmatch(pages, -1) {
		if m[1] != m[2] {
			t.Errorf("a page of %s links to %s", m[2], m[1])
		}
		shown = append(shown, m[2]+"/"+m[3])
	}
	return strings.Join(shown, " ")
}

// Story 202: on the letter's page, before accepting, the operator turns
// a sheet's sides together or one alone, marks a side folded in half and
// numbers the pages in the order they are read; Pages shows them so,
// each linked to its side. Accepted, Pages stays and the layout is no
// longer offered.
func TestLayingOutALettersPages(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	// one folded sheet, its envelope, and a sheet whose back is left out
	a.Feeder = &batchtest.Feeder{Sides: []uint8{40, 50, 90, 120, 60, 255}}
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"Letters home"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"feed"}})
	loc := act(t, a, h, "/batches/20261020-1/feeds", url.Values{"description": {"From Ada, 1951"}})
	act(t, a, h, loc+"/arrange", url.Values{"letter-1": {"1"}, "role-1": {"sheet"}, "letter-2": {"1"}, "role-2": {"envelope"}, "letter-3": {"1"}, "role-3": {"sheet"}, "blank-3-back": {"on"}})
	v := strconv.Itoa(rendition.Version)
	layout := func() string {
		t.Helper()
		rec, _, err := batch.FindLetter(a.Staging, 0)
		if err != nil {
			t.Fatal(err)
		}
		var sides []string
		for _, s := range rec.EverySide() {
			if s.Blank {
				sides = append(sides, "blank")
				continue
			}
			side := strconv.Itoa(s.Turn) + "["
			for i, p := range s.Pages {
				if i > 0 {
					side += " "
				}
				if p.Blank {
					side += "-"
				} else {
					side += strconv.Itoa(p.N)
				}
			}
			sides = append(sides, side+"]")
		}
		return strings.Join(sides, " ")
	}
	is := func(what, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s:\n got %s\nwant %s", what, got, want)
		}
	}

	// nothing done: one page a side in sheet order; an envelope's side
	// is offered what a sheet's is, a side left out nothing
	is("layout", layout(), "0[1] 0[2] 0[3] 0[4] 0[5] blank")
	is("Pages", pagesShown(t, h), "sheet-01-front/page-01.jpg sheet-01-back/page-02.jpg sheet-02-front/page-03.jpg sheet-02-back/page-04.jpg sheet-03-front/page-05.jpg")
	page(t, h, "/letters/0", "Pages", "Folded in half", "This side only", `action="letters/0/fold"`, `action="letters/0/pages"`,
		`name="page-sheet-01-front-1" value="1"`, `name="page-sheet-02-back-1" value="4"`, `name="blank-sheet-02-back-1"`, `id="sheet-02-back"`, "Number the pages")
	body := get(t, h, "/letters/0").Body.String()
	if strings.Count(body, `name="alone"`) != 8 {
		t.Errorf("This side only is offered %d times, want twice for each of the four sides whose sheet has another", strings.Count(body, `name="alone"`))
	}
	for _, s := range []string{"sheet-03-back", "page-sheet-01-front-2"} {
		if strings.Contains(body, s) {
			t.Errorf("the letter's page has %q", s)
		}
	}
	if w := get(t, h, "/letters/0/renditions/"+v+"/sheet-01-back/page-02.jpg"); w.Code != http.StatusOK {
		t.Errorf("page picture: %d", w.Code)
	} else if _, err := jpeg.Decode(w.Body); err != nil {
		t.Errorf("page picture: %v", err)
	}
	if w := get(t, h, "/letters/0/renditions/"+v+"/sheet-01-back/recipe.yaml"); w.Code != http.StatusNotFound {
		t.Errorf("recipe.yaml served: %d", w.Code)
	}

	// turns: together, then one alone
	act(t, a, h, "/letters/0/turn", url.Values{"side": {"sheet-01-front"}, "by": {"90"}})
	is("the front turned", layout(), "90[1] 270[2] 0[3] 0[4] 0[5] blank")
	act(t, a, h, "/letters/0/turn", url.Values{"side": {"sheet-01-front"}, "by": {"-90"}, "alone": {"1"}})
	is("this side only", layout(), "0[1] 270[2] 0[3] 0[4] 0[5] blank")
	act(t, a, h, "/letters/0/turn", url.Values{"side": {"sheet-01-back"}, "by": {"90"}, "alone": {"1"}})

	// folded: the sheet's other side with it, numbered again
	act(t, a, h, "/letters/0/fold", url.Values{"side": {"sheet-01-back"}, "folded": {"1"}})
	is("folded", layout(), "0[1 2] 0[3 4] 0[5] 0[6] 0[7] blank")
	page(t, h, "/letters/0", "Take the fold off", `name="page-sheet-01-front-2" value="2"`, "folded in half")

	// numbered as the letter is read; the envelope's back and half the
	// last sheet are blank
	numbers := url.Values{
		"page-sheet-01-front-1": {"4"}, "page-sheet-01-front-2": {"1"}, "page-sheet-01-back-1": {"2"}, "page-sheet-01-back-2": {"3"},
		"page-sheet-02-front-1": {"6"}, "page-sheet-02-back-1": {"4"}, "blank-sheet-02-back-1": {"on"}, "page-sheet-03-front-1": {"5"},
	}
	if loc2 := act(t, a, h, "/letters/0/pages", numbers); loc2 != "/letters/0" {
		t.Errorf("numbering went to %s", loc2)
	}
	is("numbered", layout(), "0[4 1] 0[2 3] 0[6] 0[-] 0[5] blank")
	is("Pages in reading order", pagesShown(t, h), "sheet-01-front/page-01.jpg sheet-01-back/page-02.jpg sheet-01-back/page-03.jpg sheet-01-front/page-04.jpg sheet-03-front/page-05.jpg sheet-02-front/page-06.jpg")
	page(t, h, "/letters/0", `name="blank-sheet-02-back-1" checked`, "Page 1: Sheet 1, front, right", "Page 6: Envelope, front")
	if _, err := os.Stat(filepath.Join(a.renditionDir(0), "sheet-02-back", "display.jpg")); err != nil {
		t.Errorf("the side whose page is blank lost its own renditions: %v", err)
	}

	// refused, saying why, and nothing changed
	for say, change := range map[string][2]string{
		"page 2 is given twice": {"page-sheet-01-back-2", "2"},
		"page 3 is missing":     {"page-sheet-01-back-2", "7"},
		"neither a number nor":  {"page-sheet-01-back-2", ""},
		"a letter needs a page": {"", ""},
	} {
		form := url.Values{}
		maps.Copy(form, numbers)
		if change[0] == "" {
			for _, s := range []string{"sheet-01-front-1", "sheet-01-front-2", "sheet-01-back-1", "sheet-01-back-2", "sheet-02-front-1", "sheet-03-front-1"} {
				form.Set("blank-"+s, "on")
			}
		} else {
			form.Set(change[0], change[1])
		}
		if w := post(t, h, "/letters/0/pages", form); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), say) {
			t.Errorf("%s: %d %s", say, w.Code, w.Body)
		}
		is(say+": nothing changed", layout(), "0[4 1] 0[2 3] 0[6] 0[-] 0[5] blank")
	}

	// the fold taken off: numbered again in sheet order
	act(t, a, h, "/letters/0/fold", url.Values{"side": {"sheet-01-front"}, "folded": {"0"}})
	is("unfolded", layout(), "0[1] 0[2] 0[3] 0[4] 0[5] blank")
	act(t, a, h, "/letters/0/fold", url.Values{"side": {"sheet-03-front"}, "folded": {"1"}})
	is("a side whose other was left out", layout(), "0[1] 0[2] 0[3] 0[4] 0[5 6] blank")

	act(t, a, h, loc+"/accept", nil)
	page(t, h, "/letters/0", "Held by the archive", "Pages", "Page 6: Sheet 3, front, right")
	is("Pages after accepting", pagesShown(t, h), "sheet-01-front/page-01.jpg sheet-01-back/page-02.jpg sheet-02-front/page-03.jpg sheet-02-back/page-04.jpg sheet-03-front/page-05.jpg sheet-03-front/page-06.jpg")
	body = get(t, h, "/letters/0").Body.String()
	for _, s := range []string{"Folded in half", "Take the fold off", `name="page-`, "Number the pages", "This side only", `name="by"`} {
		if strings.Contains(body, s) {
			t.Errorf("an accepted letter's page still offers %q", s)
		}
	}
	for path, form := range map[string]url.Values{
		"/letters/0/fold":  {"side": {"sheet-01-front"}, "folded": {"1"}},
		"/letters/0/pages": {"page-sheet-01-front-1": {"1"}},
	} {
		if w := post(t, h, path, form); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s after accept: %d", path, w.Code)
		}
	}
	if w := post(t, h, "/letters/9/fold", url.Values{"side": {"sheet-01-front"}, "folded": {"1"}}); w.Code != http.StatusNotFound {
		t.Errorf("fold of an unknown letter: %d", w.Code)
	}
}

// Story 202: a letter accepted before this story has no pages recorded
// and no page pictures: Pages shows each side as one page in sheet
// order, by the side's own picture.
func TestALetterAcceptedBeforePagesShowsASideAsAPage(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	a.Feeder = &batchtest.Feeder{Sides: []uint8{40, 50}}
	h := a.Handler()
	act(t, a, h, "/batches", url.Values{"title": {"Letters home"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}, "then": {"feed"}})
	loc := act(t, a, h, "/batches/20261020-1/feeds", url.Values{})
	act(t, a, h, loc+"/arrange", url.Values{"letter-1": {"1"}, "role-1": {"sheet"}})
	act(t, a, h, loc+"/accept", nil)
	// as an older build left it: no pages in letter.yaml, renditions at
	// recipe 2 with no page pictures
	path := filepath.Join(a.Staging, "bags", "0", "data", "letter.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	old := regexp.MustCompile(`(?m)^ +pages:\n +- number: \d+\n`).ReplaceAll(raw, nil)
	if strings.Contains(string(old), "pages") {
		t.Fatalf("pages not stripped:\n%s", old)
	}
	if err := os.WriteFile(path, old, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(a.Staging, "renditions", "0")
	if err := os.Rename(filepath.Join(dir, strconv.Itoa(rendition.Version)), filepath.Join(dir, "2")); err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{"sheet-01-front", "sheet-01-back"} {
		found, _ := filepath.Glob(filepath.Join(dir, "2", side, "page-*.jpg"))
		for _, f := range found {
			os.Remove(f)
		}
	}
	if got, want := pagesShown(t, h), "sheet-01-front/display.jpg sheet-01-back/display.jpg"; got != want {
		t.Errorf("Pages: %s, want %s", got, want)
	}
	page(t, h, "/letters/0", "Page 1: Sheet 1, front", "Page 2: Sheet 1, back")
	// the pictures are the ones made at the old recipe, and are there
	page(t, h, "/letters/0", `letters/0/renditions/2/sheet-01-front/display.jpg`)
	if body := get(t, h, "/letters/0").Body.String(); strings.Contains(body, "Not developed yet") || strings.Contains(body, "renditions/"+strconv.Itoa(rendition.Version)+"/") {
		t.Error("the letter's renditions at recipe 2 are not found: its page looks for the current recipe's")
	}
	if w := get(t, h, "/letters/0/renditions/2/sheet-01-back/display.jpg"); w.Code != http.StatusOK {
		t.Errorf("the side's picture at recipe 2: %d", w.Code)
	}
	if now, _ := os.ReadFile(path); string(now) != string(old) {
		t.Errorf("letter.yaml was rewritten:\n%s", now)
	}
}
