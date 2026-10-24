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

// worker runs one load's step at a time in the background: there is one
// scanner.
type worker struct {
	mu      sync.Mutex
	load    string // the key of the load a step is running for; empty when idle
	started time.Time
	step    string // the workflow step running, once one has started
	stepAt  time.Time
	running sync.WaitGroup
}

var errBusy = errors.New("busy")

// start runs prepare and then the load's workflow up to its next wait, in
// the background, unless a step is already running.
func (a *App) start(l *batch.Load, prepare func() error) error {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	if a.work.load != "" {
		return fmt.Errorf("%w: the scanner is busy with load %s", errBusy, a.work.load)
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	a.work.load, a.work.started = l.Key(), time.Now()
	a.work.step, a.work.stepAt = "", a.work.started
	a.work.running.Go(func() {
		// a failure is recorded on the run, where the load page reads it
		l.Run(context.Background(), a.Store, a.Scanner, a.Now, func(step string) {
			a.work.mu.Lock()
			a.work.step, a.work.stepAt = step, time.Now()
			a.work.mu.Unlock()
		})
		a.work.mu.Lock()
		a.work.load = ""
		a.work.mu.Unlock()
	})
	return nil
}

// wait blocks until no step is running.
func (a *App) wait() { a.work.running.Wait() }

// running is the key of the load a step is running for, or empty.
func (a *App) running() string {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	return a.work.load
}

// busyWith reports whether a step of the load with key is running, and if
// so which workflow step it is at (empty before the first starts) and
// since when.
func (a *App) busyWith(key string) (bool, string, time.Time) {
	a.work.mu.Lock()
	defer a.work.mu.Unlock()
	return a.work.load == key, a.work.step, a.work.stepAt
}

const newBatchForm = `<h1 class="title">New batch</h1>
%s
<form action="batches" method="post" class="box">
<div class="field"><label class="label">Title: what is being scanned</label>
<div class="control"><input class="input" name="title" value="%s" required></div>
<p class="help">A batch is one sitting: as many loads of the bed as the shoebox needs. Each load’s description starts as this title.</p></div>
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
<p class="help">The detail pass scans at this ppi and is averaged down to each master’s ppi: 600, or 1200 for prints smaller than 6 × 4 inches. A higher scan ppi takes longer but lowers noise, and the master is the same size whichever you pick.</p>
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

const newLoadForm = `<h1 class="title">Batch %s: another load</h1>
<p class="subtitle">%s</p>
%s
<form action="batches/%s/loads" method="post" class="box">
<div class="field"><label class="label">Description: what is on the bed this time</label>
<div class="control"><input class="input" name="description" value="%s" required></div>
<p class="help">Copied into each of this load’s photos. Home, provenance, credit and scan ppi are the batch’s.</p></div>
<button class="button is-primary" type="submit">Scan the preview</button>
</form>`

// newLoad is the form for a batch's next load, its description starting
// as the batch's title.
func newLoad(b *batch.Batch, msg, description string) string {
	return fmt.Sprintf(newLoadForm, html.EscapeString(b.ID), html.EscapeString(b.Title), msg, html.EscapeString(b.ID), html.EscapeString(description))
}

// newBatch is the New batch page: msg above the form, and the form holding
// what was entered. With no one in the register there is no form.
func (a *App) newBatch(msg string, e entered) string {
	people, err := a.People.People()
	if err != nil {
		return `<h1 class="title">New batch</h1><div class="notification is-danger">` + html.EscapeString(err.Error()) + `</div>`
	}
	return fmt.Sprintf(newBatchForm, msg, html.EscapeString(e.title), personOptions(people, e.person),
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
		if key := a.running(); key != "" {
			fail(w, fmt.Errorf("%w: the scanner is busy with load %s", errBusy, key), http.StatusConflict)
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
			b, err = batch.Describe(a.Staging, a.People, batch.Description{
				Text: e.title, Person: person, ScanPPI: e.ppi, Provenance: e.provenance, Credit: e.credit}, a.Now())
		}
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			render(w, "New batch", a.newBatch(`<div class="notification is-danger">`+html.EscapeString(err.Error())+`</div>`, e))
			return
		}
		a.startLoad(w, r, b, "", fail)
	})
	mux.HandleFunc("GET /batches/{id}/loads/new", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if b.Finished() {
			fail(w, fmt.Errorf("batch %s: scanning is finished", b.ID), http.StatusUnprocessableEntity)
			return
		}
		render(w, "Another load", newLoad(b, "", b.Title))
	})
	mux.HandleFunc("POST /batches/{id}/loads", func(w http.ResponseWriter, r *http.Request) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if key := a.running(); key != "" {
			fail(w, fmt.Errorf("%w: the scanner is busy with load %s", errBusy, key), http.StatusConflict)
			return
		}
		a.startLoad(w, r, b, r.FormValue("description"), fail)
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

	// Each action prepares the load, then runs it to its next wait.
	act := func(name string, prepare func(r *http.Request, l *batch.Load) error) {
		mux.HandleFunc("POST /batches/{id}/loads/{n}/"+name, func(w http.ResponseWriter, r *http.Request) {
			l, err := batch.OpenLoad(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if err := a.start(l, func() error { return prepare(r, l) }); err != nil {
				fail(w, err, http.StatusUnprocessableEntity)
				return
			}
			http.Redirect(w, r, "/"+loadURL(l.Key()), http.StatusSeeOther)
		})
	}
	runner := func() *wf.Runner { return wf.NewRunner(a.Store, nil) }
	act("preview", func(r *http.Request, l *batch.Load) error {
		return l.Redo(r.Context(), runner(), "preview", 0)
	})
	act("prints", func(r *http.Request, l *batch.Load) error {
		return l.Redo(r.Context(), runner(), "confirm", 0)
	})
	act("confirm", func(r *http.Request, l *batch.Load) error { return l.Confirm() })
	act("rescan", func(r *http.Request, l *batch.Load) error {
		ppi, _ := strconv.Atoi(r.FormValue("ppi"))
		return l.Redo(r.Context(), runner(), "detail pass", ppi)
	})
	act("accept", func(r *http.Request, l *batch.Load) error { return l.Accept() })
	act("retry", func(r *http.Request, l *batch.Load) error { return nil })
	act("reshape", func(r *http.Request, l *batch.Load) error {
		n, edge, by, err := parseMove(r.FormValue("move"))
		if err != nil {
			return err
		}
		return l.Reshape(r.Context(), runner(), n-1, edge, by)
	})

	// Editing the regions needs no scanner, only that no step is running on
	// the load. Each edit names its print, counted from 1.
	edit := func(name string, change func(r *http.Request, l *batch.Load) (int, error)) {
		mux.HandleFunc("POST /batches/{id}/loads/{n}/"+name, func(w http.ResponseWriter, r *http.Request) {
			l, err := batch.OpenLoad(a.Staging, r.PathValue("id")+"/"+r.PathValue("n"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if a.running() == l.Key() {
				fail(w, fmt.Errorf("%w: wait for load %s's step to finish", errBusy, l.Key()), http.StatusConflict)
				return
			}
			n, err := change(r, l)
			if err != nil {
				fail(w, err, http.StatusUnprocessableEntity)
				return
			}
			http.Redirect(w, r, fmt.Sprintf("/%s#print-%d", loadURL(l.Key()), n), http.StatusSeeOther)
		})
	}
	edit("adjust", func(r *http.Request, l *batch.Load) (int, error) {
		n, edge, by, err := parseMove(r.FormValue("move"))
		if err != nil {
			return 0, err
		}
		return n, l.Adjust(n-1, edge, by)
	})
	edit("split", func(r *http.Request, l *batch.Load) (int, error) {
		n, err := strconv.Atoi(r.FormValue("print"))
		if err != nil {
			return 0, fmt.Errorf("no print given")
		}
		return n, l.Split(n - 1)
	})
	edit("remove", func(r *http.Request, l *batch.Load) (int, error) {
		n, err := strconv.Atoi(r.FormValue("print"))
		if err != nil {
			return 0, fmt.Errorf("no print given")
		}
		return 1, l.Remove(n - 1)
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

// startLoad adds a load to the batch and starts its preview, landing on
// its page.
func (a *App) startLoad(w http.ResponseWriter, r *http.Request, b *batch.Batch, description string, fail func(http.ResponseWriter, error, int)) {
	l, err := b.NewLoad(description, a.Now())
	if err != nil {
		fail(w, err, http.StatusUnprocessableEntity)
		return
	}
	if err := a.start(l, nil); err != nil {
		fail(w, err, http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/"+loadURL(l.Key()), http.StatusSeeOther)
}

// state writes where the load's workflow stands and the operator's
// choices there, reporting whether a step is running and the step the
// load is waiting at, if any.
func (a *App) state(r *http.Request, c *lofigui.Context, l *batch.Load) (bool, string) {
	if busy, step, since := a.busyWith(l.Key()); busy {
		c.HTML(`<div id="progress">` + a.progress(r.Context(), l, step, since) + `</div>` + fmt.Sprintf(pollProgress, loadURL(l.Key())))
		return true, ""
	}
	run, waiting := a.where(r.Context(), l)
	if run == nil {
		c.HTML(`<div class="notification">Not started.</div>`)
		return false, ""
	}
	button := func(action, label, class string) string {
		return fmt.Sprintf(`<form action="%s/%s" method="post" style="display:inline"><button class="button %s" type="submit">%s</button></form> `,
			loadURL(l.Key()), action, class, label)
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
		c.HTML(`<div class="notification is-warning">Check each photo's quality, then accept or send the load back.</div><div class="buttons">` +
			button("accept", "Accept and archive", "is-primary") +
			fmt.Sprintf(`<form action="%s/rescan" method="post" style="display:inline"><div class="select"><select name="ppi">%s</select></div> <button class="button" type="submit">Rescan at</button></form> `,
				loadURL(l.Key()), ppiOptions(l.Sitting().ScanPPI)) +
			button("prints", "Back to the prints", "") + `</div>`)
	default:
		c.HTML(`<div class="notification">` + html.EscapeString(string(run.Status)) + `</div>` +
			`<div class="buttons">` + button("retry", "Carry on", "") + `</div>`)
	}
	return false, waiting
}

// where is the load's run and the step it waits at, if it is waiting.
func (a *App) where(ctx context.Context, l *batch.Load) (*wf.RunRecord, string) {
	run, err := a.Store.FindRunByKey(ctx, batch.LoadWorkflow, l.Key())
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
// load is busy, and the page once it is not.
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
// loads before, how long it should take.
func (a *App) progress(ctx context.Context, b *batch.Load, step string, since time.Time) string {
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
