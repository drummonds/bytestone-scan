package batch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Delete removes a batch, open, closed or purged: its run and its scans'
// and photos' runs go from the workflow database with their steps, and
// its directory and its photos' bags, renditions and the archive's
// answers go from staging. Nothing the archive holds is touched: a held
// photo stays held, only staging's copy goes. A photo not yet deposited
// loses its masters, which the operator is told before confirming. The
// batch's id is left on record so it is not given out again.
func (b *Batch) Delete(ctx context.Context, store wf.Store) error {
	ids, err := b.photos()
	if err != nil {
		return err
	}
	scans, err := b.Scans()
	if err != nil {
		return err
	}
	remove := func(t wf.WorkflowType, key string) error {
		r, err := store.FindRunByKey(ctx, t, key)
		if err != nil || r == nil {
			return err
		}
		return store.DeleteRun(ctx, r.ID)
	}
	for _, id := range ids {
		if err := remove(PhotoWorkflow, id.String()); err != nil {
			return err
		}
	}
	for _, l := range scans {
		if err := remove(ScanWorkflow, l.Key()); err != nil {
			return err
		}
	}
	if err := remove(Workflow, b.ID); err != nil {
		return err
	}
	for _, id := range ids {
		for _, path := range []string{filepath.Join(b.root, "bags", id.String()), filepath.Join(b.root, "bags", id.String()+".deposit.yaml"), filepath.Join(b.root, "renditions", id.String())} {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}
	if err := os.RemoveAll(b.Dir()); err != nil {
		return err
	}
	return os.WriteFile(deletedMarker(b.root, b.ID), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

// deletedMarker is the record a deleted batch leaves beside where it was,
// so its id is not given out again.
func deletedMarker(root, id string) string { return filepath.Join(root, "batches", id+".deleted") }

// photos are the ids of every item of the batch's scans: each print
// found, and each import's page item.
func (b *Batch) photos() ([]pid.ID, error) {
	scans, err := b.Scans()
	if err != nil {
		return nil, err
	}
	var ids []pid.ID
	for _, l := range scans {
		found, err := l.items()
		if err != nil {
			return nil, err
		}
		ids = append(ids, found...)
	}
	return ids, nil
}

// Stale is something under staging that no readable batch claims: a batch
// directory in a layout the code cannot read, or an id directory under
// bags/ or renditions/ with no batch behind it and no answer from the
// archive on file. Path is relative to the staging root.
type Stale struct {
	Path string
	Why  string
}

// ListStale finds the stale data under root, batches first. A rejected
// photo's bag, kept on purpose once its batch is purged, is not stale:
// the archive's answer is on file beside it.
func ListStale(root string) ([]Stale, error) {
	entries, err := os.ReadDir(filepath.Join(root, "batches"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var stale []Stale
	claimed := map[pid.ID]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := Open(root, e.Name())
		if err != nil {
			stale = append(stale, Stale{Path: "batches/" + e.Name(), Why: "a batch in a layout this build does not read"})
			continue
		}
		ids, err := b.photos()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			claimed[id] = true
		}
	}
	for _, kind := range []string{"bags", "renditions"} {
		entries, err := os.ReadDir(filepath.Join(root, kind))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			id, err := pid.Parse(e.Name())
			if err != nil || claimed[id] {
				continue
			}
			if d, err := DepositOf(root, id); err != nil {
				return nil, err
			} else if d != nil {
				continue
			}
			stale = append(stale, Stale{Path: kind + "/" + e.Name(), Why: "no batch claims photo " + e.Name()})
		}
	}
	return stale, nil
}

// DeleteStale removes the stale item at path, relative to root, which
// must be one ListStale lists.
func DeleteStale(root, path string) error {
	stale, err := ListStale(root)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(stale, func(s Stale) bool { return s.Path == path }) {
		return fmt.Errorf("%s is not stale", path)
	}
	return os.RemoveAll(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path, "/"))))
}
