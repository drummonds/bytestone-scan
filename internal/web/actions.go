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

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
	"git.bytestone.uk/hum3/bytestone-scan/internal/register"
)

// worker runs one batch step at a time in the background: there is one
// scanner.
type worker struct {
	mu      sync.Mutex
	batch   string // the batch a step is running for; empty when idle
	started time.Time
	running sync.WaitGroup
}

var errBusy = errors.New("busy")

// start runs prepare and then the batch's workflow up to its next wait, in
// the background, unless a step is already running.
func (a *App) start(b *batch.Batch, prepare func() error) error {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	if a.work.batch != "" {
		return fmt.Errorf("%w: the scanner is busy with batch %s", errBusy, a.work.batch)
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	a.work.batch, a.work.started = b.ID, time.Now()
	a.work.running.Go(func() {
		// a failure is recorded on the run, where the batch page reads it
		b.Run(context.Background(), wf.NewRunner(a.Store, nil), a.Scanner, a.Now)
		a.work.mu.Lock()
		a.work.batch = ""
		a.work.mu.Unlock()
	})
	return nil
}

// wait blocks until no step is running.
func (a *App) wait() { a.work.running.Wait() }

// running is the batch a step is running for, or empty.
func (a *App) running() string {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	return a.work.batch
}

func (a *App) busyWith(id string) (bool, time.Time) {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	return a.work.batch == id, a.work.started
}

const newBatchForm = `<h1 class="title">New batch</h1>
%s
<form action="batches" method="post" class="box">
<div class="field"><label class="label">What is being scanned</label>
<div class="control"><input class="input" name="description" value="%s" required></div></div>
<div class="field"><label class="label">Home: the person whose collection the photos belong to</label>
<div class="control"><div class="select"><select name="person" required>%s</select></div></div></div>
<div class="field"><label class="label">Provenance: where the prints came from</label>
<div class="control"><input class="input" name="provenance" value="%s" placeholder="Aunt Jean’s shoebox, envelope marked 1950s"></div>
<p class="help">Copied into every photo. Leave it blank if unknown.</p></div>
<div class="field"><label class="label">Credit: the terms the photos are shown under</label>
<div class="control"><input class="input" name="credit" value="%s" placeholder="Drummond family collection, CC BY 4.0" required></div></div>
<div class="field"><label class="label">Scan at</label>
<div class="control"><div class="select"><select name="ppi">%s</select></div></div>
<p class="help">The detail pass scans at this ppi and is averaged down to each master’s ppi: 600, or 1200 for prints smaller than 6 × 4 inches. A higher scan ppi takes longer but lowers noise, and the master is the same size whichever you pick.</p>
<p class="help">600 ppi takes about 15 s for a 7 × 5 inch print, 1200 about 75 s, 2400 about 2½ minutes.</p></div>
<button class="button is-primary" type="submit">Scan the preview</button>
</form>`

// entered is what the New batch form holds, as typed.
type entered struct {
	description, person, provenance, credit string
	ppi                                     int
}

// newBatch is the New batch page: msg above the form, and the form holding
// what was entered. With no one in the register there is no form.
func (a *App) newBatch(msg string, e entered) string {
	people, err := a.People.People()
	if err != nil {
		return `<h1 class="title">New batch</h1><div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`
	}
	return fmt.Sprintf(newBatchForm, msg, html.EscapeString(e.description), personOptions(people, e.person),
		html.EscapeString(e.provenance), html.EscapeString(e.credit), ppiOptions(e.ppi))
}

// prefilled is a fresh form: provenance and credit are the latest
// batch's, since a shoebox is many loads of the bed.
func (a *App) prefilled() entered {
	e := entered{ppi: 1200}
	if latest, err := batch.Latest(a.Staging); err == nil && latest != nil {
		e.provenance, e.credit = latest.Provenance, latest.Credit
	}
	return e
}

func personOptions(people []register.Person, selected string) string {
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
		if id := a.running(); id != "" {
			fail(w, fmt.Errorf("%w: the scanner is busy with batch %s", errBusy, id), http.StatusConflict)
			return
		}
		e := entered{description: r.FormValue("description"), person: r.FormValue("person"),
			provenance: r.FormValue("provenance"), credit: r.FormValue("credit")}
		e.ppi, _ = strconv.Atoi(r.FormValue("ppi"))
		person, err := pid.Parse(e.person)
		if e.person == "" {
			err = errors.New("choose the home person")
		}
		var b *batch.Batch
		if err == nil {
			b, err = batch.Describe(a.Staging, a.People, batch.Description{
				Text: e.description, Person: person, ScanPPI: e.ppi, Provenance: e.provenance, Credit: e.credit}, a.Now())
		}
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			render(w, "New batch", a.newBatch(`<div class="notification is-danger">`+html.EscapeString(err.Error())+`</div>`, e))
			return
		}
		if err := a.start(b, nil); err != nil {
			fail(w, err, http.StatusConflict)
			return
		}
		http.Redirect(w, r, "/batches/"+b.ID, http.StatusSeeOther)
	})

	// Each action prepares the batch, then runs it to its next wait.
	act := func(name string, prepare func(r *http.Request, b *batch.Batch) error) {
		mux.HandleFunc("POST /batches/{id}/"+name, func(w http.ResponseWriter, r *http.Request) {
			b, err := batch.Open(a.Staging, r.PathValue("id"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if err := a.start(b, func() error { return prepare(r, b) }); err != nil {
				fail(w, err, http.StatusUnprocessableEntity)
				return
			}
			http.Redirect(w, r, "/batches/"+b.ID, http.StatusSeeOther)
		})
	}
	runner := func() *wf.Runner { return wf.NewRunner(a.Store, nil) }
	act("preview", func(r *http.Request, b *batch.Batch) error {
		return b.Redo(r.Context(), runner(), "preview", 0)
	})
	act("prints", func(r *http.Request, b *batch.Batch) error {
		return b.Redo(r.Context(), runner(), "confirm", 0)
	})
	act("confirm", func(r *http.Request, b *batch.Batch) error { return b.Confirm() })
	act("rescan", func(r *http.Request, b *batch.Batch) error {
		ppi, _ := strconv.Atoi(r.FormValue("ppi"))
		return b.Redo(r.Context(), runner(), "detail pass", ppi)
	})
	act("accept", func(r *http.Request, b *batch.Batch) error { return b.Accept() })
	act("retry", func(r *http.Request, b *batch.Batch) error { return nil })
	act("reshape", func(r *http.Request, b *batch.Batch) error {
		n, edge, by, err := parseMove(r.FormValue("move"))
		if err != nil {
			return err
		}
		return b.Reshape(r.Context(), runner(), n-1, edge, by)
	})

	// Editing the regions needs no scanner, only that no step is running on
	// the batch. Each edit names its print, counted from 1.
	edit := func(name string, change func(r *http.Request, b *batch.Batch) (int, error)) {
		mux.HandleFunc("POST /batches/{id}/"+name, func(w http.ResponseWriter, r *http.Request) {
			b, err := batch.Open(a.Staging, r.PathValue("id"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if a.running() == b.ID {
				fail(w, fmt.Errorf("%w: wait for batch %s's step to finish", errBusy, b.ID), http.StatusConflict)
				return
			}
			n, err := change(r, b)
			if err != nil {
				fail(w, err, http.StatusUnprocessableEntity)
				return
			}
			http.Redirect(w, r, fmt.Sprintf("/batches/%s#print-%d", b.ID, n), http.StatusSeeOther)
		})
	}
	edit("adjust", func(r *http.Request, b *batch.Batch) (int, error) {
		n, edge, by, err := parseMove(r.FormValue("move"))
		if err != nil {
			return 0, err
		}
		return n, b.Adjust(n-1, edge, by)
	})
	edit("split", func(r *http.Request, b *batch.Batch) (int, error) {
		n, err := strconv.Atoi(r.FormValue("print"))
		if err != nil {
			return 0, fmt.Errorf("no print given")
		}
		return n, b.Split(n - 1)
	})
	edit("remove", func(r *http.Request, b *batch.Batch) (int, error) {
		n, err := strconv.Atoi(r.FormValue("print"))
		if err != nil {
			return 0, fmt.Errorf("no print given")
		}
		return 1, b.Remove(n - 1)
	})
}

// parseMove reads an arrow's "<print> <edge> <mm>", mm outwards when positive.
func parseMove(s string) (int, find.Edge, float64, error) {
	f := strings.Fields(s)
	if len(f) != 3 {
		return 0, 0, 0, fmt.Errorf("bad move %q", s)
	}
	n, err := strconv.Atoi(f[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("bad move %q", s)
	}
	edge, err := find.ParseEdge(f[1])
	if err != nil {
		return 0, 0, 0, err
	}
	by, err := strconv.ParseFloat(f[2], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("bad move %q", s)
	}
	return n, edge, by, nil
}

// state writes where the batch's workflow stands and the operator's
// choices there, reporting whether a step is running and the step the
// batch is waiting at, if any.
func (a *App) state(r *http.Request, c *lofigui.Context, b *batch.Batch) (bool, string) {
	if busy, since := a.busyWith(b.ID); busy {
		c.HTML(fmt.Sprintf(`<div class="notification is-info">Working: the scanner has been busy for %s. This page refreshes itself.</div>`,
			time.Since(since).Round(time.Second)))
		return true, ""
	}
	run, waiting := a.where(r.Context(), b)
	if run == nil {
		c.HTML(`<div class="notification">Not started.</div>`)
		return false, ""
	}
	button := func(action, label, class string) string {
		return fmt.Sprintf(`<form action="batches/%s/%s" method="post" style="display:inline"><button class="button %s" type="submit">%s</button></form> `,
			html.EscapeString(b.ID), action, class, label)
	}
	switch {
	case run.Status == wf.StatusFailed:
		c.HTML(`<div class="notification is-danger">Failed: ` + html.EscapeString(run.Error) + `</div>` +
			`<div class="buttons">` + button("retry", "Retry", "is-primary") + `</div>`)
	case run.Status == wf.StatusCompleted:
		c.HTML(`<div class="notification is-success">Done: every photo is archived.</div>`)
	case waiting == "confirm":
		c.HTML(`<div class="notification is-warning">Check the prints found on the preview.</div><div class="buttons">` +
			button("confirm", "Confirm and scan", "is-primary") + button("preview", "Rescan the preview", "") + `</div>`)
	case waiting == "check quality":
		c.HTML(`<div class="notification is-warning">Check each photo's quality, then accept or send the batch back.</div><div class="buttons">` +
			button("accept", "Accept and archive", "is-primary") +
			fmt.Sprintf(`<form action="batches/%s/rescan" method="post" style="display:inline"><div class="select"><select name="ppi">%s</select></div> <button class="button" type="submit">Rescan at</button></form> `,
				html.EscapeString(b.ID), ppiOptions(b.ScanPPI)) +
			button("prints", "Back to the prints", "") + `</div>`)
	default:
		c.HTML(`<div class="notification">` + html.EscapeString(string(run.Status)) + `</div>` +
			`<div class="buttons">` + button("retry", "Carry on", "") + `</div>`)
	}
	return false, waiting
}

// where is the batch's run and the step it waits at, if it is waiting.
func (a *App) where(ctx context.Context, b *batch.Batch) (*wf.RunRecord, string) {
	run, err := a.Store.FindRunByKey(ctx, batch.Workflow, b.ID)
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
