package web

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/rendition"
)

// newFeed is the form that starts a feed of a batch: one load of the
// sheet feeder, the sheets of letters (story 117).
func newFeed(b *batch.Batch, msg, description string) string {
	return fmt.Sprintf(`<h1 class="title">Batch %s: scan letters</h1>
<p class="subtitle">%s</p>
%s
<form action="batches/%s/feeds" method="post">
<div class="field"><label class="label">Description</label><div class="control"><input class="input" name="description" value="%s"></div>
<p class="help">What this load of sheets is, pre-filled from the batch's title. One load is usually one letter; the sheets are arranged into letters once they are fed.</p></div>
<p class="help">Load the sheets in the feeder face down, the first sheet on top, the envelope with them. Every sheet is fed both sides at %d ppi, %d bits, and kept whole: the pages are cut from the sheets later, on the renditions.</p>
<div class="field"><div class="control"><button class="button is-primary" type="submit">Feed the sheets ›</button> <a class="button" href="batches/%s">Cancel</a></div></div>
</form>`, html.EscapeString(b.ID), html.EscapeString(b.Title), msg, html.EscapeString(b.ID), html.EscapeString(description), batch.FeedPPI, batch.FeedDepth, html.EscapeString(b.ID))
}

// feedPage is a feed's details, where its workflow stands, its sheets as
// fed, the arrange form while the operator is to arrange them, and its
// letters after.
func (a *App) feedPage(r *http.Request, l *batch.Scan) string {
	ctx := r.Context()
	c := lofigui.NewContext()
	busy, waiting := a.state(r, c, l)
	b := l.Sitting()
	capture := fmt.Sprintf(`<tr><th>Fed at</th><td>%d ppi, %d bits</td></tr>`, l.Feed.PPI, batch.FeedDepth)
	if fed, err := l.Fed(); err == nil {
		leftOut := ""
		if _, n, err := l.SidesLeftOut(); err == nil && n > 0 {
			leftOut = fmt.Sprintf("; %d left out as blank", n)
		}
		capture += fmt.Sprintf(`<tr><th>Feeder</th><td>%s</td></tr><tr><th>Sides</th><td>%d, %d sheets, fed %s%s</td></tr>`,
			html.EscapeString(fed.Scanner), fed.Sides, fed.Sides/2, fed.Captured.Local().Format("2 Jan 2006 15:04"), leftOut)
	}
	c.HTML(fmt.Sprintf(`<h1 class="title">Feed %d of <a href="batches/%s">batch %s</a></h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Batch</th><td>%s</td></tr>
<tr><th>Home</th><td>%s</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
%s
<tr><th>Started</th><td>%s</td></tr>
</tbody></table>`, l.N, html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(l.Description), html.EscapeString(b.Title),
		homeText(b.PersonName, b.Person), orUnknown(b.Provenance), html.EscapeString(b.Credit),
		capture, l.Created.Local().Format("2 Jan 2006 15:04")))
	if dots := a.runDots(ctx, batch.FeedWorkflow, l.Key()); dots != "" {
		c.HTML(`<p>` + dots + ` ` + a.scanDots(ctx, l) + `</p>`)
	}
	sheets, err := l.Sheets()
	if err != nil {
		c.HTML(`<p>No sheets fed yet.</p>`)
		c.HTML(a.feedSteps(ctx, l))
		return c.Buffer()
	}
	letters, _ := l.Letters()
	final := false
	for _, lt := range letters {
		final = final || lt.ID != nil
	}
	arranging := waiting == "arrange" && !busy && !final
	var t strings.Builder
	if arranging {
		fmt.Fprintf(&t, `<form action="%s/arrange" method="post">`, scanURL(l.Key()))
	}
	t.WriteString(`<h2 class="title is-5">Sheets as fed</h2><table class="table is-striped"><thead><tr><th>Sheet</th><th>Front</th><th>Back</th><th>Letter</th><th>Role</th></tr></thead><tbody>`)
	for i, s := range sheets {
		n := i + 1
		letter, role := strconv.Itoa(s.Letter), html.EscapeString(s.Role)
		if arranging {
			letter = fmt.Sprintf(`<input class="input is-small" type="number" min="1" name="letter-%d" value="%d" style="width:5em">`, n, s.Letter)
			var opts strings.Builder
			for _, r := range batch.Roles {
				sel := ""
				if r == s.Role {
					sel = " selected"
				}
				fmt.Fprintf(&opts, `<option value="%s"%s>%s</option>`, r, sel, r)
			}
			role = fmt.Sprintf(`<div class="select is-small"><select name="role-%d">%s</select></div>`, n, opts.String())
		}
		fmt.Fprintf(&t, `<tr><td>Sheet %d</td><td><img src="%s/sides/%d" alt="Sheet %d, front" style="height:160px">%s</td><td><img src="%s/sides/%d" alt="Sheet %d, back" style="height:160px">%s</td><td>%s</td><td>%s</td></tr>`,
			n, scanURL(l.Key()), 2*n-1, n, blankMark(arranging, n, "front", s.BlankFront),
			scanURL(l.Key()), 2*n, n, blankMark(arranging, n, "back", s.BlankBack), letter, role)
	}
	t.WriteString(`</tbody></table>`)
	if arranging {
		t.WriteString(`<p class="help">Which letter each sheet is part of, counted from 1, and whether it is a sheet of the letter or its envelope. One load is usually one letter. A side marked blank is left out of the letter and recorded as blank; look at each before marking it, since faint pencil and show-through are easy to miss.</p>` +
			`<div class="buttons"><button class="button is-primary" type="submit">Arrange into letters ›</button></div></form>`)
	}
	c.HTML(t.String())
	if len(letters) > 0 {
		var u strings.Builder
		u.WriteString(`<h2 class="title is-5">Letters</h2><ul>`)
		for i, lt := range letters {
			name := fmt.Sprintf("Letter %d", i+1)
			thumbs := ""
			if lt.ID != nil {
				name = fmt.Sprintf(`Letter <a href="letters/%s">%s</a>`, *lt.ID, *lt.ID)
				thumbs = a.letterThumbnails([]pid.ID{*lt.ID})
			}
			fmt.Fprintf(&u, `<li>%s: %s %s</li>`, name, sheetList(lt.Sheets), thumbs)
		}
		u.WriteString(`</ul>`)
		if out := leftOutList(sheets); out != "" {
			u.WriteString(`<p class="help">Left out as blank: ` + out + `. The sides as fed are kept with the feed until the batch is purged.</p>`)
		}
		c.HTML(u.String())
	}
	c.HTML(a.feedSteps(ctx, l))
	return c.Buffer()
}

