package web

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/film"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// newFilm is the form that imports a film into a batch: the one file,
// its title, transfer and poster asked once it is probed (story 116).
func newFilm(b *batch.Batch, msg string) string {
	return fmt.Sprintf(`<h1 class="title">Batch %s: import a film</h1>
<p class="subtitle">%s</p>
%s
<form action="batches/%s/films" method="post" enctype="multipart/form-data" class="box">
<div class="field"><label class="label">The film as received</label><div class="control"><input class="input" type="file" name="film" accept="%s" required></div>
<p class="help">One file, %s: kept exactly as sent, never re-encoded. Its home, provenance and credit are the batch's. What the file says of itself is read once it is here, and its title, its transfer and its poster asked then.</p></div>
<div class="field"><div class="control"><button class="button is-primary" type="submit">Import the film ›</button> <a class="button" href="batches/%s">Cancel</a></div></div>
</form>`, html.EscapeString(b.ID), html.EscapeString(b.Title), msg, html.EscapeString(b.ID),
		strings.Join(batch.FilmFormats, ","), strings.Join(batch.FilmFormats, ", "), html.EscapeString(b.ID))
}

// filmRoutes are the film import's pages and actions.
func (a *App) filmRoutes(mux *http.ServeMux, render func(http.ResponseWriter, string, string), fail func(http.ResponseWriter, error, int)) {
	open := func(w http.ResponseWriter, r *http.Request) (*batch.Batch, bool) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return nil, false
		}
		if b.Finished() {
			fail(w, fmt.Errorf("batch %s: scanning is finished", b.ID), http.StatusUnprocessableEntity)
			return nil, false
		}
		if !a.homed(w, b, render) {
			return nil, false
		}
		return b, true
	}
	mux.HandleFunc("GET /batches/{id}/films/new", func(w http.ResponseWriter, r *http.Request) {
		b, ok := open(w, r)
		if !ok {
			return
		}
		if err := film.Available(); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		render(w, "Import a film", newFilm(b, ""))
	})
	// the upload is read part by part and written to staging as it
	// arrives: a film is never held in memory
	mux.HandleFunc("POST /batches/{id}/films", func(w http.ResponseWriter, r *http.Request) {
		b, ok := open(w, r)
		if !ok {
			return
		}
		refuse := func(err error) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			render(w, "Import a film", newFilm(b, `<div class="notification is-danger">`+html.EscapeString(err.Error())+`</div>`))
		}
		mr, err := r.MultipartReader()
		if err != nil {
			refuse(errors.New("nothing to import: choose a file"))
			return
		}
		var l *batch.Scan
		for l == nil {
			part, err := mr.NextPart()
			if err != nil {
				refuse(errors.New("nothing to import: choose a file"))
				return
			}
			if part.FormName() != "film" || part.FileName() == "" {
				continue
			}
			if l, err = b.NewFilm(r.Context(), batch.FilmImport{File: part.FileName(), Data: part}, a.Now()); err != nil {
				refuse(err)
				return
			}
		}
		a.tick(l)
		http.Redirect(w, r, "/"+scanURL(l.Key()), http.StatusSeeOther)
	})
	// a film's steps need no scanner: each is prepared, then the import
	// ticked to its next wait
	act := func(name string, prepare func(r *http.Request, l *batch.Scan) error) {
		mux.HandleFunc("POST /batches/{id}/scans/{n}/"+name, func(w http.ResponseWriter, r *http.Request) {
			l, err := batch.OpenScan(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
			if err != nil || !l.IsFilm() {
				http.NotFound(w, r)
				return
			}
			if err := prepare(r, l); err != nil {
				fail(w, err, http.StatusUnprocessableEntity)
				return
			}
			a.tick(l)
			http.Redirect(w, r, "/"+scanURL(l.Key()), http.StatusSeeOther)
		})
	}
	act("film-confirm", func(r *http.Request, l *batch.Scan) error {
		return l.ConfirmFilm(r.Context(), batch.FilmDetails{Title: r.FormValue("title"),
			Transfer: batch.Transfer{By: r.FormValue("by"), Date: r.FormValue("date"), From: r.FormValue("from"), Note: r.FormValue("note")},
			Poster:   posterOf(r)})
	})
	act("film-describe", func(r *http.Request, l *batch.Scan) error {
		d := batch.FilmDescription{Made: batch.MadeClaim{Value: r.FormValue("made"), Approximate: r.FormValue("approximate") != "", Source: r.FormValue("source")}}
		for i := 1; r.Form.Has(fmt.Sprintf("role-%d", i)) || r.Form.Has(fmt.Sprintf("person-%d", i)); i++ {
			d.People = append(d.People, batch.RoleChoice{Person: r.FormValue(fmt.Sprintf("person-%d", i)), Role: r.FormValue(fmt.Sprintf("role-%d", i))})
		}
		if r.Form.Has("poster") {
			p := posterOf(r)
			d.Poster = &p
		}
		return l.DescribeFilm(r.Context(), a.Archive, d, a.Now())
	})
	mux.HandleFunc("GET /batches/{id}/scans/{n}/poster.jpg", func(w http.ResponseWriter, r *http.Request) {
		l, err := batch.OpenScan(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
		if err != nil || !l.IsFilm() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store") // remade when the poster is changed
		http.ServeFile(w, r, l.PosterPreview())
	})
	film := func(w http.ResponseWriter, r *http.Request) (pid.ID, batch.FilmRecord, *batch.Scan, bool) {
		id, err := pid.Parse(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return 0, batch.FilmRecord{}, nil, false
		}
		rec, _, err := batch.FindFilm(a.Staging, id)
		if err != nil {
			http.NotFound(w, r)
			return 0, rec, nil, false
		}
		l, err := batch.OpenScan(a.Staging, rec.Batch+"/"+strconv.Itoa(rec.Film))
		if err != nil {
			http.NotFound(w, r)
			return 0, rec, nil, false
		}
		return id, rec, l, true
	}
	mux.HandleFunc("GET /films/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, rec, l, ok := film(w, r); ok {
			render(w, "Film "+id.String(), a.filmPage(r, id, rec, l))
		}
	})
	// a film's retry ticks its import, which runs the film as far as it goes
	mux.HandleFunc("POST /films/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		if id, _, l, ok := film(w, r); ok {
			a.tick(l)
			http.Redirect(w, r, "/films/"+id.String(), http.StatusSeeOther)
		}
	})
	mux.HandleFunc("GET /films/{id}/renditions/{version}/{file}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pid.Parse(r.PathValue("id"))
		version, verr := strconv.Atoi(r.PathValue("version"))
		file := r.PathValue("file")
		if err != nil || verr != nil || version < 1 || version > rendition.Version || (file != "display.jpg" && file != "thumb.jpg" && file != "tiny.jpg" && file != "play.mp4") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, filepath.Join(a.Staging, "renditions", id.String(), strconv.Itoa(version), file))
	})
}

