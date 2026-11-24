package web

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	lw "git.bytestone.uk/hum3/life-weaver/writing"
	"git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/writing"
)

// newWriting is the form that imports a writing into a batch: the one
// file and its title (story 190).
func newWriting(b *batch.Batch, msg string) string {
	return fmt.Sprintf(`<h1 class="title">Batch %s: import a writing</h1>
<p class="subtitle">%s</p>
%s
<form action="batches/%s/writings" method="post" enctype="multipart/form-data" class="box">
<div class="field"><label class="label">The writing as received</label><div class="control"><input class="input" type="file" name="writing" accept="%s" required></div>
<p class="help">One file, a Word document or plain text (%s). Its words are taken out into djot, which is what the archive keeps: headings, paragraphs, bold and italic, lists, line breaks, links to the web and the author's notes. Pictures, tables and embedded objects are counted and left out, and the file itself is let go once the writing is bagged, its name, size and sha256 kept. A document that carries macros is refused. Its home, provenance and credit are the batch's.</p></div>
<div class="field"><label class="label">Title</label><div class="control"><input class="input" name="title" placeholder="the file's name, when left blank"></div></div>
<div class="field"><div class="control"><button class="button is-primary" type="submit">Import the writing ›</button> <a class="button" href="batches/%s">Cancel</a></div></div>
</form>`, html.EscapeString(b.ID), html.EscapeString(b.Title), msg, html.EscapeString(b.ID),
		strings.Join(writing.Formats, ","), strings.Join(writing.Formats, ", "), html.EscapeString(b.ID))
}

// maxWriting is the most of an upload a writing import reads: a document
// with its pictures, which are not kept.
const maxWriting = 1 << 30

