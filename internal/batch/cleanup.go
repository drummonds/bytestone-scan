package batch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Cleanup is what Clean up's Delete below would do from one row of the
// batches, newest first: delete every closed or purged batch below it,
// each as Delete does, leaving the open ones, and remove the orphans.
type Cleanup struct {
	Below   *Batch   // the row chosen; nil when only the orphans are to go
	Batches []*Batch // the completed batches below it, newest first
	Kept    []*Batch // the open batches below it, left alone
	Orphans []Orphan
	root    string
}

// An Orphan is what no readable batch claims: a bag, the archive's
// answer or a renditions directory under staging, whatever the archive
// answered, or a run in the workflow database whose batch has no
// directory. A deletion that failed part way, or a batch moved by hand,
// leaves them.
type Orphan struct {
	Path string        // relative to the staging root, for a file or directory
	Run  *wf.RunRecord // for a run
}

func (o Orphan) String() string {
	switch {
	case o.Run == nil:
		return o.Path
	case o.Run.WorkflowType == PhotoWorkflow:
		return "run photo " + o.Run.Key
	default:
		return "run " + o.Run.Key
	}
}

// PlanCleanup is the clean-up from the batch below, or the orphans alone
// when below is empty.
func PlanCleanup(ctx context.Context, root string, store wf.Store, below string) (*Cleanup, error) {
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	c := &Cleanup{root: root}
	if below != "" {
		i := 0
		for i < len(all) && all[i].ID != below {
			i++
		}
		if i == len(all) {
			return nil, fmt.Errorf("batch %s is not there", below)
		}
		c.Below = all[i]
		for _, b := range all[i+1:] {
			if b.Completed() {
				c.Batches = append(c.Batches, b)
			} else {
				c.Kept = append(c.Kept, b)
			}
		}
	}
	c.Orphans, err = orphans(ctx, root, store, all)
	return c, err
}

// Completed reports whether the batch is closed or purged: its scanning
// done and every photo answered for by the archive.
func (b *Batch) Completed() bool {
	if _, ok := b.Purged(); ok {
		return true
	}
	_, ok := b.Closed()
	return ok
}

// orphans finds what none of the readable batches claims: under bags/
// and renditions/, then in the workflow database.
func orphans(ctx context.Context, root string, store wf.Store, all []*Batch) ([]Orphan, error) {
	claimed := map[pid.ID]bool{}
	dirs := map[string]bool{}
	for _, b := range all {
		dirs[b.ID] = true
		ids, err := b.photos()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			claimed[id] = true
		}
	}
	var found []Orphan
	for _, kind := range []string{"bags", "renditions"} {
		entries, err := os.ReadDir(filepath.Join(root, kind))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, e := range entries {
			name, _, _ := strings.Cut(e.Name(), ".")
			if id, err := pid.Parse(name); err != nil || claimed[id] {
				continue
			}
			found = append(found, Orphan{Path: kind + "/" + e.Name()})
		}
	}
	for _, t := range []wf.WorkflowType{Workflow, ScanWorkflow, PhotoWorkflow} {
		runs, err := store.ListRuns(ctx, wf.RunFilter{Types: []wf.WorkflowType{t}})
		if err != nil {
			return nil, err
		}
		for i := range runs {
			r := &runs[i]
			var orphaned bool
			switch t {
			case PhotoWorkflow:
				id, err := pid.Parse(r.Key)
				orphaned = err == nil && !claimed[id]
			default:
				batch, _, _ := strings.Cut(r.Key, "/")
				orphaned = !dirs[batch]
			}
			if orphaned {
				found = append(found, Orphan{Run: r})
			}
		}
	}
	return found, nil
}

// Run does the clean-up: the batches, then the orphans.
func (c *Cleanup) Run(ctx context.Context, store wf.Store) error {
	for _, b := range c.Batches {
		if err := b.Delete(ctx, store); err != nil {
			return err
		}
	}
	for _, o := range c.Orphans {
		if o.Run != nil {
			if err := store.DeleteRun(ctx, o.Run.ID); err != nil {
				return err
			}
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.root, filepath.FromSlash(o.Path))); err != nil {
			return err
		}
	}
	return nil
}
