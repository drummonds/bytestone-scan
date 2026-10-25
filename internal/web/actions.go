package web

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/lofigui"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// worker runs one scan's step at a time in the background: there is one
// scanner.
type worker struct {
	mu      sync.Mutex
	scan    string // the key of the scan a step is running for; empty when idle
	started time.Time
	step    string // the workflow step running, once one has started
	stepAt  time.Time
	running sync.WaitGroup
	pending []*batch.Scan // scans to run to their next wait once the step running ends
}

var errBusy = errors.New("busy")

// start runs prepare and then the scan's workflow up to its next wait, in
// the background, unless a step is already running.
func (a *App) start(l *batch.Scan, prepare func() error) error {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	if a.work.scan != "" {
		return fmt.Errorf("%w: the scanner is busy with scan %s", errBusy, a.work.scan)
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	a.launch(l)
	return nil
}

// tick runs the scan's workflow to its next wait: now when the worker is
// idle, else once the step running ends. For what needs no scanner and
// cannot be refused, such as a photo's description.
func (a *App) tick(l *batch.Scan) {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	if a.work.scan == "" {
		a.launch(l)
		return
	}
	for _, p := range a.work.pending {
		if p.Key() == l.Key() {
			return
		}
	}
	a.work.pending = append(a.work.pending, l)
}

// launch runs the scan's workflow in the background, then the first scan
// pending, if any. The lock is held.
func (a *App) launch(l *batch.Scan) {
	a.work.scan, a.work.started = l.Key(), time.Now()
	a.work.step, a.work.stepAt = "", a.work.started
	a.work.running.Go(func() {
		// a failure is recorded on the run, where the scan page reads it
		l.Run(context.Background(), a.env(), func(step string) {
			a.work.mu.Lock()
			a.work.step, a.work.stepAt = step, time.Now()
			a.work.mu.Unlock()
		})
		a.work.mu.Lock()
		a.work.scan = ""
		if len(a.work.pending) > 0 {
			next := a.work.pending[0]
			a.work.pending = a.work.pending[1:]
			a.launch(next)
		}
		a.work.mu.Unlock()
	})
}

// wait blocks until no step is running.
func (a *App) wait() { a.work.running.Wait() }

// env is what the scans' workflows run with.
func (a *App) env() batch.Env {
	return batch.Env{Store: a.Store, Scanner: a.Scanner, Archive: a.Archive, Now: a.Now}
}

// running is the key of the scan a step is running for, or empty.
func (a *App) running() string {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	return a.work.scan
}

// busyWith reports whether a step of the scan with key is running, and if
// so which workflow step it is at (empty before the first starts) and
// since when.
func (a *App) busyWith(key string) (bool, string, time.Time) {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	return a.work.scan == key, a.work.step, a.work.stepAt
}

const newBatchForm = `<h1 class="title">New batch</h1>
%s
<form action="batches" method="post" class="box">
<div class="field"><label class="label">Title: what is being scanned</label>
<div class="control"><input class="input" name="title" value="%s" required></div>
<p class="help">A batch is one sitting: as many scans of the bed as the shoebox needs. Each scan’s description starts as this title.</p></div>
<div class="field"><label class="label">Home: the person whose collection the photos belong to</label>
<div class="control"><div class="select"><select name="person" required>%s</select></div></div></div>
<div class="field"><label class="label">Provenance: where the prints came from</label>
<div class="control"><input class="input" name="provenance" value="%s" placeholder="Aunt Jean’s shoebox, envelope marked 1950s"></div>
<p class="help">Copied into every photo. Leave it blank if unknown.</p></div>
<div class="field"><label class="label">Credit: the terms the photos are shown under</label>
<div class="field has-addons">
<div class="control is-expanded"><input class="input" name="credit" value="%s" placeholder="Drummond family collection, CC BY 4.0" required></div>
<div class="control"><button class="button" type="button" data-insert="©" title="Insert the copyright sign">©</button></div>
<div class="control"><button class="button" type="button" data-insert="CC BY 4.0" title="Insert the Creative Commons attribution licence">CC BY 4.0</button></div>
</div></div>
<div class="field"><label class="label">Scan at</label>
<div class="control"><div class="select"><select name="ppi">%s</select></div></div>
<p class="help">The fronts are scanned at this ppi and averaged down to each master’s ppi: 600, or 1200 for prints smaller than 6 × 4 inches. A higher scan ppi takes longer but lowers noise, and the master is the same size whichever you pick. The backs are scanned at 300 ppi.</p>
<p class="help">600 ppi takes about 15 s for a 7 × 5 inch print, 1200 about 75 s, 2400 about 2½ minutes.</p></div>
<button class="button is-primary" type="submit">Scan the preview</button>
</form>
<script>
// Each data-insert button puts its text into the credit field at the cursor.
document.querySelectorAll('[data-insert]').forEach(function (b) {
	b.addEventListener('click', function () {
		var f = document.querySelector('[name=credit]'), t = b.dataset.insert;
		var at = f.selectionStart, v = f.value;
		if (at > 0 && v[at - 1] !== ' ') t = ' ' + t;
		f.setRangeText(t, at, f.selectionEnd, 'end');
		f.focus();
	});
});
</script>`

// entered is what the New batch form holds, as typed.
type entered struct {
	title, person, provenance, credit string
	ppi                               int
}

const newScanForm = `<h1 class="title">Batch %s: one more scan</h1>
<p class="subtitle">%s</p>
%s
<form action="batches/%s/scans" method="post" class="box">
<div class="field"><label class="label">Description: what is on the bed this time</label>
<div class="control"><input class="input" name="description" value="%s" required></div>
<p class="help">Copied into each of this scan’s photos. Home, provenance, credit and scan ppi are the batch’s.</p></div>
<button class="button is-primary" type="submit">Scan the preview</button>
</form>`

// newScan is the form for a batch's next scan, its description starting
// as the batch's title.
func newScan(b *batch.Batch, msg, description string) string {
	return fmt.Sprintf(newScanForm, html.EscapeString(b.ID), html.EscapeString(b.Title), msg, html.EscapeString(b.ID), html.EscapeString(description))
}

// newBatch is the New batch page: msg above the form, and the form holding
// what was entered. With no one in the register there is no form.
func (a *App) newBatch(msg string, e entered) string {
	people, err := a.Archive.People()
	if err != nil {
		return `<h1 class="title">New batch</h1><div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`
	}
	return fmt.Sprintf(newBatchForm, msg, html.EscapeString(e.title), personOptions(people, e.person),
		html.EscapeString(e.provenance), html.EscapeString(e.credit), ppiOptions(e.ppi))
}

// prefilled is a fresh form: provenance and credit are the latest
// batch's, since a shoebox is many scans of the bed.
func (a *App) prefilled() entered {
	e := entered{ppi: 2400}
	if latest, err := batch.Latest(a.Staging); err == nil && latest != nil {
		e.provenance, e.credit = latest.Provenance, latest.Credit
	}
	return e
}

func personOptions(people []archive.Person, selected string) string {
	var s strings.Builder
	s.WriteString(`<option value="">Choose…</option>`)
	for _, p := range people {
		sel := ""
		if p.ID.String() == selected {
			sel = " selected"
		}
		s.WriteString(fmt.Sprintf(`<option value="%s"%s>%s (%s)</option>`, p.ID, sel, html.EscapeString(p.Name), p.ID))
	}
	return s.String()
}

func ppiOptions(selected int) string {
	var s strings.Builder
	for _, p := range []int{600, 1200, 2400} {
		sel := ""
		if p == selected {
			sel = " selected"
		}
		s.WriteString(fmt.Sprintf(`<option value="%d"%s>%d ppi</option>`, p, sel, p))
	}
	return s.String()
}

func (a *App) actions(mux *http.ServeMux, render func(http.ResponseWriter, string, string)) {
	fail := func(w http.ResponseWriter, err error, code int) {
		if errors.Is(err, errBusy) {
			code = http.StatusConflict
		}
		w.WriteHeader(code)
		render(w, "Not done", `<div class="notification is-danger">`+html.EscapeString(err.Error())+`</div>`)
	}

	mux.HandleFunc("GET /batches/new", func(w http.ResponseWriter, r *http.Request) {
		render(w, "New batch", a.newBatch("", a.prefilled()))
	})
	mux.HandleFunc("POST /batches", func(w http.ResponseWriter, r *http.Request) {
		// refuse before recording a batch that could not be scanned
		if key := a.running(); key != "" {
			fail(w, fmt.Errorf("%w: the scanner is busy with scan %s", errBusy, key), http.StatusConflict)
			return
		}
		e := entered{title: r.FormValue("title"), person: r.FormValue("person"),
			provenance: r.FormValue("provenance"), credit: r.FormValue("credit")}
		e.ppi, _ = strconv.Atoi(r.FormValue("ppi"))
		person, err := pid.Parse(e.person)
		if e.person == "" {
			err = errors.New("choose the home person")
		}
		var b *batch.Batch
		if err == nil {
			b, err = batch.Describe(a.Staging, a.Archive, a.Store, batch.Description{
				Text: e.title, Person: person, ScanPPI: e.ppi, Provenance: e.provenance, Credit: e.credit}, a.Now())
		}
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			render(w, "New batch", a.newBatch(`<div class="notification is-danger">`+html.EscapeString(err.Error())+`</div>`, e))
			return
		}
		a.startScan(w, r, b, "", fail)
	})
	mux.HandleFunc("GET /batches/{id}/scans/new", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if b.Finished() {
			fail(w, fmt.Errorf("batch %s: scanning is finished", b.ID), http.StatusUnprocessableEntity)
			return
		}
		render(w, "One more scan", newScan(b, "", b.Title))
	})
	mux.HandleFunc("POST /batches/{id}/scans", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if key := a.running(); key != "" {
			fail(w, fmt.Errorf("%w: the scanner is busy with scan %s", errBusy, key), http.StatusConflict)
			return
		}
		a.startScan(w, r, b, r.FormValue("description"), fail)
	})
	mux.HandleFunc("POST /batches/{id}/finish", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if _, err := b.Finish(r.Context(), a.Store); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		http.Redirect(w, r, "/batches/"+b.ID, http.StatusSeeOther)
	})

	// Each action prepares the scan, then runs it to its next wait.
	act := func(name string, prepare func(r *http.Request, l *batch.Scan) error) {
		mux.HandleFunc("POST /batches/{id}/scans/{n}/"+name, func(w http.ResponseWriter, r *http.Request) {
			l, err := batch.OpenScan(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if err := a.start(l, func() error { return prepare(r, l) }); err != nil {
				fail(w, err, http.StatusUnprocessableEntity)
				return
			}
			http.Redirect(w, r, "/"+scanURL(l.Key()), http.StatusSeeOther)
		})
	}
	act("preview", func(r *http.Request, l *batch.Scan) error {
		return l.Redo(r.Context(), a.Store, "preview", 0)
	})
	act("prints", func(r *http.Request, l *batch.Scan) error {
		return l.Redo(r.Context(), a.Store, "confirm", 0)
	})
	act("confirm", func(r *http.Request, l *batch.Scan) error { return l.Confirm() })
	act("rescan", func(r *http.Request, l *batch.Scan) error {
		ppi, _ := strconv.Atoi(r.FormValue("ppi"))
		return l.Redo(r.Context(), a.Store, "fronts", ppi)
	})
	act("backs", func(r *http.Request, l *batch.Scan) error { return l.Turned() })
	act("no-backs", func(r *http.Request, l *batch.Scan) error { return l.NoBacks() })
	// the ticks say which backs are scanned: an unticked back is declared
	// blank, a ticked one not after all; a post without them scans every
	// back not already declared blank
	act("confirm-backs", func(r *http.Request, l *batch.Scan) error {
		if r.FormValue("select") != "" {
			_, ids, err := l.Found()
			if err != nil {
				return err
			}
			ticked := map[string]bool{}
			for _, n := range r.Form["scan"] {
				ticked[n] = true
			}
			for i, id := range ids {
				if err := l.SetBackBlank(id, !ticked[strconv.Itoa(i+1)]); err != nil {
					return err
				}
			}
		}
		return l.ConfirmBacks()
	})
	// previewing again, and a rescan from the quality check, go through
	// the backs preview with the prints still turned
	act("preview-backs", func(r *http.Request, l *batch.Scan) error {
		return l.PreviewBacksAgain(r.Context(), a.Store)
	})
	act("rescan-backs", func(r *http.Request, l *batch.Scan) error {
		return l.PreviewBacksAgain(r.Context(), a.Store)
	})
	act("accept", func(r *http.Request, l *batch.Scan) error { return l.Accept() })
	act("retry", func(r *http.Request, l *batch.Scan) error { return nil })
	act("reshape", func(r *http.Request, l *batch.Scan) error {
		n, edge, by, absolute, err := parseMove(r.FormValue("move"))
		if err != nil {
			return err
		}
		if absolute {
			return fmt.Errorf("bad move %q: the quality page's arrows move by a distance", r.FormValue("move"))
		}
		return l.Reshape(r.Context(), a.Store, n-1, edge, by)
	})

	// Describing a photo prepares its scan's photos to go on, then ticks them.
	mux.HandleFunc("POST /photos/{id}/describe", func(w http.ResponseWriter, r *http.Request) {
		id, err := pid.Parse(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		p, _, err := batch.FindPhoto(a.Staging, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		l, err := batch.OpenScan(a.Staging, fmt.Sprintf("%s/%d", p.Batch, p.Scan))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		// the caption is not a claim: "nothing known" keeps it
		d := batch.PhotoDescription{Caption: r.FormValue("caption")}
		if r.FormValue("skip") == "" {
			d = batch.PhotoDescription{Caption: d.Caption, Date: r.FormValue("date"), Approximate: r.FormValue("approximate") != "",
				Photographer: r.FormValue("photographer"), Back: r.FormValue("back"), Source: r.FormValue("source")}
		}
		// A photo whose run has reached the accepted wait, or waits to be
		// described, is no longer written by any step of its scan: its
		// description is recorded whatever the scanner is doing, and its
		// run ticked when the step running ends. Before that, the scan's
		// own step may be writing the photo's record, so Save waits for it.
		describe := func() error { return l.DescribePhoto(id, d) }
		if _, step := a.whereRun(r.Context(), batch.PhotoWorkflow, id.String()); step == "accepted" || step == "describe" {
			if err := describe(); err != nil {
				fail(w, err, http.StatusUnprocessableEntity)
				return
			}
			a.tick(l)
		} else if err := a.start(l, describe); err != nil {
			if busy, step, _ := a.busyWith(l.Key()); busy {
				err = fmt.Errorf("%w: wait for scan %s's %s step to finish", errBusy, l.Key(), step)
			}
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		http.Redirect(w, r, "/photos/"+id.String(), http.StatusSeeOther)
	})

	// Editing the regions needs no scanner, only that no step is running on
	// the scan. Each edit names its print, counted from 1.
	edit := func(name string, change func(r *http.Request, l *batch.Scan) (int, error)) {
		mux.HandleFunc("POST /batches/{id}/scans/{n}/"+name, func(w http.ResponseWriter, r *http.Request) {
			l, err := batch.OpenScan(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if a.running() == l.Key() {
				fail(w, fmt.Errorf("%w: wait for scan %s's step to finish", errBusy, l.Key()), http.StatusConflict)
				return
			}
			n, err := change(r, l)
			if err != nil {
				fail(w, err, http.StatusUnprocessableEntity)
				return
			}
			http.Redirect(w, r, fmt.Sprintf("/%s#print-%d", scanURL(l.Key()), n), http.StatusSeeOther)
		})
	}
	// Keeping a scan for diagnosis copies its files, so it waits for a
	// step of the scan to finish writing them; another scan's step is no
	// bar. The scan itself is untouched.
	mux.HandleFunc("POST /batches/{id}/scans/{n}/keep", func(w http.ResponseWriter, r *http.Request) {
		l, err := batch.OpenScan(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if busy, _, _ := a.busyWith(l.Key()); busy {
			fail(w, fmt.Errorf("%w: wait for scan %s's step to finish", errBusy, l.Key()), http.StatusConflict)
			return
		}
		state := "not started"
		if run, waiting := a.where(r.Context(), l); run != nil {
			state = string(run.Status)
			if waiting != "" {
				state += " at " + waiting
			}
		}
		now := time.Now()
		if a.Now != nil {
			now = a.Now()
		}
		o := batch.KeepOptions{Note: r.FormValue("note"), State: state, Version: version(), Masters: r.FormValue("masters") != "", Now: now}
		if _, err := l.Keep(r.Context(), a.Store, o); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		http.Redirect(w, r, "/"+scanURL(l.Key()), http.StatusSeeOther)
	})
	// A back declared blank, or not after all: the photo's record changes
	// and a back master may go, so it waits for a step of the scan.
	mux.HandleFunc("POST /batches/{id}/scans/{n}/blank-back", func(w http.ResponseWriter, r *http.Request) {
		l, err := batch.OpenScan(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		id, err := pid.Parse(r.FormValue("photo"))
		if err != nil {
			fail(w, fmt.Errorf("photo %q: %w", r.FormValue("photo"), err), http.StatusUnprocessableEntity)
			return
		}
		if busy, _, _ := a.busyWith(l.Key()); busy {
			fail(w, fmt.Errorf("%w: wait for scan %s's step to finish", errBusy, l.Key()), http.StatusConflict)
			return
		}
		if err := l.SetBackBlank(id, r.FormValue("blank") == "1"); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		a.tick(l) // a photo waiting for its back goes on without one
		http.Redirect(w, r, "/"+scanURL(l.Key()), http.StatusSeeOther)
	})
	mux.HandleFunc("POST /diagnosis/discard", func(w http.ResponseWriter, r *http.Request) {
		if err := batch.DiscardDiagnosis(a.Staging, r.FormValue("name")); err != nil {
			fail(w, err, http.StatusUnprocessableEntity)
			return
		}
		http.Redirect(w, r, "/batches", http.StatusSeeOther)
	})

	edit("adjust", func(r *http.Request, l *batch.Scan) (int, error) {
		n, edge, by, absolute, err := parseMove(r.FormValue("move"))
		if err != nil {
			return 0, err
		}
		if absolute {
			return n, l.SetEdge(n-1, edge, by)
		}
		return n, l.Adjust(n-1, edge, by)
	})
	edit("split", func(r *http.Request, l *batch.Scan) (int, error) {
		n, err := strconv.Atoi(r.FormValue("print"))
		if err != nil {
			return 0, fmt.Errorf("no print given")
		}
		return n, l.Split(n - 1)
	})
	edit("remove", func(r *http.Request, l *batch.Scan) (int, error) {
		n, err := strconv.Atoi(r.FormValue("print"))
		if err != nil {
			return 0, fmt.Errorf("no print given")
		}
		return 1, l.Remove(n - 1)
	})
	a.deletion(mux, render, fail)
}

// parseMove reads an arrow's "<print> <edge> <mm>", mm outwards when
// positive, or a ruler's "<print> <edge> =<mm>", the edge's position in
// the region's own frame, reported as absolute.
func parseMove(s string) (n int, edge find.Edge, by float64, absolute bool, err error) {
	f := strings.Fields(s)
	if len(f) != 3 {
		return 0, 0, 0, false, fmt.Errorf("bad move %q", s)
	}
	n, err = strconv.Atoi(f[0])
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("bad move %q", s)
	}
	edge, err = find.ParseEdge(f[1])
	if err != nil {
		return 0, 0, 0, false, err
	}
	absolute = strings.HasPrefix(f[2], "=")
	by, err = strconv.ParseFloat(strings.TrimPrefix(f[2], "="), 64)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("bad move %q", s)
	}
	return n, edge, by, absolute, nil
}

// startScan adds a scan to the batch and starts its preview, landing on
// its page.
func (a *App) startScan(w http.ResponseWriter, r *http.Request, b *batch.Batch, description string, fail func(http.ResponseWriter, error, int)) {
	l, err := b.NewScan(description, a.Now())
	if err != nil {
		fail(w, err, http.StatusUnprocessableEntity)
		return
	}
	if err := a.start(l, nil); err != nil {
		fail(w, err, http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/"+scanURL(l.Key()), http.StatusSeeOther)
}

// state writes where the scan's workflow stands and the operator's
// choices there, reporting whether a step is running and the step the
// scan is waiting at, if any.
func (a *App) state(r *http.Request, c *lofigui.Context, l *batch.Scan) (bool, string) {
	if busy, step, since := a.busyWith(l.Key()); busy {
		c.HTML(`<div id="progress">` + a.progress(r.Context(), l, step, since) + `</div>` + fmt.Sprintf(pollProgress, scanURL(l.Key())))
		return true, ""
	}
	run, waiting := a.where(r.Context(), l)
	if run == nil {
		c.HTML(`<div class="notification">Not started.</div>`)
		return false, ""
	}
	button := func(action, label, class string) string {
		return fmt.Sprintf(`<form action="%s/%s" method="post" style="display:inline"><button class="button %s" type="submit">%s</button></form> `,
			scanURL(l.Key()), action, class, label)
	}
	switch {
	case run.Status == wf.StatusFailed:
		c.HTML(`<div class="notification is-danger">Failed: ` + html.EscapeString(run.Error) + `</div>` +
			`<div class="buttons">` + button("retry", "Retry", "is-primary") + `</div>`)
	case run.Status == wf.StatusCompleted:
		if waiting := a.undescribed(r.Context(), l); len(waiting) > 0 {
			var links []string
			for _, id := range waiting {
				links = append(links, fmt.Sprintf(`<a href="photos/%s">%s</a>`, id, id))
			}
			c.HTML(fmt.Sprintf(`<div class="notification is-warning">Accepted. %d %s waiting to be described: %s. Say what is known of each on its page, or that nothing is.</div>`+
				`<div class="buttons"><a class="button is-primary" href="photos/%s">Describe photo %s ›</a></div>`,
				len(waiting), plural(len(waiting), "photo", "photos"), strings.Join(links, ", "), waiting[0], waiting[0]))
		} else {
			c.HTML(a.outcome(l))
		}
	case waiting == "confirm":
		c.HTML(`<div class="notification is-warning">Check the prints found on the preview.</div><div class="buttons">` +
			button("confirm", "Confirm and scan ›", "is-primary") + button("preview", "Rescan the preview", "") + `</div>`)
	case waiting == "backs preview":
		c.HTML(`<div class="notification is-warning">Turn each print over where it lies, so its back faces the glass, then preview the backs. If nothing is written or printed on any back, skip them.</div><div class="buttons">` +
			button("backs", "Preview the backs ›", "is-primary") + button("no-backs", "No backs ›", "") + button("prints", "‹ Back to the prints", "") + `</div>`)
	case waiting == "backs":
		notice := "Check that each back lies where its front did, and untick any back with nothing on it, then scan the backs. A print that moved is put back and the backs previewed again. If nothing is written or printed on any back after all, skip them."
		if backs, err := l.BacksFound(); err == nil {
			var warnings []string
			for i, s := range backs {
				if s.Warning != "" {
					warnings = append(warnings, fmt.Sprintf("print %d: %s", i+1, html.EscapeString(s.Warning)))
				}
			}
			if len(warnings) > 0 {
				notice += `<ul><li>` + strings.Join(warnings, `</li><li>`) + `</li></ul>`
			}
		}
		c.HTML(`<div class="notification is-warning">` + notice + `</div><div class="buttons">` +
			fmt.Sprintf(`<form id="scan-backs" action="%s/confirm-backs" method="post" style="display:inline"><input type="hidden" name="select" value="1"><button class="button is-primary" type="submit">Scan the backs ›</button></form> `, scanURL(l.Key())) +
			button("no-backs", "No backs ›", "") + button("preview-backs", "Preview the backs again", "") + button("prints", "‹ Back to the prints", "") + `</div>`)
	case waiting == "check quality":
		// the forward step is what accepting leads to: the describing
		// still to do, or, every photo described, the archive
		notice, accept := "Check each photo's quality, front and back, then accept or send the scan back. Every photo is described: accepting sends them to the archive.", "Accept and archive ›"
		if n := a.notDescribed(l); n > 0 {
			notice = fmt.Sprintf("Check each photo's quality, front and back, then accept or send the scan back. %d %s still to be described: say what is known of each on its page, now or after accepting.", n, plural(n, "photo", "photos"))
			accept = fmt.Sprintf("Accept, then describe %d %s ›", n, plural(n, "photo", "photos"))
		}
		c.HTML(`<div class="notification is-warning">` + notice + `</div><div class="buttons">` +
			button("accept", accept, "is-primary") +
			fmt.Sprintf(`<form action="%s/rescan" method="post" style="display:inline"><div class="select"><select name="ppi">%s</select></div> <button class="button" type="submit">Rescan at</button></form> `,
				scanURL(l.Key()), ppiOptions(l.Sitting().ScanPPI)) +
			button("rescan-backs", "Rescan the backs", "") +
			button("prints", "‹ Back to the prints", "") + `</div>`)
	default:
		c.HTML(`<div class="notification">` + html.EscapeString(string(run.Status)) + `</div>` +
			`<div class="buttons">` + button("retry", "Carry on", "") + `</div>`)
	}
	return false, waiting
}

// outcome is how a finished scan's photos stand with the archive.
func (a *App) outcome(l *batch.Scan) string {
	o, err := l.Outcome()
	if err != nil {
		return `<div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`
	}
	msg, class := outcomeText(o)
	return `<div class="notification ` + class + `">Done: ` + msg + `.</div>`
}

// outcomeText says how photos stand with the archive, the rejected ones
// linked, and the Bulma class that colours it: success unless any is
// rejected or not there yet.
func outcomeText(o batch.Outcome) (msg, class string) {
	msg = fmt.Sprintf("%d %s held by the archive", o.Held, plural(o.Held, "photo", "photos"))
	class = "is-success"
	if len(o.Rejected) > 0 {
		links := make([]string, len(o.Rejected))
		for i, id := range o.Rejected {
			links[i] = fmt.Sprintf(`<a href="photos/%s">%s</a>`, id, id)
		}
		msg += fmt.Sprintf("; %d rejected as a duplicate: %s", len(o.Rejected), strings.Join(links, ", "))
		class = "is-warning"
	}
	if o.Pending > 0 {
		msg += fmt.Sprintf("; %d not yet deposited", o.Pending)
		class = "is-warning"
	}
	return msg, class
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// notDescribed counts the scan's photos not yet described, whether or
// not their runs have reached the describe step.
func (a *App) notDescribed(l *batch.Scan) int {
	_, ids, _ := l.Found()
	n := 0
	for _, id := range ids {
		if p, _, err := batch.FindPhoto(a.Staging, id); err == nil && p.Described == nil {
			n++
		}
	}
	return n
}

// undescribed are the scan's photos waiting to be described.
func (a *App) undescribed(ctx context.Context, l *batch.Scan) []pid.ID {
	_, ids, _ := l.Found()
	var waiting []pid.ID
	for _, id := range ids {
		if _, step := a.whereRun(ctx, batch.PhotoWorkflow, id.String()); step == "describe" {
			waiting = append(waiting, id)
		}
	}
	return waiting
}

// where is the scan's run and the step it waits at, if it is waiting.
func (a *App) where(ctx context.Context, l *batch.Scan) (*wf.RunRecord, string) {
	return a.whereRun(ctx, batch.ScanWorkflow, l.Key())
}

// whereRun is the run of the instance key of typ and the step it waits
// at, if it is waiting.
func (a *App) whereRun(ctx context.Context, typ wf.WorkflowType, key string) (*wf.RunRecord, string) {
	run, err := a.Store.FindRunByKey(ctx, typ, key)
	if err != nil || run == nil {
		return nil, ""
	}
	if run.Status != wf.StatusWaiting {
		return run, ""
	}
	steps, _ := a.Store.ListSteps(ctx, run.ID)
	waiting := ""
	for _, s := range steps {
		if s.Status == wf.StatusWaiting {
			waiting = s.StepName
		}
	}
	return run, waiting
}

// pollProgress redraws the progress block every two seconds while the
// scan is busy, and the page once it is not.
const pollProgress = `<script>
(function poll() {
	setTimeout(function () {
		fetch('%s/progress').then(function (r) {
			if (r.status === 204) { location.reload(); return; }
			return r.text().then(function (h) { document.getElementById('progress').innerHTML = h; poll(); });
		}).catch(poll);
	}, 2000);
})();
</script>`

// progress is the running step, how long it has run and, judged from the
// scans before, how long it should take.
func (a *App) progress(ctx context.Context, b *batch.Scan, step string, since time.Time) string {
	label := "Starting"
	if step != "" {
		label = strings.ToUpper(step[:1]) + step[1:]
	}
	elapsed := time.Since(since).Round(time.Second)
	want, ok := batch.Expected(ctx, a.Store, a.Staging, b, step)
	if !ok || want <= 0 {
		return fmt.Sprintf(`<div class="notification is-info">Working. %s: %s so far.</div><progress class="progress is-info" max="100"></progress>`,
			label, elapsed)
	}
	want = max(want.Round(time.Second), time.Second)
	return fmt.Sprintf(`<div class="notification is-info">Working. %s: %s of about %s.</div><progress class="progress is-info" value="%d" max="%d"></progress>`,
		label, elapsed, want, min(int(elapsed.Seconds()), int(want.Seconds())), int(want.Seconds()))
}
