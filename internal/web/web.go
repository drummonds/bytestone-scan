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
	"io"
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
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
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
	views := &view.Views{
		Store:       a.Store,
		Definitions: batch.Definitions,
		Diagrams:    diagrams(),
		Base:        "workflows/",
		Detail:      a.batchLink,
		Cancel:      a.Store.CancelRun,
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
		render(w, "Batch "+b.ID, a.batchPage(r, b))
	})
	// load opens the load a request names, writing a 404 when there is none.
	load := func(w http.ResponseWriter, r *http.Request) (*batch.Load, bool) {
		l, err := batch.OpenLoad(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
		if err != nil {
			http.NotFound(w, r)
			return nil, false
		}
		return l, true
	}
	mux.HandleFunc("GET /batches/{id}/loads/{n}", func(w http.ResponseWriter, r *http.Request) {
		if l, ok := load(w, r); ok {
			render(w, fmt.Sprintf("Batch %s, load %d", l.Batch, l.N), a.loadPage(r, l))
		}
	})
	mux.HandleFunc("GET /batches/{id}/loads/{n}/progress", func(w http.ResponseWriter, r *http.Request) {
		l, ok := load(w, r)
		if !ok {
			return
		}
		busy, step, since := a.busyWith(l.Key())
		if !busy {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, a.progress(r.Context(), l, step, since))
	})
	servePNG := func(w http.ResponseWriter, r *http.Request, draw func(l *batch.Load) (image.Image, error)) {
		l, ok := load(w, r)
		if !ok {
			return
		}
		img, err := draw(l)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store") // redrawn after every adjustment
		png.Encode(w, img)
	}
	mux.HandleFunc("GET /batches/{id}/loads/{n}/prints.png", func(w http.ResponseWriter, r *http.Request) {
		servePNG(w, r, (*batch.Load).Annotated)
	})
	mux.HandleFunc("GET /batches/{id}/loads/{n}/backs.png", func(w http.ResponseWriter, r *http.Request) {
		servePNG(w, r, (*batch.Load).AnnotatedBacks)
	})
	mux.HandleFunc("GET /batches/{id}/loads/{n}/regions/{print}", func(w http.ResponseWriter, r *http.Request) {
		p, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("print"), ".png"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		servePNG(w, r, func(l *batch.Load) (image.Image, error) { return l.CloseUp(p - 1) })
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
			size, err := strconv.Atoi(r.URL.Query().Get("size"))
			if err != nil {
				size = CropSize
			}
			w.Header().Set("Content-Type", "image/png")
			png.Encode(w, quality.Crop(img, image.Pt(x, y), min(max(size, 32), 1024)))
		}
	})
	// a photo's renditions: one directory per recipe version, the back's
	// under it, served as files
	renditionFile := func(w http.ResponseWriter, r *http.Request, side string) {
		id, err := pid.Parse(r.PathValue("id"))
		version, verr := strconv.Atoi(r.PathValue("version"))
		file := r.PathValue("file")
		if err != nil || verr != nil || (file != "display.jpg" && file != "thumb.jpg" && file != "tiny.jpg") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store") // remade when the master is
		http.ServeFile(w, r, filepath.Join(a.Staging, "renditions", id.String(), strconv.Itoa(version), side, file))
	}
	mux.HandleFunc("GET /photos/{id}/renditions/{version}/{file}", func(w http.ResponseWriter, r *http.Request) {
		renditionFile(w, r, "")
	})
	mux.HandleFunc("GET /photos/{id}/renditions/{version}/back/{file}", func(w http.ResponseWriter, r *http.Request) {
		renditionFile(w, r, "back")
	})
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) { render(w, "About", a.about()) })
	a.actions(mux, render)
	return mux
}

// batchLink is what a batch's or a load's workflow page adds below its steps.
func (a *App) batchLink(_ *http.Request, run wf.RunRecord, _ []wf.StepResult, _ string) template.HTML {
	switch run.WorkflowType {
	case batch.Workflow:
		return template.HTML(fmt.Sprintf(`<p><a class="button is-link" href="batches/%s">Batch %s: its loads</a></p>`,
			html.EscapeString(run.Key), html.EscapeString(run.Key)))
	case batch.LoadWorkflow:
		return template.HTML(fmt.Sprintf(`<p><a class="button is-link" href="%s">Load %s: prints and photos</a></p>`,
			loadURL(run.Key), html.EscapeString(run.Key)))
	case batch.PhotoWorkflow:
		return template.HTML(fmt.Sprintf(`<p><a class="button is-link" href="photos/%s">Photo %s</a></p>`,
			html.EscapeString(run.Key), html.EscapeString(run.Key)))
	}
	return ""
}

// loadURL is the page of the load with key, <batch id>/<n>.
func loadURL(key string) string {
	id, n, _ := strings.Cut(key, "/")
	return "batches/" + html.EscapeString(id) + "/loads/" + html.EscapeString(n)
}

