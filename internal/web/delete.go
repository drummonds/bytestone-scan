package web

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

// deletion is the Batches page and the operator's way of removing what
// will never close: a batch, confirmed on its own page, and the stale
// data no readable batch claims. Nothing the archive holds is touched.
func (a *App) deletion(mux *http.ServeMux, render func(http.ResponseWriter, string, string), fail func(http.ResponseWriter, error, int)) {
	mux.HandleFunc("GET /batches", func(w http.ResponseWriter, r *http.Request) {
		render(w, "Batches", a.batchesPage())
	})
	mux.HandleFunc("GET /batches/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		scans, _ := b.Scans()
		render(w, "Delete batch "+b.ID, fmt.Sprintf(`<h1 class="title">Delete batch %s?</h1>
<p class="subtitle">%s</p>
<p>Its %d scans and their photos' work files, bags and renditions go from staging, and its runs are cancelled. Nothing the archive holds is touched.</p>
<form action="batches/%s/delete" method="post" class="buttons mt-4"><button class="button is-danger" type="submit">Delete</button> <a class="button" href="batches/%s">Keep</a></form>`,
			html.EscapeString(b.ID), html.EscapeString(b.Title), len(scans), html.EscapeString(b.ID), html.EscapeString(b.ID)))
	})
	mux.HandleFunc("POST /batches/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if key := a.running(); strings.HasPrefix(key, b.ID+"/") {
			fail(w, fmt.Errorf("%w: wait for scan %s's step to finish", errBusy, key), http.StatusConflict)
			return
		}
		if err := b.Delete(r.Context(), a.Store); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		http.Redirect(w, r, "/batches", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /stale", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		stale, err := batch.ListStale(a.Staging)
		if err != nil {
			fail(w, err, http.StatusInternalServerError)
			return
		}
		for _, s := range stale {
			if s.Path == path {
				render(w, "Delete "+path, fmt.Sprintf(`<h1 class="title">Delete %s?</h1>
<p class="subtitle">%s</p>
<p>It goes from staging. Nothing the archive holds is touched.</p>
<form action="stale/delete" method="post" class="buttons mt-4"><input type="hidden" name="path" value="%s"><button class="button is-danger" type="submit">Delete</button> <a class="button" href="batches">Keep</a></form>`,
					html.EscapeString(path), html.EscapeString(s.Why), html.EscapeString(path)))
				return
			}
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("POST /stale/delete", func(w http.ResponseWriter, r *http.Request) {
		if err := batch.DeleteStale(a.Staging, r.FormValue("path")); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		http.Redirect(w, r, "/batches", http.StatusSeeOther)
	})
}

// batchesPage lists every batch in staging and where it stands, then the
// stale data no readable batch claims.
func (a *App) batchesPage() string {
	c := lofigui.NewContext()
	c.HTML(`<h1 class="title">Batches</h1>`)
	batches, err := batch.All(a.Staging)
	if err != nil {
		c.HTML(`<div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`)
		return c.Buffer()
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-striped"><thead><tr><th>Batch</th><th>Title</th><th>Home</th><th>Started</th><th>Scans</th><th>Where</th></tr></thead><tbody>`)
	for _, b := range batches {
		scans, _ := b.Scans()
		where := "open"
		if _, ok := b.Purged(); ok {
			where = "purged"
		} else if _, ok := b.Closed(); ok {
			where = "closed"
		} else if b.Finished() {
			where = "finished"
		}
		fmt.Fprintf(&t, `<tr><td><a href="batches/%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>`,
			html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(b.Title), html.EscapeString(b.PersonName),
			b.Created.Local().Format("2 Jan 2006 15:04"), len(scans), where)
	}
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	stale, err := batch.ListStale(a.Staging)
	if err != nil {
		c.HTML(`<div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`)
		return c.Buffer()
	}
	if len(stale) > 0 {
		var s strings.Builder
		s.WriteString(`<h2 class="title is-5">Stale</h2><p class="help">What no readable batch claims: left by older builds or by hand. Nothing here is held by the archive.</p>
<table class="table is-striped"><thead><tr><th>Path</th><th>Why</th><th></th></tr></thead><tbody>`)
		for _, st := range stale {
			fmt.Fprintf(&s, `<tr><td><code>%s</code></td><td>%s</td><td><a class="button is-danger is-light is-small" href="stale?path=%s">Delete</a></td></tr>`,
				html.EscapeString(st.Path), html.EscapeString(st.Why), url.QueryEscape(st.Path))
		}
		s.WriteString(`</tbody></table>`)
		c.HTML(s.String())
	}
	kept, err := batch.ListDiagnoses(a.Staging)
	if err != nil {
		c.HTML(`<div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`)
		return c.Buffer()
	}
	if len(kept) > 0 {
		var s strings.Builder
		s.WriteString(`<h2 class="title is-5">Kept for diagnosis</h2><p class="help">What a scan had when the operator kept it to be looked into: under <code>diagnosis/</code> in staging, never purged, never deposited.</p>
<table class="table is-striped"><thead><tr><th>Kept</th><th>Scan</th><th>State</th><th>Note</th><th>Size</th><th></th></tr></thead><tbody>`)
		for _, d := range kept {
			fmt.Fprintf(&s, `<tr><td><code>%s</code><br>%s</td><td><a href="%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td><td><form action="diagnosis/discard" method="post"><input type="hidden" name="name" value="%s"><button class="button is-danger is-light is-small" type="submit">Discard</button></form></td></tr>`,
				html.EscapeString(d.Name), d.Kept.Local().Format("2 Jan 2006 15:04"), scanURL(d.Scan), html.EscapeString(d.Scan), html.EscapeString(d.State), html.EscapeString(orNothing(d.Note)), megabytes(d.Size), html.EscapeString(d.Name))
		}
		s.WriteString(`</tbody></table>`)
		c.HTML(s.String())
	}
	return c.Buffer()
}

// megabytes is n bytes as the page says them.
func megabytes(n int64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%d kB", (n+1023)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
