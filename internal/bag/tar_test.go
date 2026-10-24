package bag

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Story 041: a bag travels to the archive service as a tar of its
// directory, every path relative to the bag.
func TestTarIsTheBagRelativeToItself(t *testing.T) {
	root := t.TempDir()
	p, err := Create(root, 7)
	if err != nil {
		t.Fatal(err)
	}
	p.Add("front.tif", func(w io.Writer) error { _, err := w.Write([]byte("front")); return err })
	p.Add("photo.yaml", func(w io.Writer) error { _, err := w.Write([]byte("id: 7\n")); return err })
	if err := p.Close(Info{Batch: "b", Load: 1, Description: "d", Bagged: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Tar(filepath.Join(root, "7"), &buf); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	tr := tar.NewReader(&buf)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(tr)
		got[h.Name] = string(body)
	}
	for _, name := range []string{"bagit.txt", "bag-info.txt", "manifest-sha256.txt", "tagmanifest-sha256.txt", "data/front.tif", "data/photo.yaml"} {
		disk, _ := os.ReadFile(filepath.Join(root, "7", name))
		if got[name] != string(disk) {
			t.Errorf("%s: %q in the tar, %q on disk", name, got[name], disk)
		}
	}
	if len(got) != 6 {
		t.Errorf("%d entries, want 6: %v", len(got), got)
	}
}