// posterOf is the poster as the form gives it: the picture attached, or
// the frame at the time typed.
func posterOf(r *http.Request) batch.Poster {
	if r.FormValue("poster") == "attached" {
		return batch.Poster{Attached: true}
	}
	return batch.Poster{Frame: r.FormValue("frame")}
}

// fileLine is what was read off the file in a line: "00:15:36, h264 and
// aac in mp4, 25 fps, 1.1 GB".
func fileLine(duration, container, video, audio string, rate float64, size int64) string {
	codecs := video
	if audio != "" {
		codecs += " and " + audio
	}
	return fmt.Sprintf("%s, %s in %s, %s fps, %s", duration, html.EscapeString(codecs), html.EscapeString(container),
		strconv.FormatFloat(rate, 'f', -1, 64), bytesText(size))
}

// bytesText is a size as the operator reads it.
func bytesText(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d kB", (n+1023)>>10)
}

func chapterList(chapters []film.Chapter) string {
	if len(chapters) == 0 {
		return "none"
	}
	var parts []string
	for _, c := range chapters {
		parts = append(parts, fmt.Sprintf("%s (%s)", html.EscapeString(c.Title), c.Start))
	}
	return strings.Join(parts, ", ")
}

// posterText says the poster as chosen.
func posterText(p batch.Poster) string {
	if p.Attached {
		return "the picture attached in the file"
	}
	return "the frame at " + html.EscapeString(p.Frame)
}

