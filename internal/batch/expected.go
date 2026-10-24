package batch

import (
	"context"
	"slices"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// Expected is how long step should take for load b, judged from the loads
// run before it: the detail pass from loads scanned at the same ppi, by
// the area each scanned; any other step from how long it last took. The
// median of the last few is taken. ok is false with nothing to judge from.
func Expected(ctx context.Context, store wf.Store, root string, b *Load, step string) (d time.Duration, ok bool) {
	runs, err := store.ListRuns(ctx, wf.RunFilter{Types: []wf.WorkflowType{LoadWorkflow}, Limit: 20})
	if err != nil {
		return 0, false
	}
	var area float64 // mm² this batch's detail pass covers; 0 for any other step
	if step == "detail pass" {
		p, _, err := b.pass()
		if err != nil {
			return 0, false
		}
		area = size(p.Area)
	}
	var samples []float64 // seconds, or seconds per mm² for the detail pass
	for _, r := range runs {
		steps, err := store.ListSteps(ctx, r.ID)
		if err != nil {
			continue
		}
		for _, s := range steps {
			if s.StepName != step || s.Status != wf.StatusCompleted {
				continue
			}
			v := time.Duration(s.DurationNs).Seconds()
			if area > 0 {
				past, err := OpenLoad(root, r.Key)
				if err != nil || past.batch.ScanPPI != b.batch.ScanPPI {
					continue
				}
				p, _, err := past.pass()
				if err != nil {
					continue
				}
				v /= size(p.Area)
			}
			samples = append(samples, v)
		}
		if len(samples) >= 5 {
			break
		}
	}
	if len(samples) == 0 {
		return 0, false
	}
	v := median(samples)
	if area > 0 {
		v *= area
	}
	return time.Duration(v * float64(time.Second)), true
}

func size(r find.Rect) float64 { return (r.X1 - r.X0) * (r.Y1 - r.Y0) }

func median(v []float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	if n := len(s); n%2 == 1 {
		return s[n/2]
	} else {
		return (s[n/2-1] + s[n/2]) / 2
	}
}