// blankMark is one side's blank mark under its picture: a checkbox
// while the operator arranges, off unless they set it; after, the word
// for a side left out.
func blankMark(arranging bool, sheet int, side string, blank bool) string {
	if arranging {
		checked := ""
		if blank {
			checked = " checked"
		}
		return fmt.Sprintf(`<br><label class="checkbox is-size-7"><input type="checkbox" name="blank-%d-%s"%s> blank: leave out</label>`, sheet, side, checked)
	}
	if blank {
		return `<br><span class="tag is-light">blank: left out</span>`
	}
	return ""
}

// leftOutList names the sides left out as blank by their sheet in the
// feed: "Sheet 1's back, both sides of sheet 3"; "" for none.
func leftOutList(sheets []batch.SheetChoice) string {
	var parts []string
	for i, s := range sheets {
		switch {
		case s.BlankFront && s.BlankBack:
			parts = append(parts, fmt.Sprintf("both sides of sheet %d", i+1))
		case s.BlankFront:
			parts = append(parts, fmt.Sprintf("Sheet %d's front", i+1))
		case s.BlankBack:
			parts = append(parts, fmt.Sprintf("Sheet %d's back", i+1))
		}
	}
	return strings.Join(parts, ", ")
}

// sheetList names a letter's sheets by their number in the feed: "sheets
// 1, 2 (envelope)".
func sheetList(sheets []batch.ArrangedSheet) string {
	var parts []string
	for _, s := range sheets {
		part := strconv.Itoa(s.N)
		if s.Role != "sheet" {
			part += " (" + html.EscapeString(s.Role) + ")"
		}
		parts = append(parts, part)
	}
	return plural(len(sheets), "sheet", "sheets") + " " + strings.Join(parts, ", ")
}