// writingRoutes are the writing import's pages and actions.
func (a *App) writingRoutes(mux *http.ServeMux, render func(http.ResponseWriter, string, string), fail func(http.ResponseWriter, error, int)) {
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
	mux.HandleFunc("GET /batches/{id}/writings/new", func(w http.ResponseWriter, r *http.Request) {
		if b, ok := open(w, r); ok {
			render(w, "Import a writing", newWriting(b, ""))
		}
	})
	// the upload is read part by part and written to staging as it
	// arrives; the title comes before the file, as the form has them
	mux.HandleFunc("POST /batches/{id}/writings", func(w http.ResponseWriter, r *http.Request) {
		b, ok := open(w, r)
		if !ok {
			return
		}
		refuse := func(err error) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			render(w, "Import a writing", newWriting(b, `<div class="notification is-danger">`+html.EscapeString(err.Error())+`</div>`))
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxWriting)
		parts, err := r.MultipartReader()
		if err != nil {
			refuse(errors.New("nothing to import: choose a file"))
			return
		}
		var l *batch.Scan
		title := ""
		for {
			part, err := parts.NextPart()
			if err != nil {
				break
			}
			switch {
			case part.FormName() == "title":
				said, _ := io.ReadAll(io.LimitReader(part, 512))
				title = string(said)
			case part.FormName() == "writing" && part.FileName() != "" && l == nil:
				if l, err = b.NewWriting(batch.WritingImport{File: part.FileName(), Title: title, Data: part}, a.Now()); err != nil {
					refuse(err)
					return
				}
			}
		}
		if l == nil {
			refuse(errors.New("nothing to import: choose a file"))
			return
		}
		a.tick(l)
		http.Redirect(w, r, "/"+scanURL(l.Key()), http.StatusSeeOther)
	})
	// a writing's steps need no scanner: each is prepared, then the import
	// ticked to its next wait
	act := func(name string, prepare func(r *http.Request, l *batch.Scan) error) {
		mux.HandleFunc("POST /batches/{id}/scans/{n}/"+name, func(w http.ResponseWriter, r *http.Request) {
			l, err := batch.OpenScan(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
			if err != nil || !l.IsWriting() {
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
	act("writing-read", func(r *http.Request, l *batch.Scan) error { return l.ConfirmWriting(r.FormValue("title")) })
	act("writing-describe", func(r *http.Request, l *batch.Scan) error {
		if err := r.ParseForm(); err != nil {
			return err
		}
		d := batch.WritingDescription{Written: batch.MadeClaim{Value: r.FormValue("written"), Approximate: r.FormValue("approximate") != "", Source: r.FormValue("source")}}
		for i := 1; r.Form.Has(fmt.Sprintf("role-%d", i)) || r.Form.Has(fmt.Sprintf("person-%d", i)); i++ {
			d.People = append(d.People, batch.RoleChoice{Person: r.FormValue(fmt.Sprintf("person-%d", i)), Role: r.FormValue(fmt.Sprintf("role-%d", i))})
		}
		// the hands in the order of their rows, as the operator left them
		for i := 1; r.Form.Has(fmt.Sprintf("hand-part-%d", i)); i++ {
			field := func(name string) string { return r.FormValue(fmt.Sprintf("hand-%s-%d", name, i)) }
			d.Transmission = append(d.Transmission, batch.HandChoice{Part: field("part"), Person: field("person"), Name: field("name"), Date: field("date"), Note: field("note")})
		}
		return l.DescribeWriting(a.Archive, d, a.Now())
	})
	found := func(w http.ResponseWriter, r *http.Request) (pid.ID, batch.WritingRecord, []byte, *batch.Scan, bool) {
		id, err := pid.Parse(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return 0, batch.WritingRecord{}, nil, nil, false
		}
		rec, text, err := batch.FindWriting(a.Staging, id)
		if err != nil {
			http.NotFound(w, r)
			return 0, rec, nil, nil, false
		}
		l, err := batch.OpenScan(a.Staging, rec.Batch+"/"+strconv.Itoa(rec.Writing))
		if err != nil {
			http.NotFound(w, r)
			return 0, rec, nil, nil, false
		}
		return id, rec, text, l, true
	}
	mux.HandleFunc("GET /writings/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, rec, text, l, ok := found(w, r); ok {
			render(w, "Writing "+id.String(), a.writingPage(r, id, rec, text, l))
		}
	})
	// a writing's retry ticks its import, which runs the writing as far as it goes
	mux.HandleFunc("POST /writings/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		if id, _, _, l, ok := found(w, r); ok {
			a.tick(l)
			http.Redirect(w, r, "/writings/"+id.String(), http.StatusSeeOther)
		}
	})
}

// writingLinks link each writing by its id: it has no picture to show.
func writingLinks(ids []pid.ID) string {
	var t strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&t, `<a href="writings/%s">writing %s</a> `, id, id)
	}
	return t.String()
}

// receivedLine is the file received in a line: its name, format and
// size, and what was left out of it.
func receivedLine(f batch.ReceivedFile) string {
	left := f.LeftOut.String()
	if left == "" {
		left = "Nothing left out"
	}
	return fmt.Sprintf(`<code>%s</code>: %s, %s. %s.`, html.EscapeString(f.Name), html.EscapeString(f.Format), bytesText(f.Size), left)
}

// readPage is the djot as the site will show it, life-weaver's own
// rendering: nothing raw, no picture and no link but to the web,
// whatever the djot holds. Under it, the count of words and notes.
func readPage(djot []byte) string {
	page, err := lw.Render(djot)
	if err != nil {
		return danger(err)
	}
	words, notes := writing.Count(djot)
	said := "no notes"
	if notes > 0 {
		said = fmt.Sprintf("%d %s", notes, plural(notes, "note", "notes"))
	}
	return fmt.Sprintf(`<article class="box content">%s</article>
<p class="help">%d %s, %s.</p>`, page, words, plural(words, "word", "words"), said)
}