// batchPage is a batch's details, its loads and what the operator can do
// next: scan another load, or finish scanning.
func (a *App) batchPage(r *http.Request, b *batch.Batch) string {
	c := lofigui.NewContext()
	c.HTML(fmt.Sprintf(`<h1 class="title">Batch %s</h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Home</th><td>%s (%s)</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
<tr><th>Scanned at</th><td>%d ppi</td></tr>
<tr><th>Started</th><td>%s</td></tr>
</tbody></table>`, html.EscapeString(b.ID), html.EscapeString(b.Title),
		html.EscapeString(b.PersonName), b.Person, orUnknown(b.Provenance), html.EscapeString(b.Credit),
		b.ScanPPI, b.Created.Local().Format("2 Jan 2006 15:04")))
	loads, err := b.Loads()
	if err != nil {
		c.HTML(`<div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`)
		return c.Buffer()
	}
	var t strings.Builder
	t.WriteString(`<h2 class="title is-5">Loads</h2><table class="table is-striped"><thead><tr><th>#</th><th>Description</th><th>Prints</th><th>Where</th></tr></thead><tbody>`)
	for _, l := range loads {
		where := "not started"
		if busy, step, _ := a.busyWith(l.Key()); busy {
			where = "working: " + step
		} else if run, waiting := a.where(r.Context(), l); run != nil {
			where = string(run.Status)
			if waiting != "" {
				where = "waiting at " + waiting
			}
		}
		prints, _, _ := l.Found()
		fmt.Fprintf(&t, `<tr><td><a href="%s">Load %d</a></td><td>%s</td><td>%d</td><td>%s</td></tr>`,
			loadURL(l.Key()), l.N, html.EscapeString(l.Description), len(prints), html.EscapeString(where))
	}
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	if b.Finished() {
		c.HTML(`<div class="notification is-success">Finished: every load is through.</div>`)
		return c.Buffer()
	}
	c.HTML(fmt.Sprintf(`<div class="buttons"><a class="button is-primary" href="batches/%s/loads/new">Scan another load</a> `+
		`<form action="batches/%s/finish" method="post" style="display:inline"><button class="button" type="submit">Finish scanning</button></form></div>`,
		html.EscapeString(b.ID), html.EscapeString(b.ID)))
	return c.Buffer()
}