// feedSteps is the feed page's steps section: its definition, its batch,
// its steps and its letters.
func (a *App) feedSteps(ctx context.Context, l *batch.Scan) string {
	ids, _ := l.Items()
	return a.stepsHeading(ctx, batch.FeedWorkflow, l.Key()) +
		a.parentLine(ctx, "batches/"+html.EscapeString(l.Batch), "batch "+l.Batch, batch.Workflow, l.Batch) +
		a.stepsTable(ctx, batch.FeedWorkflow, l.Key(), feedAction(l)) +
		a.itemRows(ctx, ids, batch.LetterWorkflow, "letters", "letter")
}

// feedAction is what the operator can do at one of the feed's steps:
// Retry a failed one. A feed gone wrong is cancelled and fed again.
func feedAction(l *batch.Scan) func(step string, rec wf.StepResult) string {
	return func(step string, rec wf.StepResult) string {
		if rec.Status == wf.StatusFailed {
			return smallForm(scanURL(l.Key())+"/retry", "Retry", " is-primary")
		}
		return ""
	}
}

// letterSteps is the letter page's steps section: its definition, its
// feed and its steps.
func (a *App) letterSteps(ctx context.Context, id pid.ID, key string) string {
	return a.stepsHeading(ctx, batch.LetterWorkflow, id.String()) +
		a.parentLine(ctx, scanURL(key), "feed "+key, batch.FeedWorkflow, key) +
		a.stepsTable(ctx, batch.LetterWorkflow, id.String(), itemAction("letters", id))
}

// letterThumbnails are the tiny renditions of each letter's first side,
// linking to the letter.
func (a *App) letterThumbnails(ids []pid.ID) string {
	var t strings.Builder
	for _, id := range ids {
		rec, _, err := batch.FindLetter(a.Staging, id)
		sides := rec.Sides()
		if err != nil || len(sides) == 0 {
			continue
		}
		dir := a.renditionDir(id)
		if _, err := os.Stat(filepath.Join(dir, sides[0].Stem(), "tiny.jpg")); err != nil {
			continue
		}
		fmt.Fprintf(&t, `<a href="letters/%s"><img src="letters/%s/renditions/%s/%s/tiny.jpg" alt="letter %s" title="letter %s" style="height:48px;margin-right:4px"></a>`,
			id, id, filepath.Base(dir), sides[0].Stem(), id, id)
	}
	return t.String()
}

