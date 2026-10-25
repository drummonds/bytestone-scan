package batch

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	wf "git.bytestone.uk/hum3/gobank-workflow"
)

// diagnosisDir is where kept diagnoses live under the staging root: beside
// the batches, where a purge and a batch delete leave them.
const diagnosisDir = "diagnosis"

// Diagnosis is a kept copy of what a scan had when the operator wanted it
// looked into: its work files and photo records, the annotated previews as
// rendered, its runs with their steps, and the operator's note. The master
// TIFFs are copied only when asked, since at 2400 ppi they are most of the
// disk; otherwise the manifest names them.
type Diagnosis struct {
	Name    string     `yaml:"-"` // the directory under diagnosis/
	Scan    string     `yaml:"scan"`
	Kept    time.Time  `yaml:"kept"`
	Note    string     `yaml:"note,omitempty"`
	State   string     `yaml:"state,omitempty"` // where the scan stood, as its page said
	Version string     `yaml:"version,omitempty"`
	Masters bool       `yaml:"masters"`
	Files   []KeptFile `yaml:"files"`
	Size    int64      `yaml:"-"` // bytes on disk
}

// KeptFile is one of the scan's files: copied, or, a master not asked
// for, named with its size so it can be fetched while the scan has it.
type KeptFile struct {
	Path string `yaml:"path"`
	Size int64  `yaml:"size"`
	Kept bool   `yaml:"kept"`
}

// KeepOptions is what the operator says when keeping a scan for
// diagnosis, and what the caller knows that the scan does not.
type KeepOptions struct {
	Note    string
	State   string
	Version string
	Masters bool
	Now     time.Time
}

// keptRun is a run and its steps as kept in runs.yaml.
type keptRun struct {
	Run   wf.RunRecord    `yaml:"run"`
	Steps []wf.StepResult `yaml:"steps"`
}

// Keep copies what the scan has into a diagnosis directory of its own,
// named for the scan and the time, never overwriting an earlier one. The
// scan itself is untouched: the operator carries on, rescans or deletes
// as they would have.
func (l *Scan) Keep(ctx context.Context, store wf.Store, o KeepOptions) (Diagnosis, error) {
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	d := Diagnosis{Scan: l.Key(), Kept: now.UTC(), Note: strings.TrimSpace(o.Note), State: o.State, Version: o.Version, Masters: o.Masters}
	base := fmt.Sprintf("%s-%d-%s", l.Batch, l.N, d.Kept.Format("20060102T150405Z"))
	var dir string
	for i := 1; ; i++ {
		d.Name = base
		if i > 1 {
			d.Name = fmt.Sprintf("%s-%d", base, i)
		}
		dir = filepath.Join(l.root, diagnosisDir, d.Name)
		if err := os.Mkdir(dir, 0o755); err == nil {
			break
		} else if !errors.Is(err, fs.ErrExist) {
			if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
				return d, err
			}
			if err := os.Mkdir(dir, 0o755); err != nil {
				return d, err
			}
			break
		}
	}
	err := filepath.WalkDir(l.Dir(), func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(l.Dir(), path)
		info, err := e.Info()
		if err != nil {
			return err
		}
		f := KeptFile{Path: filepath.ToSlash(rel), Size: info.Size()}
		isMaster := strings.HasPrefix(f.Path, "masters/") && (strings.HasSuffix(f.Path, ".tif") || strings.HasSuffix(f.Path, ".tiff"))
		if !isMaster || o.Masters {
			if err := copyFile(path, filepath.Join(dir, rel)); err != nil {
				return err
			}
			f.Kept = true
		}
		d.Files = append(d.Files, f)
		return nil
	})
	if err != nil {
		return d, err
	}
	// the previews as the page draws them, where there are any yet
	if img, err := l.Annotated(); err == nil {
		writeAtomic(filepath.Join(dir, "annotated.png"), func(w io.Writer) error { return png.Encode(w, img) })
	}
	if _, err := os.Stat(l.path("backs.png")); err == nil {
		if img, err := l.AnnotatedBacks(); err == nil {
			writeAtomic(filepath.Join(dir, "annotated-backs.png"), func(w io.Writer) error { return png.Encode(w, img) })
		}
	}
	runs, err := l.keptRuns(ctx, store)
	if err != nil {
		return d, err
	}
	if err := writeYAML(filepath.Join(dir, "runs.yaml"), runs); err != nil {
		return d, err
	}
	if err := writeYAML(filepath.Join(dir, "manifest.yaml"), d); err != nil {
		return d, err
	}
	d.Size, _ = dirSize(dir)
	return d, nil
}

// keptRuns is the scan's run and its photos' runs, each with its steps.
func (l *Scan) keptRuns(ctx context.Context, store wf.Store) ([]keptRun, error) {
	run, err := store.FindRunByKey(ctx, ScanWorkflow, l.Key())
	if err != nil || run == nil {
		return nil, err
	}
	with := func(r wf.RunRecord) (keptRun, error) {
		steps, err := store.ListSteps(ctx, r.ID)
		return keptRun{Run: r, Steps: steps}, err
	}
	first, err := with(*run)
	if err != nil {
		return nil, err
	}
	runs := []keptRun{first}
	children, err := store.ListRuns(ctx, wf.RunFilter{ParentID: run.ID})
	if err != nil {
		return nil, err
	}
	for _, c := range children {
		k, err := with(c)
		if err != nil {
			return nil, err
		}
		runs = append(runs, k)
	}
	return runs, nil
}

// ListDiagnoses is every diagnosis kept under root, newest first.
func ListDiagnoses(root string) ([]Diagnosis, error) {
	entries, err := os.ReadDir(filepath.Join(root, diagnosisDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var list []Diagnosis
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, diagnosisDir, e.Name())
		var d Diagnosis
		if err := readYAML(filepath.Join(dir, "manifest.yaml"), &d); err != nil {
			d = Diagnosis{Note: "no manifest: " + err.Error()}
		}
		d.Name = e.Name()
		d.Size, _ = dirSize(dir)
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].Kept.Equal(list[j].Kept) {
			return list[i].Kept.After(list[j].Kept)
		}
		return list[i].Name > list[j].Name // the same instant: the later suffix first
	})
	return list, nil
}

// DiscardDiagnosis removes the diagnosis named, which must be one listed.
func DiscardDiagnosis(root, name string) error {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("%q is not a diagnosis", name)
	}
	dir := filepath.Join(root, diagnosisDir, name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("%q is not a diagnosis", name)
	}
	return os.RemoveAll(dir)
}

// copyFile copies src to dst, making dst's directory.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeAtomic(dst, func(w io.Writer) error { _, err := io.Copy(w, in); return err })
}

// dirSize is the bytes under dir.
func dirSize(dir string) (int64, error) {
	var n int64
	err := filepath.WalkDir(dir, func(_ string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		info, err := e.Info()
		if err == nil {
			n += info.Size()
		}
		return err
	})
	return n, err
}