// writingImportPage is a writing import's details, where its workflow
// stands and what the operator is to do there: read the page and go on
// under its title, then say who it names, when it was written and the
// hands it passed through.
func (a *App) writingImportPage(r *http.Request, l *batch.Scan) string {
	ctx := r.Context()
	c := lofigui.NewContext()
	busy, waiting := a.state(r, c, l)
	b, w := l.Sitting(), l.Writing
	c.HTML(fmt.Sprintf(`<h1 class="title">Writing %d of <a href="batches/%s">batch %s</a></h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Batch</th><td>%s</td></tr>
<tr><th>Home</th><td>%s</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
<tr><th>Received</th><td>%s</td></tr>
<tr><th>Imported</th><td>%s</td></tr>
</tbody></table>`, l.N, html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(l.Description), html.EscapeString(b.Title),
		homeText(b.PersonName, b.Person), orUnknown(b.Provenance), html.EscapeString(b.Credit),
		receivedLine(w.Received), l.Created.Local().Format("2 Jan 2006 15:04")))
	if dots := a.runDots(ctx, batch.WritingImportWorkflow, l.Key()); dots != "" {
		c.HTML(`<p>` + dots + ` ` + a.scanDots(ctx, l) + `</p>`)
	}
	switch {
	case busy:
	case waiting == "read":
		text, err := l.WritingText()
		if err != nil {
			c.HTML(danger(err))
			break
		}
		c.HTML(readPage(text) + fmt.Sprintf(`<form action="%s/writing-read" method="post" class="box">
<div class="field"><label class="label">Title</label><div class="control"><input class="input" name="title" value="%s" required></div>
<p class="help">The page above is the writing as the site will show it; the text is not edited here. If the wrong file came, or not all of it, cancel the import.</p></div>
<button class="button is-primary" type="submit">Go on ›</button>
</form>`, scanURL(l.Key()), html.EscapeString(w.Title)))
	case waiting == "describe":
		c.HTML(a.writingDescribeForm(l))
	case w.ID != nil:
		c.HTML(fmt.Sprintf(`<p>%s</p>`, writingLinks([]pid.ID{*w.ID})))
	}
	ids, _ := l.Items()
	c.HTML(a.stepsHeading(ctx, batch.WritingImportWorkflow, l.Key()) +
		a.parentLine(ctx, "batches/"+html.EscapeString(l.Batch), "batch "+l.Batch, batch.Workflow, l.Batch) +
		a.stepsTable(ctx, batch.WritingImportWorkflow, l.Key(), feedAction(l)) +
		a.itemRows(ctx, ids, batch.WritingWorkflow, "writings", "writing"))
	return c.Buffer()
}

// options are a select's options, the one chosen selected; the first is
// none.
func options(values []string, chosen string) string {
	var t strings.Builder
	t.WriteString(`<option value="">—</option>`)
	for _, v := range values {
		sel := ""
		if v == chosen {
			sel = " selected"
		}
		fmt.Fprintf(&t, `<option value="%s"%s>%s</option>`, v, sel, v)
	}
	return t.String()
}

// handsScript adds, removes and reorders the transmission's rows, and
// numbers their fields again as they then stand, so that the order sent
// is the order seen. Without it the rows are sent as shown.
const handsScript = `<script>
(function () {
	var hands = document.getElementById('hands');
	function number() {
		Array.prototype.forEach.call(hands.querySelectorAll('.hand'), function (row, i) {
			row.querySelectorAll('[name]').forEach(function (f) { f.name = f.name.replace(/-\d+$/, '-' + (i + 1)); });
		});
	}
	hands.addEventListener('click', function (e) {
		var row = e.target.closest('.hand');
		if (!row || !e.target.dataset.hand) return;
		switch (e.target.dataset.hand) {
		case 'up': if (row.previousElementSibling) hands.insertBefore(row, row.previousElementSibling); break;
		case 'down': if (row.nextElementSibling) hands.insertBefore(row.nextElementSibling, row); break;
		case 'remove': if (hands.querySelectorAll('.hand').length > 1) row.remove(); else row.querySelectorAll('input, select').forEach(function (f) { f.value = ''; }); break;
		}
		number();
	});
	document.getElementById('add-hand').addEventListener('click', function () {
		var row = hands.querySelector('.hand').cloneNode(true);
		row.querySelectorAll('input, select').forEach(function (f) { f.value = ''; });
		hands.appendChild(row);
		number();
	});
})();
</script>`

