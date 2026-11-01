package batch

import (
	"context"
	"fmt"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Standing is where a scan stands in its batch: its own run's status
// until that run is through, then the status of the photo that holds the
// scan back, if any, since a scan is through only once every photo is
// held by the archive or rejected by it. Step is the step the status is
// at, Error why it failed, and Photo the photo whose run it is.
type Standing struct {
	Status wf.RunStatus // empty before the scan's run exists
	Step   string
	Error  string
	Photo  *pid.ID
}

// Through is whether the scan and every photo of it are through.
func (s Standing) Through() bool { return s.Status == wf.StatusCompleted }

// String says the standing as the operator reads it: "not started",
// "waiting at confirm", "photo 16 failed at deposit: refused …", "through".
func (s Standing) String() string {
	who := ""
	if s.Photo != nil {
		who = "photo " + s.Photo.String() + " "
	}
	at := ""
	if s.Step != "" {
		at = " at " + s.Step
	}
	switch s.Status {
	case "":
		return "not started"
	case wf.StatusCompleted:
		return "through"
	case wf.StatusFailed:
		if s.Error != "" {
			return who + "failed" + at + ": " + s.Error
		}
		return who + "failed" + at
	case wf.StatusCancelled:
		return who + "cancelled"
	}
	return who + string(s.Status) + at
}

// rank orders standings by how much they hold the scan back: a failure
// outranks a cancellation, which outranks work still going on.
func (s Standing) rank() int {
	switch s.Status {
	case wf.StatusFailed:
		return 5
	case wf.StatusCancelled:
		return 4
	case wf.StatusRunning:
		return 3
	case wf.StatusWaiting:
		return 2
	case wf.StatusPending:
		return 1
	}
	return 0
}

// Standing is where the scan stands: its run's status, or, once its run
// is through, that of the photo furthest from being held.
func (l *Scan) Standing(ctx context.Context, store wf.Store) (Standing, error) {
	s, err := standingOf(ctx, store, ScanWorkflow, l.Key())
	if err != nil || !s.Through() {
		return s, err
	}
	_, ids, err := l.Found()
	if err != nil {
		return Standing{}, err
	}
	for _, id := range ids {
		p, err := standingOf(ctx, store, PhotoWorkflow, id.String())
		if err != nil {
			return Standing{}, err
		}
		if p.Status == "" {
			p.Status = wf.StatusPending
		}
		if p.rank() > s.rank() {
			id := id
			p.Photo = &id
			s = p
		}
	}
	return s, nil
}

// standingOf is the standing of one run: its status, the step it is at
// when not through, and its error.
func standingOf(ctx context.Context, store wf.Store, typ wf.WorkflowType, key string) (Standing, error) {
	run, err := store.FindRunByKey(ctx, typ, key)
	if err != nil {
		return Standing{}, fmt.Errorf("%s %s: %w", typ, key, err)
	}
	if run == nil {
		return Standing{}, nil
	}
	s := Standing{Status: run.Status, Error: run.Error}
	if s.Through() {
		return s, nil
	}
	steps, err := store.ListSteps(ctx, run.ID)
	if err != nil {
		return Standing{}, fmt.Errorf("%s %s: %w", typ, key, err)
	}
	for _, step := range steps {
		if step.Status != wf.StatusCompleted {
			s.Step = step.StepName
		}
	}
	return s, nil
}

// PhotoStanding is where one photo's own run stands.
func PhotoStanding(ctx context.Context, store wf.Store, id pid.ID) (Standing, error) {
	return standingOf(ctx, store, PhotoWorkflow, id.String())
}
