package web

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/site"
)

// fakeSite serves an item page for each of the paths given, 404 for the
// rest, as life-weaver's static site does.
func fakeSite(t *testing.T, name string, paths ...string) site.Site {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slices.Contains(paths, r.URL.Path) {
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return site.Site{Name: name, URL: srv.URL}
}

// #118: a completed scan's page offers Verify against the archive, a
// page that asks the archive afresh for each of the scan's photos and
// says how it stands: held with every checksum as receipted, or not.
func TestVerifyPage(t *testing.T) {
	a, l := scanned(t)
	// #143: the site has photo 0 but not 1, so the batch is published only in part
	uk, org := fakeSite(t, "i.bytestone.uk", "/a/items/0/", "/a/items/1/"), fakeSite(t, "i.bytestone.org", "/a/items/0/")
	a.Sites = []site.Site{uk, org}
	h := a.Handler()
	scan := "/batches/" + l.Batch + "/scans/1"
	page(t, h, scan, `href="batches/`+l.Batch+`/scans/1/verify"`, "Verify against the archive")
	page(t, h, scan+"/verify", `<h1 class="title">Verification of scan 1`, `href="batches/`+l.Batch+`/scans/1"`,
		`<a href="photos/0">0</a>`, `<a href="photos/1">1</a>`, "<td>verified</td>", "<td>3</td>",
		// #141: links to the item on both sites, by its home person; #143: each tagged with the site's answer
		`<th>Published</th>`, `<a href="`+uk.URL+`/a/items/0/">i.bytestone.uk</a> <span class="tag is-success">published</span>`,
		`<a href="`+org.URL+`/a/items/1/">i.bytestone.org</a> <span class="tag is-warning">not yet</span>`,
		`is-warning">Verified: 2 photos held by the archive, every checksum as receipted. On i.bytestone.uk 2 of 2 published; on i.bytestone.org 1 of 2.`,
		"task publish", "task promote")

	a.Sites[1] = fakeSite(t, "i.bytestone.org", "/a/items/0/", "/a/items/1/")
	page(t, h, scan+"/verify", `is-success">Verified: 2 photos held by the archive, every checksum as receipted. On i.bytestone.uk 2 of 2 published; on i.bytestone.org 2 of 2.`)

	a.Archive.(*archivetest.Fake).Forget(1)
	page(t, h, scan+"/verify", "<td>verified</td>", "<td>unknown</td>", `is-danger">Not verified: 1 of 2 photos not held by the archive as receipted. On i.bytestone.uk 2 of 2 published`)

	a.Sites[0] = site.Site{Name: "i.bytestone.uk", URL: "http://127.0.0.1:1"}
	page(t, h, scan+"/verify", `<span class="tag is-danger">unreachable</span>`, "On i.bytestone.uk 0 of 2 (unreachable)")
}
