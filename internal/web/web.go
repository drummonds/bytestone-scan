// Package web is bytestone-scan's lofigui app: the batches as
// gobank-workflow's work queue, each batch's prints, and each photo's
// quality page.
package web

import (
	"errors"
	"fmt"
	"html"
	"html/template"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/view"
	"git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/quality"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
)

const (
	FitWidth = 1000 // widest the whole master is shown
	CropSize = 512  // side of a 1:1 crop, in master pixels
)

// App serves the pages over a staging root and the workflow store, and
// runs a batch's steps on the scanner.
type App struct {
	Staging string
	Store   wf.Store
	Scanner batch.Scanner
	People  register.Register
	Now     func() time.Time

	mu     sync.Mutex // the last master decoded, since a page and its images all want it
	cached pid.ID
	img    *master.Image

	work worker
}

const layout = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<base href="/">
<title>{{.title}} — bytestone-scan</title>
{{if .refresh}}<meta http-equiv="refresh" content="3">{{end}}
<link rel="stylesheet" href="assets/bulma.min.css">
</head><body>
<nav class="navbar is-primary"><div class="navbar-brand">
<a class="navbar-item has-text-weight-bold" href="workflows/">bytestone-scan</a>
<a class="navbar-item" href="workflows/">Batches</a>
<a class="navbar-item" href="batches/new">New batch</a>
<a class="navbar-item" href="workflows/definitions">Workflows</a>
<a class="navbar-item" href="about">About</a>
{{.status}}
</div></nav>
<section class="section"><div class="container">{{.content}}</div></section>
</body></html>`

func (a *App) Handler() http.Handler {
	ctrl, err := lofigui.NewControllerFromString(layout)
	if err != nil {
		panic(err)
	}
	render := func(w http.ResponseWriter, title string, content string) {
		ctrl.RenderTemplate(w, lofigui.TemplateContext{"title": title, "content": template.HTML(content), "status": a.indicator()})
	}
	refreshing := func(w http.ResponseWriter, title string, content string) {
		ctrl.RenderTemplate(w, lofigui.TemplateContext{"title": title, "content": template.HTML(content), "status": a.indicator(), "refresh": true})
	}
	views := &view.Views{
		Store:       a.Store,
		Definitions: wf.Definitions,
		Base:        "workflows/",
		Detail:      a.batchLink,
		Frame: func(w http.ResponseWriter, r *http.Request, p view.Page) {
			render(w, "Batches", string(p.HTML))
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/workflows/", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /assets/bulma.min.css", lofigui.ServeBulma)
	mux.HandleFunc("GET /favicon.ico", lofigui.ServeFavicon)
	mux.Handle("/workflows/", http.StripPrefix("/workflows", views.Handler()))

	mux.HandleFunc("GET /batches/{id}", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		content, busy := a.batchPage(r, b)
		if busy {
			refreshing(w, "Batch "+b.ID, content)
			return
		}
		render(w, "Batch "+b.ID, content)
	})
	mux.HandleFunc("GET /batches/{id}/prints.png", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(b.Dir(), "prints.png"))
	})

	mux.HandleFunc("GET /photos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, photo, img, ok := a.photo(w, r)
		if !ok {
			return
		}
		render(w, "Photo "+id.String(), a.qualityPage(r, id, photo, img))
	})
	mux.HandleFunc("GET /photos/{id}/fit.jpg", func(w http.ResponseWriter, r *http.Request) {
		if _, _, img, ok := a.photo(w, r); ok {
			w.Header().Set("Content-Type", "image/jpeg")
			jpeg.Encode(w, quality.Fit(img, FitWidth), &jpeg.Options{Quality: 90})
		}
	})
	mux.HandleFunc("GET /photos/{id}/crop.png", func(w http.ResponseWriter, r *http.Request) {
		if _, _, img, ok := a.photo(w, r); ok {
			x, _ := strconv.Atoi(r.URL.Query().Get("x"))
			y, _ := strconv.Atoi(r.URL.Query().Get("y"))
			w.Header().Set("Content-Type", "image/png")
			png.Encode(w, quality.Crop(img, image.Pt(x, y), CropSize))
		}
	})
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) { render(w, "About", a.about()) })
	a.actions(mux, render)
	return mux
}

// batchLink is what a batch's workflow page adds below its steps.
func (a *App) batchLink(_ *http.Request, run wf.RunRecord, _ []wf.StepResult, _ string) template.HTML {
	if run.WorkflowType != batch.Workflow {
		return ""
	}
	return template.HTML(fmt.Sprintf(`<p><a class="button is-link" href="batches/%s">Batch %s: prints and photos</a></p>`,
		html.EscapeString(run.Key), html.EscapeString(run.Key)))
}

// batchPage is a batch's details, where its workflow stands and what the
// operator can do there; busy reports that a step is running.
func (a *App) batchPage(r *http.Request, b *batch.Batch) (string, bool) {
	c := lofigui.NewContext()
	busy := a.state(r, c, b)
	c.HTML(fmt.Sprintf(`<h1 class="title">Batch %s</h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Home</th><td>%s (%s)</td></tr>
