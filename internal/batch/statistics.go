package batch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Stats is what staging holds, counted: the batches by state, their
// scans, their photos by how they stand with the archive, and the size of
// each entry at the staging root.
type Stats struct {
	Batches map[string]int // by State
	Scans   int
	Photos  Outcome
	Staging []Usage // each entry at the staging root, in name order
}

// Usage is the bytes under one entry at the staging root.
type Usage struct {
	Name  string
	Bytes int64
}

// Total is the bytes under every entry.
func (s Stats) Total() int64 {
	var n int64
	for _, u := range s.Staging {
		n += u.Bytes
	}
	return n
}

// State is where the batch stands: open, finished, closed or purged.
func (b *Batch) State() string {
	if _, ok := b.Purged(); ok {
		return "purged"
	}
	if _, ok := b.Closed(); ok {
		return "closed"
	}
	if b.Finished() {
		return "finished"
	}
	return "open"
}

// Statistics counts what staging under root holds.
func Statistics(root string) (Stats, error) {
	s := Stats{Batches: map[string]int{}}
	all, err := All(root)
	if err != nil {
		return s, err
	}
	for _, b := range all {
		s.Batches[b.State()]++
		scans, err := b.Scans()
		if err != nil {
			return s, err
		}
		s.Scans += len(scans)
		o, err := b.Outcome()
		if err != nil {
			return s, err
		}
		s.Photos.Held += o.Held
		s.Photos.Rejected = append(s.Photos.Rejected, o.Rejected...)
		s.Photos.Pending += o.Pending
	}
	entries, err := os.ReadDir(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return s, err
	}
	for _, e := range entries {
		n, err := usage(filepath.Join(root, e.Name()))
		if err != nil {
			return s, err
		}
		s.Staging = append(s.Staging, Usage{Name: e.Name(), Bytes: n})
	}
	return s, nil
}

// usage is the bytes under path, a file or a directory.
func usage(path string) (int64, error) {
	var n int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info, err := d.Info(); err == nil && !d.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n, err
}
