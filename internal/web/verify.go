package web

import (
	"fmt"
	"html"
	"strings"

	lofigui "git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/site"
)

// sites are where the items are published, asked in the order they get
// them.
func (a *App) sites() []site.Site {
	if a.Sites == nil {
		return site.Defaults()
	}
	return a.Sites
}

// publication is what each site answers for one item.
type publication struct {
	site   site.Site
	status site.Status
}

// published asks each site for the item's page and links it, tagged
// with the answer.
func published(sites []site.Site, ch batch.Check) ([]publication, string) {
	var pubs []publication
	var links []string
	for _, s := range sites {
		p := publication{s, s.Status(ch.Home, ch.Photo)}
		pubs = append(pubs, p)
		links = append(links, fmt.Sprintf(`<a href="%s">%s</a> <span class="tag %s">%s</span>`, s.Item(ch.Home, ch.Photo), html.EscapeString(s.Name), tagClass(p.status), p.status))
	}
	return pubs, strings.Join(links, " ")
}

func tagClass(st site.Status) string {
	switch st {
	case site.Published:
		return "is-success"
	case site.NotYet:
		return "is-warning"
	default:
		return "is-danger"
	}
}

// sitesText sums up, per site, how many of the n deposited items it has
// published, in the sites' order; a site that could not be asked for
// some item says so. complete is whether every site has every item.
func sitesText(sites []site.Site, pubs []publication, n int) (msg string, complete bool) {
	if n == 0 {
		return "", true
	}
	complete = true
	var parts []string
	for _, s := range sites {
		var published int
		unreachable := false
		for _, p := range pubs {
			if p.site != s {
				continue
			}
			switch p.status {
			case site.Published:
				published++
			case site.Unreachable:
				unreachable = true
			}
		}
		if published < n {
			complete = false
		}
		part := fmt.Sprintf("%s %d of %d", html.EscapeString(s.Name), published, n)
		if unreachable {
			part += " (unreachable)"
		}
		parts = append(parts, part)
	}
	return " On " + strings.Join(parts, " published; on ") + ".", complete
}

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
	// #143: each site asked for every deposited item, before the summary
	// that reports them
	sites := a.sites()
	var pubs []publication
	var deposited int
	cells := make([]string, len(checks))
	for i, ch := range checks {
		if ch.Verdict == batch.Pending || ch.Verdict == batch.Rejected {
			continue
		}
		deposited++
		var p []publication
		p, cells[i] = published(sites, ch)
		pubs = append(pubs, p...)
	}
	sitesMsg, complete := sitesText(sites, pubs, deposited)
	if !complete && class == "is-success" {
		class = "is-warning"
	}
	c.HTML(`<div class="notification ` + class + `">` + msg + sitesMsg + `</div>`)
	if len(checks) == 0 {
		return c.Buffer()
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-striped"><thead><tr><th>Item</th><th>Verdict</th><th>Files</th><th>Detail</th><th>Published</th></tr></thead><tbody>`)
	for i, ch := range checks {
		item := fmt.Sprintf(`<a href="photos/%s">%s</a>`, ch.Photo, ch.Photo)
		if ch.Page {
			item = fmt.Sprintf(`page <code>%s</code>`, ch.Photo)
		}
		files := ""
		if ch.Files > 0 {
			files = fmt.Sprint(ch.Files)
		}
		fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`, item, ch.Verdict, files, html.EscapeString(ch.Detail), cells[i])
	}
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	c.HTML(fmt.Sprintf(`<p class="help">The archive is asked to recompute each item's checksums from its disk (or for its receipt, when it is too old to), and the answer is compared, checksum for checksum, with the receipt taken at deposit and with the bag while it is still in staging; nothing is downloaded.
Each site is asked for the item's page, nothing downloaded: life-weaver's <code>task publish</code> puts an item on i.bytestone.uk and <code>task promote</code> then on i.bytestone.org.</p>
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