<tr><th>Scanned at</th><td>%d ppi</td></tr>
<tr><th>Started</th><td>%s</td></tr>
</tbody></table>`, html.EscapeString(b.ID), html.EscapeString(b.Description),
		html.EscapeString(b.PersonName), b.Person, b.ScanPPI, b.Created.Local().Format("2 Jan 2006 15:04")))

	prints, ids, err := b.Found()
	if err != nil {
		c.HTML(`<p>No preview yet.</p>`)
		return c.Buffer(), busy
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-striped"><thead><tr><th>#</th><th>Print</th><th>At</th><th>Skew</th><th>Fill</th><th>Photo</th></tr></thead><tbody>`)
	for i, p := range prints {
		photo := "—"
		if i < len(ids) {
			photo = fmt.Sprintf(`<a href="photos/%s">%s</a>`, ids[i], ids[i])
		}
		review := ""
		if p.Review {
			review = ` <span class="tag is-danger">review</span>`
		}
		fmt.Fprintf(&t, `<tr><td>%d%s</td><td>%.0f × %.0f mm</td><td>%.0f, %.0f mm</td><td>%.1f°</td><td>%.3f</td><td>%s</td></tr>`,
			i+1, review, p.Print.X1-p.Print.X0, p.Print.Y1-p.Print.Y0, p.Print.X0, p.Print.Y0, p.Skew, p.Fill, photo)
	}
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	c.HTML(fmt.Sprintf(`<figure><img src="batches/%s/prints.png" alt="The bed, with the prints found">
<figcaption class="help">Preview, 75 ppi, with the regions found: green as found, red to review.</figcaption></figure>`, html.EscapeString(b.ID)))
	return c.Buffer(), busy
}

// photo reads the photo the request names, writing a 404 when there is none.
func (a *App) photo(w http.ResponseWriter, r *http.Request) (pid.ID, batch.Photo, *master.Image, bool) {
	id, err := pid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return 0, batch.Photo{}, nil, false
	}
	photo, path, err := batch.FindPhoto(a.Staging, id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return 0, batch.Photo{}, nil, false
	}
	img, err := a.decode(id, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return 0, batch.Photo{}, nil, false
	}
	return id, photo, img, true
}