// writingDescribeForm asks who the writing names and in what part, when
// it was written, and its transmission: a row a hand, its part, a person
// picked or a name typed, a date and a note; rows added, removed and
// reordered.
func (a *App) writingDescribeForm(l *batch.Scan) string {
	w := l.Writing
	people, _ := a.Archive.People()
	roles := append([]batch.Role(nil), w.People...)
	for range 3 {
		roles = append(roles, batch.Role{})
	}
	var named strings.Builder
	for i, r := range roles {
		who := ""
		if r.Role != "" {
			who = r.Person.String()
		}
		fmt.Fprintf(&named, `<div class="field is-grouped"><div class="control"><div class="select"><select name="role-%d">%s</select></div></div>
<div class="control"><div class="select"><select name="person-%d">%s</select></div></div></div>`, i+1, options(batch.WritingRoles, r.Role), i+1, personOptions(people, who))
	}
	hands := append([]batch.Hand(nil), w.Transmission...)
	for range 2 {
		hands = append(hands, batch.Hand{})
	}
	var rows strings.Builder
	for i, h := range hands {
		who := ""
		if h.Part != "" && h.By.Name == "" {
			who = h.By.Person.String()
		}
		fmt.Fprintf(&rows, `<div class="hand field is-grouped is-grouped-multiline">
<div class="control"><div class="select"><select name="hand-part-%[1]d">%[2]s</select></div></div>
<div class="control"><div class="select"><select name="hand-person-%[1]d">%[3]s</select></div></div>
<div class="control"><input class="input" name="hand-name-%[1]d" value="%[4]s" placeholder="or a name as written"></div>
<div class="control"><input class="input" name="hand-date-%[1]d" value="%[5]s" placeholder="1998" style="width:8em"></div>
<div class="control is-expanded"><input class="input" name="hand-note-%[1]d" value="%[6]s" placeholder="note"></div>
<div class="control"><button class="button is-small" type="button" data-hand="up" title="Move this hand earlier">↑</button> <button class="button is-small" type="button" data-hand="down" title="Move this hand later">↓</button> <button class="button is-small" type="button" data-hand="remove" title="Remove this hand">✕</button></div>
</div>`, i+1, options(batch.TransmissionParts, h.Part), personOptions(people, who), html.EscapeString(h.By.Name), html.EscapeString(h.Date), html.EscapeString(h.Note))
	}
	written := batch.Made{Source: "family"}
	if w.Written != nil {
		written = *w.Written
	}
	approx := ""
	if written.Approximate {
		approx = " checked"
	}
	var sources strings.Builder
	for _, s := range batch.WrittenSources {
		sel := ""
		if s == written.Source {
			sel = " selected"
		}
		fmt.Fprintf(&sources, `<option value="%s"%s>%s</option>`, s, sel, map[string]string{"family": "a relative's word", "writing": "the writing itself: what it says"}[s])
	}
	return fmt.Sprintf(`<form action="%s/writing-describe" method="post" class="box">
<div class="field"><label class="label">Who the writing names, and in what part</label>%s
<p class="help">Each a person of the register: its author, and whom it is about. A writing homed with someone is not thereby their work: the part says so. Leave every part blank when no one is known.</p></div>
<div class="field"><label class="label">Written</label>
<div class="field is-grouped"><div class="control"><input class="input" name="written" value="%s" placeholder="1974"></div>
<div class="control"><label class="checkbox"><input type="checkbox" name="approximate"%s> approximate</label></div>
<div class="control"><div class="select"><select name="source">%s</select></div></div></div>
<p class="help">When it was written, as an ISO 8601 prefix, 1974, 1974-06 or 1974-06-14, and where that comes from. The file's own dates are the typing's, not it. Leave it blank when it is not known.</p></div>
<div class="field"><label class="label">Transmission: the hands it passed through to reach the archive, in order</label>
<div id="hands">%s</div>
<p class="help">A row a hand: its part, then who, a person picked from the register or a name typed as it is written, one or the other; a date and a note if known. A row with no part is left out.</p>
<button class="button is-small" type="button" id="add-hand">Add a hand</button></div>
<button class="button is-primary" type="submit">Describe and archive ›</button>
</form>%s`, scanURL(l.Key()), named.String(), html.EscapeString(written.Value), approx, sources.String(), rows.String(), handsScript)
}

