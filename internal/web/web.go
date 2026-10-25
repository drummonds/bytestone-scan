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

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/quality"
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
	Archive archive.Service
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
<a class="navbar-item" href="workflows/">Queue</a>
<a class="navbar-item" href="batches">Batches</a>
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
	// scan opens the scan a request names, writing a 404 when there is none.
	scan := func(w http.ResponseWriter, r *http.Request) (*batch.Scan, bool) {
		l, err := batch.OpenScan(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
		if err != nil {
			http.NotFound(w, r)
			return nil, false
		}
		return l, true
	}
	mux.HandleFunc("GET /batches/{id}/scans/{n}", func(w http.ResponseWriter, r *http.Request) {
		if l, ok := scan(w, r); ok {
			render(w, fmt.Sprintf("Scan %d of batch %s", l.N, l.Batch), a.scanPage(r, l))
		}
	})
	mux.HandleFunc("GET /batches/{id}/scans/{n}/progress", func(w http.ResponseWriter, r *http.Request) {
		l, ok := scan(w, r)
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
	servePNG := func(w http.ResponseWriter, r *http.Request, draw func(l *batch.Scan) (image.Image, error)) {
		l, ok := scan(w, r)
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
	mux.HandleFunc("GET /batches/{id}/scans/{n}/prints.png", func(w http.ResponseWriter, r *http.Request) {
		servePNG(w, r, (*batch.Scan).Annotated)
	})
	mux.HandleFunc("GET /batches/{id}/scans/{n}/backs.png", func(w http.ResponseWriter, r *http.Request) {
		servePNG(w, r, (*batch.Scan).AnnotatedBacks)
	})
	mux.HandleFunc("GET /batches/{id}/scans/{n}/regions/{print}", func(w http.ResponseWriter, r *http.Request) {
		p, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("print"), ".png"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		servePNG(w, r, func(l *batch.Scan) (image.Image, error) { return l.CloseUp(p - 1) })
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

// batchLink is what a batch's or a scan's workflow page adds below its steps.
func (a *App) batchLink(_ *http.Request, run wf.RunRecord, _ []wf.StepResult, _ string) template.HTML {
	switch run.WorkflowType {
	case batch.Workflow:
		return template.HTML(fmt.Sprintf(`<p><a class="button is-link" href="batches/%s">Batch %s: its scans</a></p>`,
			html.EscapeString(run.Key), html.EscapeString(run.Key)))
	case batch.ScanWorkflow:
		return template.HTML(fmt.Sprintf(`<p><a class="button is-link" href="%s">Scan %s: prints and photos</a></p>`,
			scanURL(run.Key), html.EscapeString(run.Key)))
	case batch.PhotoWorkflow:
		return template.HTML(fmt.Sprintf(`<p><a class="button is-link" href="photos/%s">Photo %s</a></p>`,
			html.EscapeString(run.Key), html.EscapeString(run.Key)))
	}
	return ""
}

// scanURL is the page of the scan with key, <batch id>/<n>.
func scanURL(key string) string {
	id, n, _ := strings.Cut(key, "/")
	return "batches/" + html.EscapeString(id) + "/scans/" + html.EscapeString(n)
}

// batchPage is a batch's details, its scans and what the operator can do
// next: do one more scan, or finish scanning.
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
	scans, err := b.Scans()
	if err != nil {
		c.HTML(`<div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`)
		return c.Buffer()
	}
	var t strings.Builder
	t.WriteString(`<h2 class="title is-5">Scans</h2><table class="table is-striped"><thead><tr><th>#</th><th>Description</th><th>Prints</th><th>Where</th></tr></thead><tbody>`)
	for _, l := range scans {
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
		fmt.Fprintf(&t, `<tr><td><a href="%s">Scan %d</a></td><td>%s</td><td>%d</td><td>%s</td></tr>`,
			scanURL(l.Key()), l.N, html.EscapeString(l.Description), len(prints), html.EscapeString(where))
	}
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	if at, ok := b.Purged(); ok {
		c.HTML(fmt.Sprintf(`<div class="notification">Purged on %s: the work files of this batch are gone; its photos are held by the archive.</div>`, at.Local().Format("2 Jan 2006")))
		return c.Buffer()
	}
	if at, ok := b.Closed(); ok {
		msg, class := a.batchOutcome(b)
		c.HTML(fmt.Sprintf(`<div class="notification %s">Closed on %s: %s. The work files are purged %d days after closing, or sooner when staging is short of space.</div>`,
			class, at.Local().Format("2 Jan 2006"), msg, int(batch.DefaultGrace.Hours()/24)))
		return c.Buffer()
	}
	// not closed: the operator can still delete it, so long as the archive holds none of its photos
	del := fmt.Sprintf(`<a class="button is-danger is-light" href="batches/%s/delete">Delete</a>`, html.EscapeString(b.ID))
	if b.Finished() {
		msg, class := a.batchOutcome(b)
		c.HTML(fmt.Sprintf(`<div class="notification %s">Finished: %s.</div><div class="buttons">%s</div>`, class, msg, del))
		return c.Buffer()
	}
	c.HTML(fmt.Sprintf(`<div class="buttons"><a class="button is-primary" href="batches/%s/scans/new">Do one more scan</a> `+
		`<form action="batches/%s/finish" method="post" style="display:inline"><button class="button" type="submit">Finish scanning</button></form> %s</div>`,
		html.EscapeString(b.ID), html.EscapeString(b.ID), del))
	return c.Buffer()
}

// batchOutcome says how the batch's photos stand with the archive, and
// the class that colours it.
func (a *App) batchOutcome(b *batch.Batch) (msg, class string) {
	o, err := b.Outcome()
	if err != nil {
		return html.EscapeString(err.Error()), "is-danger"
	}
	return outcomeText(o)
}

// scanPage is a scan's details, where its workflow stands and what the
// operator can do there.
func (a *App) scanPage(r *http.Request, l *batch.Scan) string {
	c := lofigui.NewContext()
	busy, waiting := a.state(r, c, l)
	b := l.Sitting()
	c.HTML(fmt.Sprintf(`<h1 class="title">Scan %d of <a href="batches/%s">batch %s</a></h1>
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
		c.HTML(a.diagnosisSection(l))
		return c.Buffer()
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-striped"><thead><tr><th>#</th><th>Print</th><th>At</th><th>Skew</th><th>Fill</th><th>Photo</th><th>Back</th></tr></thead><tbody>`)
	backsFound, _ := l.BacksFound()
	for i, p := range prints {
		photo, back := "—", "—"
		if i < len(ids) {
			photo = fmt.Sprintf(`<a href="photos/%s">%s</a>`, ids[i], ids[i])
			back = a.backCell(l, ids[i], i, backsFound)
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
	cut := ""
	if l.Confirmed() {
		cut = ", cut to the regions"
	}
	c.HTML(fmt.Sprintf(`<figure><img src="%s/prints.png" alt="The bed, with the prints found">
<figcaption class="help">Preview, 150 ppi%s, with the regions found: green as found, red to review.</figcaption></figure>`, scanURL(l.Key()), cut))
	if _, err := os.Stat(filepath.Join(l.Dir(), "backs.png")); err == nil {
		c.HTML(fmt.Sprintf(`<figure class="mt-4"><img src="%s/backs.png" alt="The bed turned over, with the fronts' regions">
<figcaption class="help">The backs, 150 ppi%s: the fronts' regions in green (red to review), where each back is scanned, and each back as found in blue.</figcaption></figure>`, scanURL(l.Key()), cut))
	}
	if waiting == "confirm" && !busy {
		for i, p := range prints {
			c.HTML(regionEditor(l, i, p))
		}
	}
	c.HTML(a.diagnosisSection(l))
	return c.Buffer()
}

// backCell is the scan page's word on print i's back: scanned, found on
// the backs preview, skipped, or declared blank, with the button that
// declares it blank, or not after all while the backs are still to scan.
func (a *App) backCell(l *batch.Scan, id pid.ID, i int, found []batch.Side) string {
	blankButton := func(blank bool, label string) string {
		v := "0"
		if blank {
			v = "1"
		}
		return fmt.Sprintf(` <form action="%s/blank-back" method="post" style="display:inline"><input type="hidden" name="photo" value="%s"><input type="hidden" name="blank" value="%s"><button class="button is-small is-light" type="submit">%s</button></form>`,
			scanURL(l.Key()), id, v, label)
	}
	rec, _, err := batch.FindPhoto(a.Staging, id)
	archived := bag.Complete(filepath.Join(a.Staging, "bags"), id)
	switch {
	case err == nil && rec.BackBlank:
		if l.BacksConfirmed() || archived {
			return "blank"
		}
		return "blank" + blankButton(false, "Not blank")
	case err == nil && rec.Back != nil:
		cell := fmt.Sprintf("%.0f × %.0f mm", rec.Back.Print.W, rec.Back.Print.H)
		if rec.Back.Review {
			cell = `<span class="tag is-danger">review</span> ` + html.EscapeString(rec.Back.Warning)
		}
		if !archived {
			cell += blankButton(true, "Blank")
		}
		return cell
	case l.BacksSkipped():
		return "none"
	case i < len(found) && !l.BacksConfirmed():
		cell := fmt.Sprintf("found %.0f × %.0f mm", found[i].Print.W, found[i].Print.H)
		if found[i].Review {
			cell = `<span class="tag is-danger">review</span> ` + html.EscapeString(found[i].Warning)
		}
		return cell + blankButton(true, "Blank")
	}
	return "—"
}

// diagnosisSection is the scan page's Keep for diagnosis form, at every
// state, and what has been kept of this scan so far.
func (a *App) diagnosisSection(l *batch.Scan) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<h2 class="title is-5 mt-5">Diagnosis</h2>
<form action="%s/keep" method="post" class="box">
<p class="help">When something has gone wrong, keep what the scan has now, with a note, where a purge and a batch delete leave it. The scan itself is untouched.</p>
<div class="field has-addons"><div class="control is-expanded"><input class="input is-small" type="text" name="note" placeholder="what you saw"></div>
<div class="control"><button class="button is-small" type="submit">Keep for diagnosis</button></div></div>
<label class="checkbox is-size-7"><input type="checkbox" name="masters"> include the masters</label>
</form>`, scanURL(l.Key()))
	kept, _ := batch.ListDiagnoses(a.Staging)
	var rows []string
	for _, d := range kept {
		if d.Scan == l.Key() {
			rows = append(rows, fmt.Sprintf(`<li><code>%s</code> %s: %s</li>`, html.EscapeString(d.Name), d.Kept.Local().Format("2 Jan 2006 15:04"), html.EscapeString(orNothing(d.Note))))
		}
	}
	if len(rows) > 0 {
		b.WriteString(`<p>Kept for diagnosis:</p><ul>` + strings.Join(rows, "") + `</ul>`)
	}
	return b.String()
}

// orNothing is s, or a dash when blank.
func orNothing(s string) string {
	if s == "" {
		return "—"
	}
	return s
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
	key := fmt.Sprintf("%s/%d", p.Batch, p.Scan)
	c.HTML(fmt.Sprintf(`<h1 class="title">Photo %s</h1>
<p class="subtitle">%s — <a href="batches/%s">batch %s</a>, <a href="%s">scan %d</a></p>`,
		id, html.EscapeString(p.Description), html.EscapeString(p.Batch), html.EscapeString(p.Batch), scanURL(key), p.Scan))

	fitW := min(img.W, FitWidth)
	fitH := img.H * fitW / img.W
	c.HTML(fmt.Sprintf(`<div class="columns"><div class="column is-two-thirds">
<form action="photos/%s" method="get">
<input type="image" name="at" src="photos/%s/fit.jpg" width="%d" height="%d" alt="The master" style="max-width:100%%;height:auto">
</form>
<p class="help">Master, %d ppi, %d × %d px, shown scaled to fit. Click it to see master pixels at 1:1.</p>`, id, id, fitW, fitH, s.MasterPPI, img.W, img.H))
	if p.Caption != "" {
		c.HTML(fmt.Sprintf(`<p class="photo-caption title is-4 mt-3 mb-1">%s</p><p class="subtitle is-6">%s</p>`, html.EscapeString(p.Caption), html.EscapeString(p.Description)))
	}
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
	if p.BackBlank {
		c.HTML(`<h2 class="title is-5 mt-5">Back</h2><p class="help">The back is blank: not kept.</p>`)
	}
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

	// while the scan waits at the quality check, the margins can be moved
	if l, err := batch.OpenScan(a.Staging, key); err == nil && a.running() != l.Key() {
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
	t.WriteString(`</tbody></table>`)
	c.HTML(t.String())
	c.HTML(a.describeSection(id, p))
	c.HTML(a.archiveSection(id))
	c.HTML(`</div></div>`)
	return c.Buffer()
}

// archiveSection is where the photo stands with the archive.
func (a *App) archiveSection(id pid.ID) string {
	d, err := batch.DepositOf(a.Staging, id)
	switch {
	case err != nil:
		return `<h2 class="title is-6 mt-4">Archive</h2><p class="has-text-danger">` + html.EscapeString(err.Error()) + `</p>`
	case d == nil:
		return `<h2 class="title is-6 mt-4">Archive</h2><p class="help">Not yet deposited.</p>`
	case d.Status == "duplicate":
		return fmt.Sprintf(`<h2 class="title is-6 mt-4">Archive</h2><p class="has-text-danger">Rejected by the archive: %s is a duplicate of item <code>%s</code> (home %s). The photo ends here; its bag stays in staging.</p>`,
			html.EscapeString(d.File), d.DuplicateOf, d.Home)
	}
	return fmt.Sprintf(`<h2 class="title is-6 mt-4">Archive</h2><p>Held by the archive since %s: item <code>%s</code>, home %s, %d files, checksums matching.</p>`,
		d.At.Local().Format("2 Jan 2006 15:04"), id, d.Home, len(d.Payload))
}

// describeSection is what is known of the photo: a form until it is
// archived, then the record.
func (a *App) describeSection(id pid.ID, p batch.Photo) string {
	claim := func(kind string) *batch.Claim {
		for i := range p.Claims {
			if p.Claims[i].Kind == kind {
				return &p.Claims[i]
			}
		}
		return nil
	}
	value := func(kind string) string {
		if c := claim(kind); c != nil {
			return html.EscapeString(c.Value)
		}
		return ""
	}
	var b strings.Builder
	b.WriteString(`<h2 class="title is-6 mt-4">What is known</h2>`)
	if bag.Complete(filepath.Join(a.Staging, "bags"), id) {
		if len(p.Claims) == 0 {
			b.WriteString(`<p class="help">Nothing known when the photo was described.</p>`)
		}
		b.WriteString(`<table class="table is-narrow"><tbody>`)
		for _, c := range p.Claims {
			note := ""
			if c.Approximate {
				note = " (approximate)"
			}
			fmt.Fprintf(&b, `<tr><th>%s</th><td>%s%s</td><td class="has-text-grey">%s, %s</td></tr>`,
				html.EscapeString(c.Kind), html.EscapeString(c.Value), note, html.EscapeString(c.Source), html.EscapeString(c.Read))
		}
		b.WriteString(`</tbody></table>`)
		return b.String()
	}
	approx, source := "", "back"
	if c := claim("date taken"); c != nil && c.Approximate {
		approx = " checked"
	}
	if len(p.Claims) > 0 {
		source = p.Claims[0].Source
	} else if p.Back == nil {
		source = "family"
	}
	var sources strings.Builder
	for _, s := range []string{"back", "front", "family"} {
		sel := ""
		if s == source {
			sel = " selected"
		}
		sources.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, s, sel, map[string]string{"back": "the back of the print", "front": "the front of the print", "family": "a relative's word"}[s]))
	}
	backField := ""
	if p.Back != nil {
		backField = fmt.Sprintf(`<div class="field"><label class="label">What the back says</label><div class="control"><textarea class="textarea is-small" name="back" rows="3">%s</textarea></div></div>`, value("back"))
	}
	fmt.Fprintf(&b, `<form action="photos/%s/describe" method="post" class="box">
<div class="field"><label class="label">Caption</label><div class="control"><input class="input is-small" name="caption" value="%s" placeholder="What the picture shows, in your own words"></div></div>
<div class="field"><label class="label">Date taken</label><div class="control"><input class="input is-small" name="date" value="%s" placeholder="1952, 1952-07 or 1952-07-14"></div>
<label class="checkbox"><input type="checkbox" name="approximate"%s> approximate</label></div>
<div class="field"><label class="label">Photographer</label><div class="control"><input class="input is-small" name="photographer" value="%s"></div></div>
%s<div class="field"><label class="label">Source</label><div class="control"><div class="select is-small"><select name="source">%s</select></div></div></div>
<div class="buttons"><button class="button is-primary is-small" type="submit">Save</button> <button class="button is-small" type="submit" name="skip" value="1">Nothing known</button></div>
</form>`, id, html.EscapeString(p.Caption), value("date taken"), approx, value("photographer"), backField, sources.String())
	if p.Described != nil {
		b.WriteString(`<p class="help">Described; can be changed until the photo is archived.</p>`)
	}
	return b.String()
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