// letterPage is a letter's details: every side at display size in sheet
// order, each laid out, turned, folded and its pages numbered, until the
// feed is accepted, a side left out as blank named and said to be so,
// its pages in reading order, the archive's answer and its steps.
func (a *App) letterPage(r *http.Request, id pid.ID, rec batch.Letter, dir string, l *batch.Scan) string {
	ctx := r.Context()
	c := lofigui.NewContext()
	envelopes := 0
	for _, s := range rec.Sheets {
		if s.Role == "envelope" {
			envelopes++
		}
	}
	sheets := strconv.Itoa(len(rec.Sheets))
	if envelopes > 0 {
		sheets += fmt.Sprintf(", %d of them %s", envelopes, plural(envelopes, "an envelope", "envelopes"))
	}
	c.HTML(fmt.Sprintf(`<h1 class="title">Letter %s</h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Batch</th><td><a href="batches/%s">%s</a></td></tr>
<tr><th>Feed</th><td><a href="%s">Feed %d</a></td></tr>
<tr><th>Home</th><td>%s</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
<tr><th>Scanned</th><td>%s on %s at %d ppi, %d bits</td></tr>
<tr><th>Sheets</th><td>%s</td></tr>
</tbody></table>`, id, html.EscapeString(rec.Description), html.EscapeString(rec.Batch), html.EscapeString(rec.Batch),
		scanURL(l.Key()), rec.Feed, a.homeNow(id, rec.Home), orUnknown(rec.Provenance), html.EscapeString(rec.Credit),
		rec.Scanned.Local().Format("2 Jan 2006 15:04"), html.EscapeString(rec.Scanner), rec.PPI, rec.Depth, sheets))
	bagged := strings.HasPrefix(dir, filepath.Join(a.Staging, "bags")+string(filepath.Separator))
	turnable := !l.Accepted() && !bagged
	if turnable {
		c.HTML(`<p class="help">Turn any side that lies wrong, so its picture is the right way up; the master lies as scanned. The feeder sees a sheet's back flipped, so its two sides turn together, opposite ways; <em>This side only</em> turns one, for a back written the other way up. Mark a side <em>Folded in half</em> when it holds two pages, then number the pages in the order they are read. Then accept the feed on its page.</p>`)
	}
	v := filepath.Base(a.renditionDir(id))
	var t strings.Builder
	for _, s := range rec.EverySide() {
		if s.Blank {
			fmt.Fprintf(&t, `<h2 class="title is-6 mt-4">%s</h2><p class="help">Blank: left out of the letter.</p>`, html.EscapeString(s.Caption))
			continue
		}
		if _, err := os.Stat(filepath.Join(a.renditionDir(id), s.Stem(), "display.jpg")); err != nil {
			fmt.Fprintf(&t, `<h2 class="title is-6 mt-4">%s</h2><p class="help">Not developed yet.</p>`, html.EscapeString(s.Caption))
			continue
		}
		said := ""
		if s.Turn != 0 {
			said = " " + turnWord(s.Turn)
		}
		if s.Folded() {
			said += ", folded in half"
		}
		layout := ""
		if turnable {
			layout = `<div class="buttons mt-2">` + letterTurnForm(id, s.Stem(), -90, "Turn ↺", false) + letterTurnForm(id, s.Stem(), 90, "Turn ↻", false)
			if s.Coupled {
				layout += letterTurnForm(id, s.Stem(), -90, "This side only ↺", true) + letterTurnForm(id, s.Stem(), 90, "This side only ↻", true)
			}
			layout += letterFoldForm(id, s.Stem(), s.Folded()) + `</div>` + pageNumberFields(s)
		}
		fmt.Fprintf(&t, `<figure class="mt-4" id="%s"><img src="letters/%s/renditions/%s/%s/display.jpg" alt="%s" style="max-width:100%%;border:1px solid #ddd">
<figcaption class="help">%s%s: <code>%s</code></figcaption></figure>%s`,
			s.Stem(), id, v, s.Stem(), html.EscapeString(s.Caption), html.EscapeString(s.Caption), said, html.EscapeString(s.File), layout)
	}
	c.HTML(t.String())
	c.HTML(a.letterPages(id, rec, turnable))
	c.HTML(a.itemArchive(ctx, id, batch.LetterWorkflow, "letters/"+id.String()+"/retry", "letter"))
	c.HTML(a.letterSteps(ctx, id, l.Key()))
	return c.Buffer()
}

// letterPages is the letter's Pages: its numbered pages in reading
// order, each picture linked to the side it is cut from; and, while the
// letter is laid out, the form the sides' page numbers are sent by. A
// page with no picture of its own, one developed before story 202, is
// shown by its side's.
func (a *App) letterPages(id pid.ID, rec batch.Letter, numbering bool) string {
	dir := a.renditionDir(id)
	v := filepath.Base(dir)
	var t strings.Builder
	t.WriteString(`<h2 class="title is-5 mt-5" id="pages">Pages</h2><div class="is-flex is-flex-wrap-wrap" style="gap:1rem">`)
	for _, p := range rec.Pages() {
		file := rendition.PageFile(p.N)
		if _, err := os.Stat(filepath.Join(dir, p.Side.Stem(), file)); err != nil {
			file = "display.jpg"
		}
		caption := fmt.Sprintf("Page %d: %s", p.N, p.Side.Caption)
		if p.Place != "" {
			caption += ", " + p.Place
		}
		fmt.Fprintf(&t, `<figure><a href="letters/%s#%s"><img src="letters/%s/renditions/%s/%s/%s" alt="%s" style="height:360px;border:1px solid #ddd"></a><figcaption class="help">%s</figcaption></figure>`,
			id, p.Side.Stem(), id, v, p.Side.Stem(), file, html.EscapeString(caption), html.EscapeString(caption))
	}
	t.WriteString(`</div>`)
	if numbering {
		fmt.Fprintf(&t, `<form id="number-pages" action="letters/%s/pages" method="post"><p class="help">The numbers beside each side are the order the letter is read in: from 1, none twice, no gap. A page with nothing on it is marked blank and has no number.</p><div class="buttons"><button class="button is-small" type="submit">Number the pages</button></div></form>`, id)
	}
	return t.String()
}

