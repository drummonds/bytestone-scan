package web

import (
	"context"

	wf "git.bytestone.uk/hum3/gobank-workflow"
	"git.bytestone.uk/hum3/gobank-workflow/view"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

// queue is the store as the Queue reads it: a list of every run is a list
// of batches, each batch's scans and photos being reached through its
// instance page. A list of one workflow's runs, or of a run's children,
// is as asked.
type queue struct{ view.Reader }

func (q queue) ListRuns(ctx context.Context, f wf.RunFilter) ([]wf.RunRecord, error) {
	if f.Types == nil && f.ParentID == "" {
		f.Types = []wf.WorkflowType{batch.Workflow}
	}
	return q.Reader.ListRuns(ctx, f)
}
