package web

import (
	"context"
	"fmt"
	"html"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// A batch, scan or photo page shows its run as the definition has it:
// the Steps heading naming the definition it is an instance of, with the
// way to the generic instance page; its parent above; each step with its
// status, its error and the action the operator has there; its children
// below. The generic instance page knows only the run; these sections
// know what the page is about, so they carry the controls.

// definitionOf is the scanner's definition of typ.
func definitionOf(typ wf.WorkflowType) *wf.Definition {
	for i := range batch.Definitions {
		if batch.Definitions[i].Type == typ {
			return &batch.Definitions[i]
		}
	}
	return nil
}

// runSteps is the run of typ and key, if any, with the latest record of
// each of its steps by name.
func (a *App) runSteps(ctx context.Context, typ wf.WorkflowType, key string) (*wf.RunRecord, map[string]wf.StepResult) {
	latest := map[string]wf.StepResult{}
	run, err := a.Store.FindRunByKey(ctx, typ, key)
	if err != nil || run == nil {
		return nil, latest
	}
	steps, _ := a.Store.ListSteps(ctx, run.ID)
	for _, s := range steps {
		latest[s.StepName] = s
	}
	return run, latest
}

// stepsHeading names the definition the page is an instance of, linking
// to it, with the instance tag beside.
func (a *App) stepsHeading(ctx context.Context, typ wf.WorkflowType, key string) string {
	name := string(typ)
	if def := definitionOf(typ); def != nil {
		name = def.Purpose.Name
	}
	return fmt.Sprintf(`<h2 class="title is-5">Steps of <a href="workflows/definitions/%s">%s</a> %s</h2>`,
		html.EscapeString(string(typ)), html.EscapeString(name), a.instanceLink(ctx, typ, key))
}

// stepsTable is the definition's steps in order, each with its status,
// its error and the action the page gives it: Back to the step, Retry,
// or a word on why there is none.
func (a *App) stepsTable(ctx context.Context, typ wf.WorkflowType, key string, action func(step string, rec wf.StepResult) string) string {
	def := definitionOf(typ)
	if def == nil {
		return ""
	}
	_, latest := a.runSteps(ctx, typ, key)
	var t strings.Builder
	t.WriteString(`<table class="table is-narrow"><thead><tr><th>Step</th><th>Status</th><th>Error</th><th></th></tr></thead><tbody>`)
	for _, step := range def.Steps {
		rec := latest[step.Name]
		fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(step.Name), statusTag(rec.Status), html.EscapeString(rec.Error), action(step.Name, rec))
	}
	t.WriteString(`</tbody></table>`)
	return t.String()
}

// standsAt is a run's step and status for a parent or child line: the
// step it is at, when it is at one, and its status tag.
func (a *App) standsAt(ctx context.Context, typ wf.WorkflowType, key string) (step string, tag string) {
	run, latest := a.runSteps(ctx, typ, key)
	if run == nil {
		return "", statusTag("")
	}
	if def := definitionOf(typ); def != nil && run.Status != wf.StatusCompleted {
		for _, s := range def.Steps {
			if latest[s.Name].Status != wf.StatusCompleted {
				step = s.Name
				break
			}
		}
	}
	return step, statusTag(run.Status)
}

// parentLine names the page's parent and where it stands.
func (a *App) parentLine(ctx context.Context, href, name string, typ wf.WorkflowType, key string) string {
	step, tag := a.standsAt(ctx, typ, key)
	at := ""
	if step != "" {
		at = " at " + html.EscapeString(step)
	}
	return fmt.Sprintf(`<p>Parent: <a href="%s">%s</a>%s %s</p>`, href, html.EscapeString(name), at, tag)
}

// photoRows lists a scan's photos, each where its run stands.
func (a *App) photoRows(ctx context.Context, ids []pid.ID) string {
	if len(ids) == 0 {
		return ""
	}
	var t strings.Builder
	t.WriteString(`<table class="table is-narrow"><thead><tr><th>Photo</th><th>Step</th><th>Status</th><th>Error</th></tr></thead><tbody>`)
	for _, id := range ids {
		step, tag := a.standsAt(ctx, batch.PhotoWorkflow, id.String())
		s, _ := batch.PhotoStanding(ctx, a.Store, id)
		errText := ""
		if s.Status == wf.StatusFailed {
			errText = s.Error
		}
		fmt.Fprintf(&t, `<tr><td><a href="photos/%s">photo %s</a></td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			id, id, html.EscapeString(step), tag, html.EscapeString(errText))
	}
	t.WriteString(`</tbody></table>`)
	return t.String()
}

// smallForm is a one-button POST to href.
func smallForm(href, label, class string) string {
	return fmt.Sprintf(`<form action="%s" method="post" style="display:inline"><button class="button is-small%s" type="submit">%s</button></form>`, href, class, label)
}

// scanAction is what the operator can do at one of the scan's steps:
// Retry a failed step; go back to one of the go-back steps while no
// photo is packaged, after which it says so.
func (a *App) scanAction(l *batch.Scan) func(step string, rec wf.StepResult) string {
	packaged := l.Packaged()
	return func(step string, rec wf.StepResult) string {
		switch {
		case rec.Status == wf.StatusFailed:
			return smallForm(scanURL(l.Key())+"/retry", "Retry", " is-primary")
		case rec.Status == wf.StatusCompleted && batch.GoBack(step) && packaged:
			return "archived and final"
		case rec.Status == wf.StatusCompleted && batch.GoBack(step):
			return fmt.Sprintf(`<form action="%s/redo" method="post" style="display:inline"><input type="hidden" name="step" value="%s"><button class="button is-small" type="submit">Back to %s</button></form>`,
				scanURL(l.Key()), html.EscapeString(step), html.EscapeString(step))
		}
		return ""
	}
}

// photoAction is what the operator can do at one of the photo's steps:
// Retry a failed step, which ticks its scan; nothing once it is final.
func photoAction(id pid.ID) func(step string, rec wf.StepResult) string {
	return func(step string, rec wf.StepResult) string {
		switch {
		case rec.Status == wf.StatusFailed:
			return smallForm("photos/"+id.String()+"/retry", "Retry", " is-primary")
		case rec.Status == wf.StatusCompleted && batch.Final(step):
			return "final"
		}
		return ""
	}
}

// noAction is a step with nothing for the operator to do on it.
func noAction(string, wf.StepResult) string { return "" }

// scanSteps is the scan page's steps section: its definition, its batch,
// its steps and its photos.
func (a *App) scanSteps(ctx context.Context, l *batch.Scan) string {
	_, ids, _ := l.Found()
	return a.stepsHeading(ctx, batch.ScanWorkflow, l.Key()) +
		a.parentLine(ctx, "batches/"+html.EscapeString(l.Batch), "batch "+l.Batch, batch.Workflow, l.Batch) +
		a.stepsTable(ctx, batch.ScanWorkflow, l.Key(), a.scanAction(l)) +
		a.photoRows(ctx, ids)
}

// photoSteps is the photo page's steps section: its definition, its
// scan and its steps.
func (a *App) photoSteps(ctx context.Context, id pid.ID, key string) string {
	return a.stepsHeading(ctx, batch.PhotoWorkflow, id.String()) +
		a.parentLine(ctx, scanURL(key), "scan "+key, batch.ScanWorkflow, key) +
		a.stepsTable(ctx, batch.PhotoWorkflow, id.String(), photoAction(id))
}
