package web

import (
	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch/batchtest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/glossary"
)

func TestEveryNavbarLinkWorks(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	h := a.Handler()
	body := get(t, h, "/workflows/").Body.String()
	links := regexp.MustCompile(`class="navbar-item[^"]*" href="([^"]+)"`).FindAllStringSubmatch(body, -1)
	if len(links) < 4 {
		t.Fatalf("navbar has %d links", len(links))
	}
	for _, l := range links {
		page(t, h, "/"+l[1])
	}
}

func TestAbout(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	page(t, a.Handler(), "/about",
		"Version", a.Staging, a.Archive.String(), "Fake Scanner",
		"https://git.bytestone.uk/hum3/bytestone-scan", "archiving-family-photographs")
}

func TestAboutHasTheGlossary(t *testing.T) {
	h := live(t, &batchtest.Scanner{}).Handler()
	want := []string{"Glossary"}
	for _, tm := range glossary.Terms() {
		want = append(want, "<th>"+html.EscapeString(tm.Term)+"</th><td>"+html.EscapeString(tm.Meaning)+"</td>")
	}
	page(t, h, "/about", want...)
}

func TestTheNavbarSaysWhenTheScannerIsBusy(t *testing.T) {
	sc := hold(&batchtest.Scanner{Prints: []batchtest.Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}})
	a := live(t, sc.Scanner)
	a.Scanner = sc
	h := a.Handler()
	if body := get(t, h, "/about").Body.String(); !strings.Contains(body, "Idle") {
		t.Error("idle scanner not shown as idle")
	}
	post(t, h, "/batches", url.Values{"title": {"first"}, "person": {"a"}, "ppi": {"600"}, "credit": {"© Example"}})
	for _, p := range []string{"/workflows/", "/about", "/batches/new"} {
		page(t, h, p, "Scan 20261020-1/1 running", `href="batches/20261020-1/scans/1"`)
	}
	close(sc.release)
	a.wait()
	page(t, h, "/workflows/", "Idle")
}

// #99: the navbar names the archive instance the scanner is on, in
// warning colours when it is not production, so a scratch register
// cannot be mistaken for the real one; About names it too.
func TestTheNavbarNamesTheArchiveInstance(t *testing.T) {
	a := live(t, &batchtest.Scanner{})
	fake := a.Archive.(*archivetest.Fake)
	h := a.Handler()
	page(t, h, "/workflows/", `<span class="tag is-warning">archive example (scratch), slot 1</span>`)
	page(t, h, "/about", "example (scratch), slot 1")
	fake.Serving = archive.Instance{Name: "bytestone", Purpose: "production"}
	page(t, h, "/workflows/", `<span class="tag is-light">archive bytestone</span>`)
	fake.Down = true
	page(t, h, "/workflows/", `<span class="tag is-danger">archive not answering</span>`)
}