// posterFields are the poster's part of a form: the picture attached,
// offered when the file has one, or a frame at a time.
func posterFields(f *batch.Filmed) string {
	frame := fmt.Sprintf(`<input class="input" name="frame" value="%s" placeholder="00:01:23" style="width:9em">`, html.EscapeString(f.Poster.Frame))
	help := fmt.Sprintf(`The still that stands for the film on a page; its display, thumb and tiny pictures are made from it. A frame's time is hh:mm:ss within the film, which runs %s.`, f.Probe.Duration)
	if len(f.Probe.Attached) == 0 {
		return `<div class="field"><label class="label">Poster: a frame at</label><div class="control"><input type="hidden" name="poster" value="frame">` + frame +
			`</div><p class="help">The file has no picture attached. ` + help + `</p></div>`
	}
	attached, at := " checked", ""
	if !f.Poster.Attached {
		attached, at = "", " checked"
	}
	return fmt.Sprintf(`<div class="field"><label class="label">Poster</label><div class="control">
<label class="radio"><input type="radio" name="poster" value="attached"%s> the picture attached in the file</label>
<label class="radio"><input type="radio" name="poster" value="frame"%s> a frame at</label> %s</div><p class="help">%s</p></div>`, attached, at, frame, help)
}

// filmImportPage is a film import's details, what was read off the file,
// where its workflow stands and the form the operator is to fill there:
// the film's title, transfer and poster, then who it names and when it
// was made.
func (a *App) filmImportPage(r *http.Request, l *batch.Scan) string {
	ctx := r.Context()
	c := lofigui.NewContext()
	busy, waiting := a.state(r, c, l)
	b, f := l.Sitting(), l.Film
	p := f.Probe
	attached := "no picture attached"
	if n := len(p.Attached); n > 0 {
		attached = fmt.Sprintf("%d %s attached", n, plural(n, "picture", "pictures"))
	}
	confirmed := ""
	if f.Confirmed != nil {
		confirmed = fmt.Sprintf(`<tr><th>Title</th><td>%s</td></tr><tr><th>Transfer</th><td>%s</td></tr><tr><th>Poster</th><td>%s</td></tr>`,
			html.EscapeString(f.Title), html.EscapeString(transferText(f.Transfer)), posterText(f.Poster))
	}
	c.HTML(fmt.Sprintf(`<h1 class="title">Film %d of <a href="batches/%s">batch %s</a></h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Batch</th><td>%s</td></tr>
<tr><th>Home</th><td>%s</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
<tr><th>File</th><td><code>%s</code>: %s; %s</td></tr>
<tr><th>Chapters</th><td>%s</td></tr>
%s
<tr><th>Imported</th><td>%s</td></tr>
</tbody></table>`, l.N, html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(l.Description), html.EscapeString(b.Title),
		homeText(b.PersonName, b.Person), orUnknown(b.Provenance), html.EscapeString(b.Credit),
		html.EscapeString(f.File), fileLine(p.Duration, p.Container, p.Video, p.Audio, p.FrameRate, p.Size), attached,
		chapterList(p.Chapters), confirmed, l.Created.Local().Format("2 Jan 2006 15:04")))
	if dots := a.runDots(ctx, batch.FilmImportWorkflow, l.Key()); dots != "" {
		c.HTML(`<p>` + dots + ` ` + a.scanDots(ctx, l) + `</p>`)
	}
	switch {
	case busy:
	case waiting == "confirm":
		c.HTML(fmt.Sprintf(`<form action="%s/film-confirm" method="post" class="box">
<div class="field"><label class="label">Title</label><div class="control"><input class="input" name="title" value="%s" required></div>
<p class="help">Pre-filled from the file's own title, when it has one.</p></div>
<div class="field"><label class="label">Transfer: who digitised the film, and when</label>
<div class="field is-grouped"><div class="control is-expanded"><input class="input" name="by" value="%s" placeholder="who made the transfer" required></div>
<div class="control"><input class="input" name="date" value="%s" placeholder="2016-03-07" required></div></div>
<p class="help">The transfer is the film's provenance, as a page's sender is. A transfer's file is dated and signed by the transfer, so both are pre-filled from the file where it says; the date is an ISO 8601 prefix, 2016, 2016-03 or 2016-03-07. Neither says when the film was made: that is asked next.</p></div>
<div class="field"><label class="label">From: the medium it was transferred from</label><div class="control"><input class="input" name="from" value="%s" placeholder="16 mm cine, VHS, …"></div></div>
<div class="field"><label class="label">Note</label><div class="control"><input class="input" name="note" value="%s"></div></div>
%s
<button class="button is-primary" type="submit">Confirm the film ›</button>
</form>`, scanURL(l.Key()), html.EscapeString(f.Title), html.EscapeString(f.Transfer.By), html.EscapeString(f.Transfer.Date),
			html.EscapeString(f.Transfer.From), html.EscapeString(f.Transfer.Note), posterFields(f)))
	case waiting == "describe":
		c.HTML(a.filmDescribeForm(l))
	case f.ID != nil:
		c.HTML(fmt.Sprintf(`<p>Film <a href="films/%s">%s</a> %s</p>`, f.ID, f.ID, a.filmThumbnails([]pid.ID{*f.ID})))
	}
	ids, _ := l.Items()
	c.HTML(a.stepsHeading(ctx, batch.FilmImportWorkflow, l.Key()) +
		a.parentLine(ctx, "batches/"+html.EscapeString(l.Batch), "batch "+l.Batch, batch.Workflow, l.Batch) +
		a.stepsTable(ctx, batch.FilmImportWorkflow, l.Key(), feedAction(l)) +
		a.itemRows(ctx, ids, batch.FilmWorkflow, "films", "film"))
	return c.Buffer()
}