// pageNumberFields are one side's page numbers, left to right as
// turned, each with its blank mark: fields of the Pages form.
func pageNumberFields(s batch.LetterSide) string {
	var t strings.Builder
	t.WriteString(`<div class="is-flex is-align-items-center mb-2" style="gap:1rem">`)
	for i, p := range s.Pages {
		label := "Page"
		if s.Folded() {
			label = []string{"Left page", "Right page"}[i]
		}
		number, checked := strconv.Itoa(p.N), ""
		if p.Blank {
			number, checked = "", " checked"
		}
		field := fmt.Sprintf("%s-%d", s.Stem(), i+1)
		fmt.Fprintf(&t, `<span class="is-size-7">%s <input class="input is-small" form="number-pages" type="number" min="1" name="page-%s" value="%s" style="width:5em"> <label class="checkbox"><input form="number-pages" type="checkbox" name="blank-%s"%s> blank</label></span>`,
			label, field, number, field, checked)
	}
	t.WriteString(`</div>`)
	return t.String()
}

// letterTurnForm is one Turn button of a letter's side: of the sheet's
// two sides together, or, alone, of this side only.
func letterTurnForm(id pid.ID, side string, by int, label string, alone bool) string {
	only := ""
	if alone {
		only = `<input type="hidden" name="alone" value="1">`
	}
	return fmt.Sprintf(`<form action="letters/%s/turn" method="post" style="display:inline"><input type="hidden" name="side" value="%s"><input type="hidden" name="by" value="%d">%s<button class="button is-small" type="submit">%s</button></form> `, id, html.EscapeString(side), by, only, label)
}

// letterFoldForm is a side's fold button: Folded in half, or, on a side
// so marked, the button that takes the fold off.
func letterFoldForm(id pid.ID, side string, folded bool) string {
	label, to := "Folded in half", 1
	if folded {
		label, to = "Take the fold off", 0
	}
	return fmt.Sprintf(`<form action="letters/%s/fold" method="post" style="display:inline"><input type="hidden" name="side" value="%s"><input type="hidden" name="folded" value="%d"><button class="button is-small" type="submit">%s</button></form> `, id, html.EscapeString(side), to, label)
}

// feedOf is the feed a letter belongs to, from its record.
func (a *App) feedOf(rec batch.Letter) (*batch.Scan, error) {
	return batch.OpenScan(a.Staging, rec.Batch+"/"+strconv.Itoa(rec.Feed))
}

// pagePicture matches the name of a letter page's picture.
var pagePicture = regexp.MustCompile(`^page-\d{2,}\.jpg$`)

// letterRendition serves one file of a letter side's renditions: its
// display images or the picture of one of its pages.
func (a *App) letterRendition(w http.ResponseWriter, r *http.Request) {
	id, err := pid.Parse(r.PathValue("id"))
	version, verr := strconv.Atoi(r.PathValue("version"))
	side, file := r.PathValue("side"), r.PathValue("file")
	if err != nil || verr != nil || version < 1 || version > rendition.Version || strings.ContainsAny(side, `/\.`) || (file != "display.jpg" && file != "thumb.jpg" && file != "tiny.jpg" && !pagePicture.MatchString(file)) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store") // remade when the side is turned or its pages numbered
	http.ServeFile(w, r, filepath.Join(a.Staging, "renditions", id.String(), strconv.Itoa(version), side, file))
}
