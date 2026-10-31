package web

import (
	"fmt"
	"html"
	"strings"

	lofigui "git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

// verifyPage asks the archive afresh for each of the scan's items and
// shows how each stands: the verdict, the files the archive holds and
// what differs, under a notification that sums them up.
func (a *App) verifyPage(l *batch.Scan) string {
	c := lofigui.NewContext()
	b := l.Sitting()
	c.HTML(fmt.Sprintf(`<h1 class="title">Verification of %s %d of <a href="batches/%s">batch %s</a></h1>
<p class="subtitle">%s</p>`, strings.ToLower(kind(l)), l.N, html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(l.Description)))
	checks, err := l.Verify(a.Archive)
	if err != nil {
		c.HTML(`<div class="notification is-danger">Nothing verified: ` + html.EscapeString(err.Error()) + `</div>`)
		return c.Buffer()
	}
	msg, class := verdictText(checks)
	c.HTML(`<div class="notification ` + class + `">` + msg + `</div>`)
	if len(checks) == 0 {
		return c.Buffer()
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-striped"><thead><tr><th>Item</th><th>Verdict</th><th>Files</th><th>Detail</th></tr></thead><tbody>`)
	for _, ch := range checks {
		item := fmt.Sprintf(`<a href="photos/%s">%s</a>`, ch.Photo, ch.Photo)
		if ch.Page {
			item = fmt.Sprintf(`page <code>%s</code>`, ch.Photo)
		}
		files := ""
		if ch.Files > 0 {
			files = fmt.Sprint(ch.Files)
		}
		fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`, item, ch.Verdict, files, html.EscapeString(ch.Detail))
	}
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	c.HTML(fmt.Sprintf(`<p class="help">Each item's receipt is asked of the archive afresh and compared, checksum for checksum, with the receipt taken at deposit and with the bag while it is still in staging; nothing is downloaded.</p>
<div class="buttons"><a class="button" href="%s">‹ Back to the scan</a></div>`, scanURL(l.Key())))
	return c.Buffer()
}

// verdictText sums the checks up and picks the Bulma class that colours
// it: danger when any item is not held as receipted, warning when some
// are pending or rejected, success when every item is verified.
func verdictText(checks []batch.Check) (msg, class string) {
	if len(checks) == 0 {
		return "Nothing to verify: the scan has no items yet.", "is-warning"
	}
	var verified, failed, pending, rejected int
	for _, c := range checks {
		switch c.Verdict {
		case batch.Verified:
			verified++
		case batch.Pending:
			pending++
		case batch.Rejected:
			rejected++
		default:
			failed++
		}
	}
	noun := plural(len(checks), "photo", "photos")
	for _, c := range checks {
		if c.Page {
			noun = "items"
		}
	}
	if failed > 0 {
		return fmt.Sprintf("Not verified: %d of %d %s not held by the archive as receipted.", failed, len(checks), noun), "is-danger"
	}
	msg, class = fmt.Sprintf("Verified: %d %s held by the archive, every checksum as receipted", verified, noun), "is-success"
	if rejected > 0 {
		msg += fmt.Sprintf("; %d rejected as a duplicate, no item to verify", rejected)
		class = "is-warning"
	}
	if pending > 0 {
		msg += fmt.Sprintf("; %d not yet deposited", pending)
		class = "is-warning"
	}
	return msg + ".", class
}