// writingPage is a writing's details: its record, the archive's answer,
// its steps, and its text as the site will show it.
func (a *App) writingPage(r *http.Request, id pid.ID, rec batch.WritingRecord, text []byte, l *batch.Scan) string {
	ctx := r.Context()
	c := lofigui.NewContext()
	name := func(p pid.ID) string {
		n, err := a.Archive.Person(p)
		if err != nil {
			n = "person " + p.String()
		}
		return fmt.Sprintf("%s (%s)", html.EscapeString(n), p)
	}
	people := "No one named"
	if len(rec.People) > 0 {
		var parts []string
		for _, p := range rec.People {
			parts = append(parts, name(p.Person)+", "+html.EscapeString(p.Role))
		}
		people = strings.Join(parts, "; ")
	}
	written := "not known"
	if m := rec.Written; m != nil {
		written = html.EscapeString(m.Value)
		if m.Approximate {
			written = "about " + written
		}
		written += ", from " + html.EscapeString(m.Source)
	}
	hands := "not recorded"
	if len(rec.Transmission) > 0 {
		var parts []string
		for _, h := range rec.Transmission {
			said := html.EscapeString(h.Part) + " by "
			if h.By.Name != "" {
				said += html.EscapeString(h.By.Name)
			} else {
				said += name(h.By.Person)
			}
			if h.Date != "" {
				said += ", " + html.EscapeString(h.Date)
			}
			if h.Note != "" {
				said += ", " + html.EscapeString(h.Note)
			}
			parts = append(parts, said)
		}
		hands = strings.Join(parts, "; then ")
	}
	c.HTML(fmt.Sprintf(`<h1 class="title">Writing %s</h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Batch</th><td><a href="batches/%s">%s</a></td></tr>
<tr><th>Import</th><td><a href="%s">Writing %d</a></td></tr>
<tr><th>Home</th><td>%s</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
<tr><th>People</th><td>%s</td></tr>
<tr><th>Written</th><td>%s</td></tr>
<tr><th>Transmission</th><td>%s</td></tr>
<tr><th>Received</th><td>%s</td></tr>
</tbody></table>`, id, html.EscapeString(rec.Title), html.EscapeString(rec.Batch), html.EscapeString(rec.Batch),
		scanURL(l.Key()), rec.Writing, a.homeNow(id, rec.Home), orUnknown(rec.Provenance), html.EscapeString(rec.Credit),
		people, written, hands, receivedLine(rec.Received)))
	c.HTML(a.itemArchive(ctx, id, batch.WritingWorkflow, "writings/"+id.String()+"/retry", "writing"))
	c.HTML(a.writingSteps(ctx, id, l.Key()))
	c.HTML(`<h2 class="title is-5">The text, as the site will show it</h2>` + readPage(text))
	return c.Buffer()
}

// writingSteps is the writing page's steps section: its definition, its
// import and its steps.
func (a *App) writingSteps(ctx context.Context, id pid.ID, key string) string {
	return a.stepsHeading(ctx, batch.WritingWorkflow, id.String()) +
		a.parentLine(ctx, scanURL(key), "writing import "+key, batch.WritingImportWorkflow, key) +
		a.stepsTable(ctx, batch.WritingWorkflow, id.String(), itemAction("writings", id))
}
