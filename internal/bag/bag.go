// Package bag writes a photo's archive directory as a BagIt bag
// (RFC 8493) named by the photo's id. A bag is built under <id>.partial
// and renamed into place when complete, so a directory named by an id is
// always a whole bag.
package bag

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

const partial = ".partial"

// Info is what bag-info.txt records about a bag.
type Info struct {
	Batch       string
	Description string
	Bagged      time.Time
}

type payload struct {
	name string
	sum  string
	size int64
}

// Bag is a bag being built.
type Bag struct {
	root  string
	id    pid.ID
	dir   string
	files []payload
}

func dir(root string, id pid.ID) string { return filepath.Join(root, id.String()) }

// Complete reports whether the bag for id is in place.
func Complete(root string, id pid.ID) bool {
	fi, err := os.Stat(dir(root, id))
	return err == nil && fi.IsDir()
}

// Reserve claims the next photo id under root: one more than the highest
// id of any bag, complete or unfinished. It returns 0 for an empty root.
func Reserve(root string) (pid.ID, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, err
	}
	next := pid.ID(0)
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), partial)
		id, err := pid.Parse(name)
		if err != nil || id.String() != name || !e.IsDir() {
			continue
		}
		next = max(next, id+1)
	}
	for {
		err := os.Mkdir(dir(root, next)+partial, 0o755)
		if err == nil {
			return next, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return 0, err
		}
		next++
	}
}

// Create starts the bag for id, discarding anything left by an unfinished
// attempt. It refuses if the bag is already complete.
func Create(root string, id pid.ID) (*Bag, error) {
	if Complete(root, id) {
		return nil, fmt.Errorf("bag %s is already complete", id)
	}
	d := dir(root, id) + partial
	if err := os.RemoveAll(d); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(d, "data"), 0o755); err != nil {
		return nil, err
	}
	return &Bag{root: root, id: id, dir: d}, nil
}

// Add writes one payload file, data/<name>, with write.
func (b *Bag) Add(name string, write func(io.Writer) error) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("bag %s: bad payload name %q", b.id, name)
	}
	f, err := os.Create(filepath.Join(b.dir, "data", name))
	if err != nil {
		return err
	}
	h := sha256.New()
	cw := &counter{w: io.MultiWriter(f, h)}
	if err := write(cw); err != nil {
		f.Close()
		return fmt.Errorf("bag %s: writing %s: %w", b.id, name, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	b.files = append(b.files, payload{name, hex.EncodeToString(h.Sum(nil)), cw.n})
	return nil
}

// Close writes the tag files and puts the bag in place.
func (b *Bag) Close(info Info) error {
	sort.Slice(b.files, func(i, j int) bool { return b.files[i].name < b.files[j].name })
	var manifest strings.Builder
	var octets int64
	for _, p := range b.files {
		fmt.Fprintf(&manifest, "%s  data/%s\n", p.sum, p.name)
		octets += p.size
	}
	bagInfo := fmt.Sprintf("Bagging-Date: %s\nExternal-Identifier: %s\nExternal-Description: %s\nBytestone-Batch: %s\nPayload-Oxum: %d.%d\n",
		info.Bagged.Format("2006-01-02"), b.id, oneLine(info.Description), info.Batch, octets, len(b.files))
	tags := []struct{ name, body string }{
		{"bagit.txt", "BagIt-Version: 1.0\nTag-File-Character-Encoding: UTF-8\n"},
		{"bag-info.txt", bagInfo},
		{"manifest-sha256.txt", manifest.String()},
	}
	var tagManifest strings.Builder
	for _, t := range tags {
		if err := os.WriteFile(filepath.Join(b.dir, t.name), []byte(t.body), 0o644); err != nil {
			return err
		}
		h := sha256.Sum256([]byte(t.body))
		fmt.Fprintf(&tagManifest, "%s  %s\n", hex.EncodeToString(h[:]), t.name)
	}
	if err := os.WriteFile(filepath.Join(b.dir, "tagmanifest-sha256.txt"), []byte(tagManifest.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(b.dir, dir(b.root, b.id))
}

// oneLine keeps a tag value on one line, as bag-info.txt requires.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

type counter struct {
	w io.Writer
	n int64
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// Release gives up a reservation, and anything an unfinished bag left
// under it, so the id can be reserved again. A complete bag is never
// released.
func Release(root string, id pid.ID) error {
	if Complete(root, id) {
		return fmt.Errorf("bag %s is complete and cannot be released", id)
	}
	return os.RemoveAll(dir(root, id) + partial)
}