// filmDescribeForm asks who the film names and in what part, and when it
// was made, beside the poster as chosen. The names the file gives are
// shown as written with their part chosen and no person: the operator
// picks who each is.
func (a *App) filmDescribeForm(l *batch.Scan) string {
	f := l.Film
	people, _ := a.Archive.People()
	type row struct{ role, person, hint string }
	var rows []row
	for _, r := range f.People {
		rows = append(rows, row{role: r.Role, person: r.Person.String()})
	}
	if len(rows) == 0 {
		for _, name := range f.Probe.Tags.Directors {
			rows = append(rows, row{role: "director", hint: name})
		}
		for _, name := range f.Probe.Tags.Producers {
			rows = append(rows, row{role: "producer", hint: name})
		}
	}
	for range 3 {
		rows = append(rows, row{})
	}
	var t strings.Builder
	for i, r := range rows {
		var roles strings.Builder
		roles.WriteString(`<option value="">—</option>`)
		for _, role := range batch.FilmRoles {
			sel := ""
			if role == r.role {
				sel = " selected"
			}
			fmt.Fprintf(&roles, `<option value="%s"%s>%s</option>`, role, sel, role)
		}
		hint := ""
		if r.hint != "" {
			hint = fmt.Sprintf(`<p class="help">The file names %s as %s: choose who that is, or clear the part.</p>`, html.EscapeString(r.hint), r.role)
		}
		fmt.Fprintf(&t, `<div class="field is-grouped"><div class="control"><div class="select"><select name="role-%d">%s</select></div></div>
<div class="control"><div class="select"><select name="person-%d">%s</select></div></div></div>%s`, i+1, roles.String(), i+1, personOptions(people, r.person), hint)
	}
	made := batch.Made{Source: "family"}
	if f.Made != nil {
		made = *f.Made
	}
	approx := ""
	if made.Approximate {
		approx = " checked"
	}
	var sources strings.Builder
	for _, s := range batch.MadeSources {
		sel := ""
		if s == made.Source {
			sel = " selected"
		}
		fmt.Fprintf(&sources, `<option value="%s"%s>%s</option>`, s, sel, map[string]string{"family": "a relative's word", "film": "the film itself: its titles, what it says"}[s])
	}
	return fmt.Sprintf(`<form action="%s/film-describe" method="post" class="box">
<figure class="mb-4"><img src="%s/poster.jpg?%s" alt="The poster" style="max-height:320px;border:1px solid #ddd"><figcaption class="help">The poster: %s.</figcaption></figure>
<div class="field"><label class="label">Who the film names, and in what part</label>%s
<p class="help">Each a person of the register. A film homed with someone is not thereby their work: the part says so. Leave every part blank when no one is known.</p></div>
<div class="field"><label class="label">Made</label>
<div class="field is-grouped"><div class="control"><input class="input" name="made" value="%s" placeholder="1970"></div>
<div class="control"><label class="checkbox"><input type="checkbox" name="approximate"%s> approximate</label></div>
<div class="control"><div class="select"><select name="source">%s</select></div></div></div>
<p class="help">When the film was made, as an ISO 8601 prefix, 1970, 1970-06 or 1970-06-14, and where that comes from. The transfer's date is not it. Leave it blank when it is not known.</p></div>
%s
<button class="button is-primary" type="submit">Describe and archive ›</button>
</form>`, scanURL(l.Key()), scanURL(l.Key()), posterKey(f.Poster), posterText(f.Poster), t.String(),
		html.EscapeString(made.Value), approx, sources.String(), posterFields(f))
}

// posterKey is a query that changes with the poster, so the browser fetches
// the picture again.
func posterKey(p batch.Poster) string {
	if p.Attached {
		return "attached"
	}
	return strings.ReplaceAll(p.Frame, ":", "")
}

