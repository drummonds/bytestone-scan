package web

import (
	"fmt"
	"html"
	"html/template"
	"runtime/debug"
	"time"
)

// indicator is the navbar's word on the scanner: idle, or busy with a
// batch and for how long.
func (a *App) indicator() template.HTML {
	a.work.mu.Lock()
	id, since := a.work.batch, a.work.started
	a.work.mu.Unlock()
	if id == "" {
		return `<span class="navbar-item"><span class="tag is-light">Idle</span></span>`
	}
	d := time.Since(since).Round(time.Second)
	return template.HTML(fmt.Sprintf(`<a class="navbar-item" href="batches/%s"><span class="tag is-warning">Scanning batch %s for %d:%02d</span></a>`,
		html.EscapeString(id), html.EscapeString(id), int(d.Minutes()), int(d.Seconds())%60))
}

// version is the build's module version and commit.
func version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	v := bi.Main.Version
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			v += ", commit " + s.Value[:min(12, len(s.Value))]
		case "vcs.modified":
			if s.Value == "true" {
				v += " (modified)"
			}
		}
	}
	return v
}

func (a *App) about() string {
	row := func(k, v string) string { return "<tr><th>" + k + "</th><td>" + v + "</td></tr>" }
	link := func(u string) string { return fmt.Sprintf(`<a href="%s">%s</a>`, u, u) }
	return `<h1 class="title">About</h1>
<p class="subtitle">Scans family photographs on the Epson 4990 into archival masters, one bag per photo, for i.bytestone.uk.</p>
<table class="table"><tbody>` +
		row("Version", html.EscapeString(version())) +
		row("Scanner", html.EscapeString(a.Scanner.Name())) +
		row("Staging", "<code>"+html.EscapeString(a.Staging)+"</code>") +
		row("People", "<code>"+html.EscapeString(a.People.Dir)+"</code>") +
		row("Format statement", link("https://www.bytestone.uk/posts/2026-10-20-archiving-family-photographs/")) +
		row("Source", link("https://git.bytestone.uk/hum3/bytestone-scan")) +
		row("Documentation", link("https://bytestone-scan.docs.bytestone.uk")) +
		`</tbody></table>`
}