func (a *App) decode(id pid.ID, path string) (*master.Image, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.img != nil && a.cached == id {
		return a.img, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := master.Decode(f)
	if err != nil {
		return nil, err
	}
	a.cached, a.img = id, img
	return img, nil
}

func (a *App) qualityPage(r *http.Request, id pid.ID, p batch.Photo, img *master.Image) string {
	c := lofigui.NewContext()
	s := p.Front
	c.HTML(fmt.Sprintf(`<h1 class="title">Photo %s</h1>
<p class="subtitle">%s — <a href="batches/%s">batch %s</a></p>`,
		id, html.EscapeString(p.Description), html.EscapeString(p.Batch), html.EscapeString(p.Batch)))

	fitW := min(img.W, FitWidth)
	fitH := img.H * fitW / img.W
	c.HTML(fmt.Sprintf(`<div class="columns"><div class="column is-two-thirds">
<form action="photos/%s" method="get">
<input type="image" name="at" src="photos/%s/fit.jpg" width="%d" height="%d" alt="The master" style="max-width:100%%;height:auto">
</form>
<p class="help">Master, %d ppi, %d × %d px, shown scaled to fit. Click it to see master pixels at 1:1.</p>`, id, id, fitW, fitH, s.MasterPPI, img.W, img.H))
	if ax, err := strconv.Atoi(r.URL.Query().Get("at.x")); err == nil {
		ay, _ := strconv.Atoi(r.URL.Query().Get("at.y"))
		mx, my := ax*img.W/fitW, ay*img.H/fitH
		c.HTML(fmt.Sprintf(`<figure class="mt-4"><img src="photos/%s/crop.png?x=%d&amp;y=%d" alt="Master pixels at 1:1">
<figcaption class="help">1:1, master pixels, %d × %d centred on %d, %d.</figcaption></figure>`, id, mx, my, CropSize, CropSize, mx, my))
	}
	c.HTML(`</div><div class="column">`)

	// the print within the master, which covers the print and its margin
	px := func(mm float64) int { return int(mm / 25.4 * float64(s.MasterPPI)) }
	inPrint := image.Rect(px(s.Print.X0-s.Region.X0), px(s.Print.Y0-s.Region.Y0), px(s.Print.X1-s.Region.X0), px(s.Print.Y1-s.Region.Y0))
	// lid shows at curled corners and in the detection's slack at the edges
	h := quality.Analyse(img, inPrint.Inset(px(3)))
	c.HTML(`<h2 class="title is-6">Within the print, 3 mm in from its edges</h2>`)
	c.HTML(histogram(h))
	c.HTML(fmt.Sprintf(`<p class="is-size-7">Clipped shadows R %.2f%% G %.2f%% B %.2f%%; clipped highlights R %.2f%% G %.2f%% B %.2f%%</p>`,
		100*h.Shadows[0], 100*h.Shadows[1], 100*h.Shadows[2], 100*h.Highlights[0], 100*h.Highlights[1], 100*h.Highlights[2]))

	review := "no"
	if s.Review {
		review = `<span class="tag is-danger">yes</span>`
	}
	rows := [][2]string{
		{"Home", html.EscapeString(fmt.Sprintf("%s (%s)", p.Home.Name, p.Home.ID))},
		{"Print", fmt.Sprintf("%.1f × %.1f mm", s.Print.X1-s.Print.X0, s.Print.Y1-s.Print.Y0)},
		{"Master", fmt.Sprintf("%d × %d px at %d ppi", img.W, img.H, s.MasterPPI)},
		{"Scanned", fmt.Sprintf("%d ppi, averaged %d × %d", s.ScanPPI, s.Averaging, s.Averaging)},
		{"Skew", fmt.Sprintf("%.2f°", s.Skew)},
		{"Fill", fmt.Sprintf("%.3f", s.Fill)},
		{"Review", review},
		{"Captured", s.Captured.Local().Format("2 Jan 2006 15:04")},
		{"Scanner", html.EscapeString(s.Scanner)},
	}
	if s.Warning != "" {
		rows = append(rows, [2]string{"Warning", `<span class="has-text-danger">` + html.EscapeString(s.Warning) + `</span>`})
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-narrow"><tbody>`)
	for _, row := range rows {
		fmt.Fprintf(&t, "<tr><th>%s</th><td>%s</td></tr>", row[0], row[1])
	}
	t.WriteString(`</tbody></table></div></div>`)
	c.HTML(t.String())
	return c.Buffer()
}

// histogram draws the three channels as lines, scaled to the fullest bin
// that is not clipped, since a clipped spike would flatten the rest.
func histogram(h quality.Histogram) string {
	const w, ht = 256, 120
	peak := 1
	for c := 0; c < 3; c++ {
		for _, n := range h.Bins[c][1:255] {
			peak = max(peak, n)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" width="100%%" role="img" aria-label="Histogram"><rect width="%d" height="%d" fill="#f5f5f5"/>`, w, ht, w, ht)
	for c, col := range []string{"#d33", "#3a3", "#36d"} {
		b.WriteString(`<polyline fill="none" stroke-width="1" stroke="` + col + `" points="`)
		for i, n := range h.Bins[c] {
			y := ht - min(ht, n*ht/peak)
			fmt.Fprintf(&b, "%d,%d ", i, y)
		}
		b.WriteString(`"/>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
}
