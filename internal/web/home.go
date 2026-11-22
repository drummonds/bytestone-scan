package web

import (
	"errors"
	"fmt"
	"html"
	"net/http"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// homeText is a home person as the pages show them. 0 is no id (#191):
// a batch or a record from before names nobody by it.
func homeText(name string, id pid.ID) string {
	if id == 0 {
		return "0, no longer an id"
	}
	return html.EscapeString(fmt.Sprintf("%s (%s)", name, id))
}

// homeNow is an item's home for its page: once the archive holds it, the
// home the archive gives now, with what it was deposited under when that
// was another; the home as sent, marked as such, when the archive cannot
// be asked. Before the deposit it is the record's.
func (a *App) homeNow(id pid.ID, sent batch.Person) string {
	d, err := batch.DepositOf(a.Staging, id)
	if err != nil || d == nil || d.Status == "duplicate" {
		return homeText(sent.Name, sent.ID)
	}
	rec, err := a.Archive.Receipt(id)
	if err != nil {
		return homeText(sent.Name, sent.ID) + ", as sent: the archive could not be asked"
	}
	if rec.Home == d.Home {
		return homeText(sent.Name, sent.ID)
	}
	name, err := a.Archive.Person(rec.Home)
	if err != nil {
		name = "person " + rec.Home.String()
	}
	return homeText(name, rec.Home) + ", " + depositedUnder(d.Home)
}

// depositedUnder says which home the archive's answer named at the time.
func depositedUnder(sent pid.ID) string {
	return fmt.Sprintf("<em>deposited under %s</em>", sent)
}

// rehomeLink is the way to the batch's Re-home form.
func rehomeLink(b *batch.Batch) string {
	return fmt.Sprintf(`<a class="button is-primary" href="batches/%s/rehome">Re-home</a>`, html.EscapeString(b.ID))
}

// homed reports whether the batch may begin a scan, import, feed or film
// import; when its person is gone it writes the refusal, which offers
// Re-home, and nothing is begun.
func (a *App) homed(w http.ResponseWriter, b *batch.Batch, render func(http.ResponseWriter, string, string)) bool {
	err := b.Homed(a.Archive)
	if err == nil {
		return true
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	render(w, "Not done", danger(err)+`<div class="buttons">`+rehomeLink(b)+`</div>`)
	return false
}

// rehomeForm is the Re-home page: the person picker, under msg.
func (a *App) rehomeForm(b *batch.Batch, msg, selected string) string {
	people, err := a.Archive.People()
	if err != nil {
		return `<h1 class="title">Re-home batch ` + html.EscapeString(b.ID) + `</h1>` + danger(err)
	}
	return fmt.Sprintf(`<h1 class="title">Re-home batch %s</h1>
<p class="subtitle">%s</p>
%s
<form action="batches/%s/rehome" method="post" class="box">
<div class="field"><label class="label">Home: the person whose collection the batch belongs to</label>
<div class="control"><div class="select"><select name="person" required>%s</select></div></div>
<p class="help">The batch names %s, whom the register no longer holds. The batch will name the person chosen; what is packaged and not yet deposited is packaged again with that home, and what is deposited is left as it was sent.</p></div>
<button class="button is-primary" type="submit">Re-home</button> <a class="button" href="batches/%s">Cancel</a>
</form>`, html.EscapeString(b.ID), html.EscapeString(b.Title), msg, html.EscapeString(b.ID),
		personOptions(people, selected), homeText(b.PersonName, b.Person), html.EscapeString(b.ID))
}

// rehomeRoutes are Re-home's form and action, offered for an open batch
// whose person the register does not hold.
func (a *App) rehomeRoutes(mux *http.ServeMux, render func(http.ResponseWriter, string, string), fail func(http.ResponseWriter, error, int)) {
	open := func(w http.ResponseWriter, r *http.Request) (*batch.Batch, bool) {
		b, err := batch.Open(a.Staging, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return nil, false
		}
		if b.Finished() {
			fail(w, fmt.Errorf("batch %s: scanning is finished, so nothing more will be deposited from it and it keeps the home it had", b.ID), http.StatusUnprocessableEntity)
			return nil, false
		}
		if err := b.Homed(a.Archive); err == nil {
			fail(w, fmt.Errorf("batch %s: person %s is in the register, and the batch keeps its home", b.ID, b.Person), http.StatusUnprocessableEntity)
			return nil, false
		}
		return b, true
	}
	mux.HandleFunc("GET /batches/{id}/rehome", func(w http.ResponseWriter, r *http.Request) {
		if b, ok := open(w, r); ok {
			render(w, "Re-home", a.rehomeForm(b, "", ""))
		}
	})
	mux.HandleFunc("POST /batches/{id}/rehome", func(w http.ResponseWriter, r *http.Request) {
		b, ok := open(w, r)
		if !ok {
			return
		}
		// not while a step is writing the batch's records
		if key := a.running(); key != "" {
			fail(w, fmt.Errorf("%w: the scanner is busy with scan %s", errBusy, key), http.StatusConflict)
			return
		}
		chosen := r.FormValue("person")
		person, err := pid.Parse(chosen)
		if chosen == "" {
			err = errors.New("choose the home person")
		}
		if err == nil {
			err = b.Rehome(a.Archive, person)
		}
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			render(w, "Re-home", a.rehomeForm(b, danger(err), chosen))
			return
		}
		http.Redirect(w, r, "/batches/"+b.ID, http.StatusSeeOther)
	})
}

// batchHome is the batch's home for the list of batches: the person's
// name, or that it names an id that is gone.
func batchHome(b *batch.Batch) string {
	if b.Person == 0 {
		return homeText("", 0)
	}
	return html.EscapeString(b.PersonName)
}