// loadPage is a load's details, where its workflow stands and what the
// operator can do there.
func (a *App) loadPage(r *http.Request, l *batch.Load) string {
	c := lofigui.NewContext()
	busy, waiting := a.state(r, c, l)
	b := l.Sitting()
	c.HTML(fmt.Sprintf(`<h1 class="title">Load %d of <a href="batches/%s">batch %s</a></h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Batch</th><td>%s</td></tr>
<tr><th>Home</th><td>%s (%s)</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
<tr><th>Scanned at</th><td>%d ppi</td></tr>
<tr><th>Started</th><td>%s</td></tr>
</tbody></table>`, l.N, html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(l.Description), html.EscapeString(b.Title),
		html.EscapeString(b.PersonName), b.Person, orUnknown(b.Provenance), html.EscapeString(b.Credit),
		b.ScanPPI, l.Created.Local().Format("2 Jan 2006 15:04")))

	prints, ids, err := l.Found()
	if err != nil {
		c.HTML(`<p>No preview yet.</p>`)
		return c.Buffer()
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-striped"><thead><tr><th>#</th><th>Print</th><th>At</th><th>Skew</th><th>Fill</th><th>Photo</th><th>Back</th></tr></thead><tbody>`)
	for i, p := range prints {
		photo, back := "—", "—"
		if i < len(ids) {
			photo = fmt.Sprintf(`<a href="photos/%s">%s</a>`, ids[i], ids[i])
			if rec, _, err := batch.FindPhoto(a.Staging, ids[i]); err == nil && rec.Back != nil {
				back = fmt.Sprintf("%.0f × %.0f mm", rec.Back.Print.W, rec.Back.Print.H)
				if rec.Back.Review {
					back = `<span class="tag is-danger">review</span> ` + html.EscapeString(rec.Back.Warning)
				}
			}
		}
		review := ""
		if p.Review {
			review = ` <span class="tag is-danger">review</span>`
		}
		fmt.Fprintf(&t, `<tr><td>%d%s</td><td>%.0f × %.0f mm</td><td>%.0f, %.0f mm</td><td>%.1f°</td><td>%.3f</td><td>%s</td><td>%s</td></tr>`,
			i+1, review, p.Print.W, p.Print.H, p.Print.Bounds().X0, p.Print.Bounds().Y0, p.Skew, p.Fill, photo, back)
	}
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	c.HTML(fmt.Sprintf(`<figure><img src="%s/prints.png" alt="The bed, with the prints found">
<figcaption class="help">Preview, 150 ppi, with the regions found: green as found, red to review.</figcaption></figure>`, loadURL(l.Key())))
	if _, err := os.Stat(filepath.Join(l.Dir(), "backs.png")); err == nil {
		c.HTML(fmt.Sprintf(`<figure class="mt-4"><img src="%s/backs.png" alt="The bed turned over, with the fronts' regions">
<figcaption class="help">The backs, 150 ppi, with the fronts' regions: each back was scanned where its front lay.</figcaption></figure>`, loadURL(l.Key())))
	}
	if waiting == "confirm" && !busy {
		for i, p := range prints {
			c.HTML(regionEditor(l, i, p))
		}
	}
	return c.Buffer()
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
	key := fmt.Sprintf("%s/%d", p.Batch, p.Load)
	c.HTML(fmt.Sprintf(`<h1 class="title">Photo %s</h1>
<p class="subtitle">%s — <a href="batches/%s">batch %s</a>, <a href="%s">load %d</a></p>`,
		id, html.EscapeString(p.Description), html.EscapeString(p.Batch), html.EscapeString(p.Batch), loadURL(key), p.Load))

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
	// the rendition: what people will see, made from the master by the
	// current recipe
	if rec, err := rendition.Load(filepath.Join(a.Staging, "renditions", id.String(), strconv.Itoa(rendition.Version))); err == nil {
		c.HTML(fmt.Sprintf(`<h2 class="title is-5 mt-5">Rendition</h2>
<figure><img src="photos/%s/renditions/%d/display.jpg" alt="The rendition" style="max-width:100%%;height:auto">
<figcaption class="help">Rendition, recipe %d, %d × %d px: cropped to the print, turned %.1f° (the region said %.1f°), levels %d–%d.</figcaption></figure>`,
			id, rec.Version, rec.Version, rec.Display.W, rec.Display.H, -rec.Skew, rec.RegionSkew, rec.Black, rec.White))
		for _, f := range rec.Flags {
			c.HTML(`<p class="has-text-danger">` + html.EscapeString(f) + `</p>`)
		}
	}
	// the back, once scanned: its rendition and what the backs preview said
	if p.Back != nil {
		c.HTML(`<h2 class="title is-5 mt-5">Back</h2>`)
		if rec, err := rendition.Load(filepath.Join(a.Staging, "renditions", id.String(), strconv.Itoa(rendition.Version), "back")); err == nil {
			c.HTML(fmt.Sprintf(`<figure><img src="photos/%s/renditions/%d/back/display.jpg" alt="The back" style="max-width:100%%;height:auto">
<figcaption class="help">Back, %d ppi, 8 bits, rendition recipe %d, %d × %d px.</figcaption></figure>`,
				id, rec.Version, p.Back.MasterPPI, rec.Version, rec.Display.W, rec.Display.H))
		}
		c.HTML(fmt.Sprintf(`<p class="help">Back %.1f × %.1f mm, scanned where the front lay.</p>`, p.Back.Print.W, p.Back.Print.H))
		if p.Back.Warning != "" {
			c.HTML(`<p class="has-text-danger">` + html.EscapeString(p.Back.Warning) + `</p>`)
		}
	}

	// the master's corners show its edges at full resolution: whether the
	// print is cut, or the margin too generous
	const corner = 256
	c.HTML(`<h2 class="title is-5 mt-5">Corners at 1:1</h2><div class="columns is-multiline is-gapless">`)
	for _, at := range []image.Point{{0, 0}, {img.W, 0}, {0, img.H}, {img.W, img.H}} {
		c.HTML(fmt.Sprintf(`<div class="column is-half"><img src="photos/%s/crop.png?x=%d&amp;y=%d&amp;size=%d" alt="Master corner at %d, %d" style="border:1px solid #ccc"></div>`,
			id, at.X, at.Y, corner, at.X, at.Y))
	}
	c.HTML(fmt.Sprintf(`</div><p class="help">Each corner of the master, %d × %d master pixels (%.1f mm square).</p>`,
		corner, corner, float64(corner)*25.4/float64(s.MasterPPI)))
	c.HTML(`</div><div class="column">`)

	// while the load waits at the quality check, the margins can be moved
	if l, err := batch.OpenLoad(a.Staging, key); err == nil && a.running() != l.Key() {
		if _, waiting := a.where(r.Context(), l); waiting == "check quality" {
			prints, ids, _ := l.Found()
			for i, pid := range ids {
				if pid == id && i < len(prints) {
					c.HTML(rescanEditor(l, i, prints[i]))
				}
			}
		}
	}

	// the print within the master, which covers the region's bounds: the
	// part of a turned print that is square to the master
	px := func(mm float64) int { return int(mm / 25.4 * float64(s.MasterPPI)) }
	in := s.Print.Inside()
	inPrint := image.Rect(px(in.X0-s.Bounds.X0), px(in.Y0-s.Bounds.Y0), px(in.X1-s.Bounds.X0), px(in.Y1-s.Bounds.Y0))
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
		{"Provenance", orUnknown(p.Provenance)},
		{"Credit", html.EscapeString(p.Credit)},
		{"Print", fmt.Sprintf("%.1f × %.1f mm", s.Print.W, s.Print.H)},
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

// orUnknown shows a provenance, or that none was known.
func orUnknown(s string) string {
	if s == "" {
		return `<span class="has-text-grey">unknown</span>`
	}
	return html.EscapeString(s)
}

// histogram draws the three channels as lines, scaled to the fullest bin
// that is not clipped, since a clipped spike would flatten the rest.
func histogram(h quality.Histogram) string {
	const w, ht = 256, 120
	peak := 1
	for c := range 3 {
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