// transferText is the transfer in a line: "Alive Studios, 2016-03-07,
// from 16 mm cine; the note".
func transferText(t batch.Transfer) string {
	said := t.By + ", " + t.Date
	if t.From != "" {
		said += ", from " + t.From
	}
	if t.Note != "" {
		said += "; " + t.Note
	}
	return said
}

// filmThumbnails are the tiny renditions of each film's poster, linking
// to the film.
func (a *App) filmThumbnails(ids []pid.ID) string {
	var t strings.Builder
	for _, id := range ids {
		dir := a.renditionDir(id)
		if _, err := os.Stat(filepath.Join(dir, "tiny.jpg")); err != nil {
			fmt.Fprintf(&t, `<a href="films/%s">film %s</a>`, id, id)
			continue
		}
		fmt.Fprintf(&t, `<a href="films/%s"><img src="films/%s/renditions/%s/tiny.jpg" alt="film %s" title="film %s" style="height:48px;margin-right:4px"></a>`,
			id, id, filepath.Base(dir), id, id)
	}
	return t.String()
}

// filmPage is a film's details: its poster and the play copy, the
// record, the archive's answer and its steps.
func (a *App) filmPage(r *http.Request, id pid.ID, rec batch.FilmRecord, l *batch.Scan) string {
	ctx := r.Context()
	c := lofigui.NewContext()
	people := "No one named"
	if len(rec.People) > 0 {
		var parts []string
		for _, p := range rec.People {
			name, err := a.Archive.Person(p.Person)
			if err != nil {
				name = "person " + p.Person.String()
			}
			parts = append(parts, fmt.Sprintf("%s (%s), %s", html.EscapeString(name), p.Person, html.EscapeString(p.Role)))
		}
		people = strings.Join(parts, "; ")
	}
	made := "not known"
	if m := rec.Made; m != nil {
		made = html.EscapeString(m.Value)
		if m.Approximate {
			made = "about " + made
		}
		made += ", from " + html.EscapeString(m.Source)
	}
	f := rec.File
	c.HTML(fmt.Sprintf(`<h1 class="title">Film %s</h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Batch</th><td><a href="batches/%s">%s</a></td></tr>
<tr><th>Import</th><td><a href="%s">Film %d</a></td></tr>
<tr><th>Home</th><td>%s</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
<tr><th>Transfer</th><td>%s</td></tr>
<tr><th>People</th><td>%s</td></tr>
<tr><th>Made</th><td>%s</td></tr>
<tr><th>File</th><td><code>%s</code>: %s</td></tr>
<tr><th>Chapters</th><td>%s</td></tr>
<tr><th>Poster</th><td>%s</td></tr>
</tbody></table>`, id, html.EscapeString(rec.Title), html.EscapeString(rec.Batch), html.EscapeString(rec.Batch),
		scanURL(l.Key()), rec.Film, a.homeNow(id, rec.Home), orUnknown(rec.Provenance), html.EscapeString(rec.Credit),
		html.EscapeString(transferText(rec.Transfer)), people, made,
		html.EscapeString(f.Name), fileLine(f.Duration, f.Container, f.Video, f.Audio, f.FrameRate, f.Size), chapterList(f.Chapters), posterText(rec.Poster)))
	dir := a.renditionDir(id)
	v := filepath.Base(dir)
	if _, err := os.Stat(filepath.Join(dir, "play.mp4")); err == nil {
		c.HTML(fmt.Sprintf(`<figure class="mt-4"><video controls preload="metadata" poster="films/%s/renditions/%s/display.jpg" src="films/%s/renditions/%s/play.mp4" style="max-width:100%%;border:1px solid #ddd"></video>
<figcaption class="help">The play copy, <code>play.mp4</code>, with the poster; the file as received is in the bag, unaltered.</figcaption></figure>`, id, v, id, v))
	} else {
		c.HTML(`<p class="help">The poster's stills and the play copy are not made yet.</p>`)
	}
	c.HTML(a.itemArchive(ctx, id, batch.FilmWorkflow, "films/"+id.String()+"/retry", "film"))
	c.HTML(a.filmSteps(ctx, id, l.Key()))
	return c.Buffer()
}

// filmSteps is the film page's steps section: its definition, its import
// and its steps.
func (a *App) filmSteps(ctx context.Context, id pid.ID, key string) string {
	return a.stepsHeading(ctx, batch.FilmWorkflow, id.String()) +
		a.parentLine(ctx, scanURL(key), "film import "+key, batch.FilmImportWorkflow, key) +
		a.stepsTable(ctx, batch.FilmWorkflow, id.String(), itemAction("films", id))
}
