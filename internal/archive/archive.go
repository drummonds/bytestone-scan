// Package archive keeps the archive of items, flat by id. A photo is
// kept as a BagIt bag (RFC 8493) named by its id, built under <id>.partial
// and renamed into place when complete, so a directory named by an id is
// always a whole photo.
package archive

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

// Info is what bag-info.txt records about a photo.
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

// Photo is a photo being written to the archive.
type Photo struct {
	root  string
	id    pid.ID
	dir   string
	files []payload
}

func dir(root string, id pid.ID) string { return filepath.Join(root, id.String()) }

// Complete reports whether the photo id is in place.
func Complete(root string, id pid.ID) bool {
	fi, err := os.Stat(dir(root, id))
	return err == nil && fi.IsDir()
}

// Reserve claims the next photo id under root: one more than the highest
// id of any photo, complete or unfinished. It returns 0 for an empty root.
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

// Create starts the photo id, discarding anything left by an unfinished
// attempt. It refuses if the photo is already complete.
func Create(root string, id pid.ID) (*Photo, error) {
	if Complete(root, id) {
		return nil, fmt.Errorf("photo %s is already complete", id)
	}
	d := dir(root, id) + partial
	if err := os.RemoveAll(d); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(d, "data"), 0o755); err != nil {
		return nil, err
	}
	return &Photo{root: root, id: id, dir: d}, nil
}

// Add writes one payload file, data/<name>, with write.
func (p *Photo) Add(name string, write func(io.Writer) error) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("photo %s: bad payload name %q", p.id, name)
	}
	f, err := os.Create(filepath.Join(p.dir, "data", name))
	if err != nil {
		return err
	}
	h := sha256.New()
	cw := &counter{w: io.MultiWriter(f, h)}
	if err := write(cw); err != nil {
		f.Close()
		return fmt.Errorf("photo %s: writing %s: %w", p.id, name, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	p.files = append(p.files, payload{name, hex.EncodeToString(h.Sum(nil)), cw.n})
	return nil
}

// Close writes the bag's tag files and puts the photo in place.
func (p *Photo) Close(info Info) error {
	sort.Slice(p.files, func(i, j int) bool { return p.files[i].name < p.files[j].name })
	var manifest strings.Builder
	var octets int64
	for _, f := range p.files {
		fmt.Fprintf(&manifest, "%s  data/%s\n", f.sum, f.name)
		octets += f.size
	}
	bagInfo := fmt.Sprintf("Bagging-Date: %s\nExternal-Identifier: %s\nExternal-Description: %s\nBytestone-Batch: %s\nPayload-Oxum: %d.%d\n",
		info.Bagged.Format("2006-01-02"), p.id, oneLine(info.Description), info.Batch, octets, len(p.files))
	tags := []struct{ name, body string }{
		{"bagit.txt", "BagIt-Version: 1.0\nTag-File-Character-Encoding: UTF-8\n"},
		{"bag-info.txt", bagInfo},
		{"manifest-sha256.txt", manifest.String()},
	}
	var tagManifest strings.Builder
	for _, t := range tags {
		if err := os.WriteFile(filepath.Join(p.dir, t.name), []byte(t.body), 0o644); err != nil {
			return err
		}
		h := sha256.Sum256([]byte(t.body))
		fmt.Fprintf(&tagManifest, "%s  %s\n", hex.EncodeToString(h[:]), t.name)
	}
	if err := os.WriteFile(filepath.Join(p.dir, "tagmanifest-sha256.txt"), []byte(tagManifest.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(p.dir, dir(p.root, p.id))
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

// Release gives up a reservation, and anything an unfinished photo left
// under it, so the id can be reserved again. A complete photo is never
// released.
func Release(root string, id pid.ID) error {
	if Complete(root, id) {
		return fmt.Errorf("photo %s is complete and cannot be released", id)
	}
	return os.RemoveAll(dir(root, id) + partial)
}
