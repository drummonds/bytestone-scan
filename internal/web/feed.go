package web

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
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
		capture += fmt.Sprintf(`<tr><th>Feeder</th><td>%s</td></tr><tr><th>Sides</th><td>%d, %d sheets, fed %s</td></tr>`,
			html.EscapeString(fed.Scanner), fed.Sides, fed.Sides/2, fed.Captured.Local().Format("2 Jan 2006 15:04"))
	}
	c.HTML(fmt.Sprintf(`<h1 class="title">Feed %d of <a href="batches/%s">batch %s</a></h1>
<p class="subtitle">%s</p>
<table class="table"><tbody>
<tr><th>Batch</th><td>%s</td></tr>
<tr><th>Home</th><td>%s (%s)</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
%s
<tr><th>Started</th><td>%s</td></tr>
</tbody></table>`, l.N, html.EscapeString(b.ID), html.EscapeString(b.ID), html.EscapeString(l.Description), html.EscapeString(b.Title),
		html.EscapeString(b.PersonName), b.Person, orUnknown(b.Provenance), html.EscapeString(b.Credit),
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
		fmt.Fprintf(&t, `<tr><td>Sheet %d</td><td><img src="%s/sides/%d" alt="Sheet %d, front" style="height:160px"></td><td><img src="%s/sides/%d" alt="Sheet %d, back" style="height:160px"></td><td>%s</td><td>%s</td></tr>`,
			n, scanURL(l.Key()), 2*n-1, n, scanURL(l.Key()), 2*n, n, letter, role)
	}
	t.WriteString(`</tbody></table>`)
	if arranging {
		t.WriteString(`<p class="help">Which letter each sheet is part of, counted from 1, and whether it is a sheet of the letter or its envelope. One load is usually one letter.</p>` +
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
		c.HTML(u.String())
	}
	c.HTML(a.feedSteps(ctx, l))
	return c.Buffer()
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
// order, each turnable until the feed is accepted, the record, the
// archive's answer and its steps.
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
<tr><th>Home</th><td>%s (%s)</td></tr>
<tr><th>Provenance</th><td>%s</td></tr>
<tr><th>Credit</th><td>%s</td></tr>
<tr><th>Scanned</th><td>%s on %s at %d ppi, %d bits</td></tr>
<tr><th>Sheets</th><td>%s</td></tr>
</tbody></table>`, id, html.EscapeString(rec.Description), html.EscapeString(rec.Batch), html.EscapeString(rec.Batch),
		scanURL(l.Key()), rec.Feed, html.EscapeString(rec.Home.Name), rec.Home.ID, orUnknown(rec.Provenance), html.EscapeString(rec.Credit),
		rec.Scanned.Local().Format("2 Jan 2006 15:04"), html.EscapeString(rec.Scanner), rec.PPI, rec.Depth, sheets))
	bagged := strings.HasPrefix(dir, filepath.Join(a.Staging, "bags")+string(filepath.Separator))
	turnable := !l.Accepted() && !bagged
	if turnable {
		c.HTML(`<p class="help">Turn any side that lies wrong, so its picture is the right way up; the master lies as scanned. Then accept the feed on its page.</p>`)
	}
	v := filepath.Base(a.renditionDir(id))
	var t strings.Builder
	for _, s := range rec.Sides() {
		if _, err := os.Stat(filepath.Join(a.renditionDir(id), s.Stem(), "display.jpg")); err != nil {
			fmt.Fprintf(&t, `<h2 class="title is-6 mt-4">%s</h2><p class="help">Not developed yet.</p>`, html.EscapeString(s.Caption))
			continue
		}
		turn := ""
		if s.Turn != 0 {
			turn = " " + turnWord(s.Turn)
		}
		turns := ""
		if turnable {
			turns = `<div class="buttons mt-2">` + letterTurnForm(id, s.Stem(), -90, "Turn ↺") + letterTurnForm(id, s.Stem(), 90, "Turn ↻") + `</div>`
		}
		fmt.Fprintf(&t, `<figure class="mt-4"><img src="letters/%s/renditions/%s/%s/display.jpg" alt="%s" style="max-width:100%%;border:1px solid #ddd">
<figcaption class="help">%s%s: <code>%s</code></figcaption></figure>%s`,
			id, v, s.Stem(), html.EscapeString(s.Caption), html.EscapeString(s.Caption), turn, html.EscapeString(s.File), turns)
	}
	c.HTML(t.String())
	c.HTML(a.itemArchive(ctx, id, batch.LetterWorkflow, "letters/"+id.String()+"/retry", "letter"))
	c.HTML(a.letterSteps(ctx, id, l.Key()))
	return c.Buffer()
}

// letterTurnForm is one Turn button of a letter's side.
func letterTurnForm(id pid.ID, side string, by int, label string) string {
	return fmt.Sprintf(`<form action="letters/%s/turn" method="post" style="display:inline"><input type="hidden" name="side" value="%s"><input type="hidden" name="by" value="%d"><button class="button is-small" type="submit">%s</button></form> `, id, html.EscapeString(side), by, label)
}

// feedOf is the feed a letter belongs to, from its record.
func (a *App) feedOf(rec batch.Letter) (*batch.Scan, error) {
	return batch.OpenScan(a.Staging, rec.Batch+"/"+strconv.Itoa(rec.Feed))
}

// letterRendition serves one file of a letter side's renditions.
func (a *App) letterRendition(w http.ResponseWriter, r *http.Request) {
	id, err := pid.Parse(r.PathValue("id"))
	version, verr := strconv.Atoi(r.PathValue("version"))
	side, file := r.PathValue("side"), r.PathValue("file")
	if err != nil || verr != nil || version < 1 || version > rendition.Version || strings.ContainsAny(side, `/\.`) || (file != "display.jpg" && file != "thumb.jpg" && file != "tiny.jpg") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store") // remade when the side is turned
	http.ServeFile(w, r, filepath.Join(a.Staging, "renditions", id.String(), strconv.Itoa(version), side, file))
}
