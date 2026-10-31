package web

import (
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
)

// #118: a completed scan's page offers Verify against the archive, a
// page that asks the archive afresh for each of the scan's photos and
// says how it stands: held with every checksum as receipted, or not.
func TestVerifyPage(t *testing.T) {
	a, l := scanned(t)
	h := a.Handler()
	scan := "/batches/" + l.Batch + "/scans/1"
	page(t, h, scan, `href="batches/`+l.Batch+`/scans/1/verify"`, "Verify against the archive")
	page(t, h, scan+"/verify", `<h1 class="title">Verification of scan 1`, `href="batches/`+l.Batch+`/scans/1"`,
		`<a href="photos/0">0</a>`, `<a href="photos/1">1</a>`, "<td>verified</td>", "<td>3</td>",
		`is-success">Verified: 2 photos held by the archive, every checksum as receipted`)

	a.Archive.(*archivetest.Fake).Forget(1)
	page(t, h, scan+"/verify", "<td>verified</td>", "<td>unknown</td>", `is-danger">Not verified: 1 of 2 photos`)
}
