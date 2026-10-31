package web

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

// cleanup is the Tools menu's Clean up: the batches newest first, each
// row offering Delete below, which deletes every completed batch below
// it after a confirm page, and the orphans with them or on their own.
func (a *App) cleanup(mux *http.ServeMux, render func(http.ResponseWriter, string, string), fail func(http.ResponseWriter, error, int)) {
	mux.HandleFunc("GET /tools/cleanup", func(w http.ResponseWriter, r *http.Request) {
		plan, err := batch.PlanCleanup(r.Context(), a.Staging, a.Store, "")
		if err != nil {
			fail(w, err, http.StatusInternalServerError)
			return
		}
		all, err := batch.All(a.Staging)
		if err != nil {
			fail(w, err, http.StatusInternalServerError)
			return
		}
		render(w, "Clean up", cleanupPage(all, plan.Orphans))
	})
	mux.HandleFunc("GET /tools/cleanup/below/{id}", func(w http.ResponseWriter, r *http.Request) {
		plan, err := batch.PlanCleanup(r.Context(), a.Staging, a.Store, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		render(w, "Delete below "+plan.Below.ID, confirmCleanup(plan))
	})
	mux.HandleFunc("POST /tools/cleanup/below/{id}", func(w http.ResponseWriter, r *http.Request) {
		plan, err := batch.PlanCleanup(r.Context(), a.Staging, a.Store, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if key := a.running(); key != "" {
			for _, b := range plan.Batches {
				if strings.HasPrefix(key, b.ID+"/") {
					fail(w, fmt.Errorf("%w: wait for scan %s's step to finish", errBusy, key), http.StatusConflict)
					return
				}
			}
		}
		if err := plan.Run(r.Context(), a.Store); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		http.Redirect(w, r, "/tools/cleanup", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /tools/cleanup/orphans", func(w http.ResponseWriter, r *http.Request) {
		plan, err := batch.PlanCleanup(r.Context(), a.Staging, a.Store, "")
		if err != nil {
			fail(w, err, http.StatusInternalServerError)
			return
		}
		if err := plan.Run(r.Context(), a.Store); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		http.Redirect(w, r, "/tools/cleanup", http.StatusSeeOther)
	})
}

// cleanupPage lists the batches newest first, Delete below on each row
// with a completed batch below it, then the orphans.
func cleanupPage(all []*batch.Batch, orphans []batch.Orphan) string {
	c := lofigui.NewContext()
	c.HTML(`<h1 class="title">Clean up</h1>
<p class="subtitle">Delete below deletes every closed or purged batch below that row, as Delete does each: their work files, bags and renditions go from staging and their runs from the Queue. Open batches stay. Nothing the archive holds is touched.</p>`)
	// how many completed batches lie below each row, counted from the oldest up
	below := make([]int, len(all)+1)
	for i := len(all) - 1; i >= 0; i-- {
		below[i] = below[i+1]
		if all[i].Completed() {
			below[i]++
		}
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-striped"><thead><tr><th>Batch</th><th>Title</th><th>Started</th><th>Scans</th><th>Where</th><th></th></tr></thead><tbody>`)
	for i, b := range all {
		button := ""
		if below[i+1] > 0 {
			button = fmt.Sprintf(`<a class="button is-danger is-light is-small" href="tools/cleanup/below/%s">Delete below</a>`, html.EscapeString(b.ID))
		}
		scans, _ := b.Scans()
		fmt.Fprintf(&t, `<tr><td><a href="batches/%s">%s</a></td><td>%s</td><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(b.Title), b.Created.Local().Format("2 Jan 2006 15:04"), len(scans), where(b), button)
	}
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	if len(orphans) > 0 {
		c.HTML(`<h2 class="title is-5">Orphans</h2><p class="help">What no batch claims: bags, the archive's answers and renditions left in staging, and runs in the Queue whose batch has gone. A deletion that failed part way, or a batch moved by hand, leaves them.</p>` +
			orphanList(orphans) +
			`<form action="tools/cleanup/orphans" method="post" class="mt-2"><button class="button is-danger is-light is-small" type="submit">Remove orphans</button></form>`)
	}
	return c.Buffer()
}

// confirmCleanup says what Delete below would do before the operator
// confirms it.
func confirmCleanup(p *batch.Cleanup) string {
	var s strings.Builder
	fmt.Fprintf(&s, `<h1 class="title">Delete below %s?</h1>`, html.EscapeString(p.Below.ID))
	if len(p.Batches) == 0 {
		s.WriteString(`<p>No completed batch lies below it.</p>`)
	} else {
		fmt.Fprintf(&s, `<p>%d %s: their work files, bags and renditions go from staging and their runs from the Queue. Nothing the archive holds is touched.</p><ul>`, len(p.Batches), plural(len(p.Batches), "batch goes", "batches go"))
		for _, b := range p.Batches {
			fmt.Fprintf(&s, `<li><a href="batches/%s">%s</a> %s, %s</li>`, html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(b.Title), where(b))
		}
		s.WriteString(`</ul>`)
	}
	for _, b := range p.Kept {
		fmt.Fprintf(&s, `<p><a href="batches/%s">%s</a> stays %s.</p>`, html.EscapeString(b.ID), html.EscapeString(b.ID), where(b))
	}
	if len(p.Orphans) > 0 {
		s.WriteString(`<p>With them go the orphans no batch claims:</p>` + orphanList(p.Orphans))
	}
	fmt.Fprintf(&s, `<form action="tools/cleanup/below/%s" method="post" class="buttons mt-4"><button class="button is-danger" type="submit">Delete</button> <a class="button" href="tools/cleanup">Keep</a></form>`, html.EscapeString(p.Below.ID))
	return s.String()
}

func orphanList(orphans []batch.Orphan) string {
	var s strings.Builder
	s.WriteString(`<ul>`)
	for _, o := range orphans {
		s.WriteString(`<li><code>` + html.EscapeString(o.String()) + `</code></li>`)
	}
	s.WriteString(`</ul>`)
	return s.String()
}
