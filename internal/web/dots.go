package web

import (
	"context"
	"fmt"
	"html"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

// runDots is the run of typ and key as the engine's row of dots, one per
// step, linking to the instance; empty before the run exists.
func (a *App) runDots(ctx context.Context, typ wf.WorkflowType, key string) string {
	run, err := a.Store.FindRunByKey(ctx, typ, key)
	if err != nil || run == nil {
		return ""
	}
	steps, _ := a.Store.ListSteps(ctx, run.ID)
	return string(a.views.Progress(*run, steps))
}

// scanDots is a scan as a row of dots titled with its Standing: a dot per
// photo, coloured by the photo's run and linking to the photo, or, before
// the scan has photos, one dot for the scan's own run, linking to the scan.
func (a *App) scanDots(ctx context.Context, l *batch.Scan) string {
	title := "not started"
	if busy, step, _ := a.busyWith(l.Key()); busy {
		title = "working: " + step
	} else if s, err := l.Standing(ctx, a.Store); err == nil {
		title = s.String()
	}
	_, ids, _ := l.Found()
	if len(ids) == 0 {
		run, _ := a.Store.FindRunByKey(ctx, batch.ScanWorkflow, l.Key())
		return fmt.Sprintf(`<a class="dots" href="%s" title="%s">%s</a>`, scanURL(l.Key()), html.EscapeString(title), dot(runColour(run)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<span class="dots" title="%s">`, html.EscapeString(title))
	for _, id := range ids {
		run, _ := a.Store.FindRunByKey(ctx, batch.PhotoWorkflow, id.String())
		status := "not started"
		if run != nil {
			status = string(run.Status)
		}
		fmt.Fprintf(&b, `<a href="photos/%s" title="photo %s: %s">%s</a>`, id, id, status, dot(runColour(run)))
	}
	b.WriteString(`</span>`)
	return b.String()
}

// dot is one dot of a progress row, as the engine draws its steps.
func dot(colour string) string {
	return `<span class="dot" style="display:inline-block;width:.75em;height:.75em;margin-right:.2em;border-radius:50%;border:1px solid #7a7a7a;background:` + colour + `"></span>`
}

// runColour is a run's dot colour, the engine's palette: green
// completed, red failed, amber waiting or running, yellow cancelled,
// white not started.
func runColour(run *wf.RunRecord) string {
	if run == nil {
		return "#ffffff"
	}
	switch run.Status {
	case wf.StatusCompleted:
		return "#48c78e"
	case wf.StatusFailed:
		return "#f14668"
	case wf.StatusCancelled:
		return "#ffe08a"
	case wf.StatusWaiting, wf.StatusRunning:
		return "#ffb000"
	}
	return "#ffffff"
}
