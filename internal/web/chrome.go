package web

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"maps"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/glossary"
)

// indicator is the navbar's word on the scanner: idle, or busy with a
// batch and for how long.
func (a *App) indicator() template.HTML {
	a.work.mu.Lock()
	key, since := a.work.scan, a.work.started
	a.work.mu.Unlock()
	if key == "" {
		return `<span class="navbar-item"><span class="tag is-light">Idle</span></span>`
	}
	d := time.Since(since).Round(time.Second)
	return template.HTML(fmt.Sprintf(`<a class="navbar-item" href="%s"><span class="tag is-warning">Scan %s running for %d:%02d</span></a>`,
		scanURL(key), html.EscapeString(key), int(d.Minutes()), int(d.Seconds())%60))
}

// archiveInstance names the instance for About, or says why it cannot.
func (a *App) archiveInstance() string {
	in, err := a.Archive.Instance()
	if err != nil {
		return err.Error()
	}
	if in.Production() {
		return in.String()
	}
	return fmt.Sprintf("%s, slot %d", in, in.Slot)
}

// archiveTag is the navbar's word on the archive: which instance the
// scanner is on, in warning colours when it is not production, so a
// scratch register is never mistaken for the real one.
func (a *App) archiveTag() template.HTML {
	in, err := a.Archive.Instance()
	switch {
	case err != nil:
		return `<span class="navbar-item"><span class="tag is-danger">archive not answering</span></span>`
	case in.Production():
		return template.HTML(`<span class="navbar-item"><span class="tag is-light">archive ` + html.EscapeString(in.Name) + `</span></span>`)
	default:
		return template.HTML(fmt.Sprintf(`<span class="navbar-item"><span class="tag is-warning">archive %s, slot %d</span></span>`, html.EscapeString(in.String()), in.Slot))
	}
}

// version is the build's module version and commit.
func version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var v strings.Builder
	v.WriteString(bi.Main.Version)
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			v.WriteString(", commit " + s.Value[:min(12, len(s.Value))])
		case "vcs.modified":
			if s.Value == "true" {
				v.WriteString(" (modified)")
			}
		}
	}
	return v.String()
}

func (a *App) about() string {
	row := func(k, v string) string { return "<tr><th>" + k + "</th><td>" + v + "</td></tr>" }
	link := func(u string) string { return fmt.Sprintf(`<a href="%s">%s</a>`, u, u) }
	return `<h1 class="title">About</h1>
<p class="subtitle">Scans family photographs on the Epson 4990 into archival masters, one archived photo per print, for i.bytestone.uk.</p>
<table class="table"><tbody>` +
		row("Version", html.EscapeString(version())) +
		row("Scanner", html.EscapeString(a.Scanner.Name())) +
		row("Staging", "<code>"+html.EscapeString(a.Staging)+"</code>") +
		row("Archive service", "<code>"+html.EscapeString(a.Archive.String())+"</code>") +
		row("Archive instance", html.EscapeString(a.archiveInstance())) +
		row("Statistics", `<a href="statistics">Batches, scans, photos and what staging holds</a>`) +
		row("Format statement", link("https://www.bytestone.uk/posts/2026-10-20-archiving-family-photographs/")) +
		row("Source", link("https://git.bytestone.uk/hum3/bytestone-scan")) +
		row("Documentation", link("https://bytestone-scan.docs.bytestone.uk")) +
		`</tbody></table>
<h2 class="subtitle">Glossary</h2>
<table class="table"><tbody>` + glossaryRows() + `</tbody></table>`
}

// statistics is the Statistics page: what staging holds, counted and
// sized, and the runs in the workflow database.
func (a *App) statistics(ctx context.Context) string {
	row := func(k string, v any) string { return fmt.Sprintf("<tr><th>%s</th><td>%v</td></tr>", k, v) }
	var s strings.Builder
	s.WriteString(`<h1 class="title">Statistics</h1>`)
	st, err := batch.Statistics(a.Staging)
	if err != nil {
		return s.String() + `<div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`
	}
	s.WriteString(`<h2 class="title is-5">Batches</h2><table class="table"><tbody>`)
	for _, state := range []string{"open", "finished", "closed", "purged"} {
		s.WriteString(row(strings.ToUpper(state[:1])+state[1:], st.Batches[state]))
	}
	s.WriteString(row("Scans", st.Scans) + `</tbody></table>`)
	s.WriteString(`<h2 class="title is-5">Photos</h2><table class="table"><tbody>` +
		row("Held", st.Photos.Held) + row("Pending", st.Photos.Pending) + row("Rejected", len(st.Photos.Rejected)) + `</tbody></table>`)
	s.WriteString(`<h2 class="title is-5">Staging</h2><p class="help"><code>` + html.EscapeString(a.Staging) + `</code></p><table class="table"><tbody>`)
	for _, u := range st.Staging {
		s.WriteString(row("<code>"+html.EscapeString(u.Name)+"</code>", megabytes(u.Bytes)))
	}
	s.WriteString(row("Total", megabytes(st.Total())) + `</tbody></table>`)
	counts, err := a.Store.CountRuns(ctx)
	if err != nil {
		return s.String() + `<div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`
	}
	s.WriteString(`<h2 class="title is-5">Runs</h2><p class="help">In the workflow database, by workflow and status.</p><table class="table"><thead><tr><th>Workflow</th><th>Status</th><th>Runs</th></tr></thead><tbody>`)
	for _, t := range slices.Sorted(maps.Keys(counts)) {
		for _, status := range slices.Sorted(maps.Keys(counts[t])) {
			fmt.Fprintf(&s, "<tr><td>%s</td><td>%s</td><td>%d</td></tr>", html.EscapeString(string(t)), html.EscapeString(string(status)), counts[t][status])
		}
		fmt.Fprintf(&s, "<tr><td>%s</td><td>all</td><td>%d</td></tr>", html.EscapeString(string(t)), counts[t].Total())
	}
	s.WriteString(`</tbody></table>`)
	return s.String()
}

func glossaryRows() string {
	var s strings.Builder
	for _, t := range glossary.Terms() {
		s.WriteString("<tr><th>" + html.EscapeString(t.Term) + "</th><td>" + html.EscapeString(t.Meaning) + "</td></tr>")
	}
	return s.String()
}
