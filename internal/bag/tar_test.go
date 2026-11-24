package bag

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Story 041, 055: a photo travels to the archive service as a tar with
// two top-level directories: bag/, the bag relative to itself, and
// renditions/<recipe>/, the front's renditions with the back's under
// back/.
func TestDepositIsTheBagAndItsRenditions(t *testing.T) {
	root := t.TempDir()
	p, err := Create(root, 7)
	if err != nil {
		t.Fatal(err)
	}
	p.Add("front.tif", func(w io.Writer) error { _, err := w.Write([]byte("front")); return err })
	p.Add("photo.yaml", func(w io.Writer) error { _, err := w.Write([]byte("id: 7\n")); return err })
	if err := p.Close(Info{Batch: "b", Scan: 1, Description: "d", Bagged: time.Now()}); err != nil {
		t.Fatal(err)
	}
	renditions := filepath.Join(root, "renditions", "7", "1")
	os.MkdirAll(filepath.Join(renditions, "back"), 0o755)
	for _, name := range []string{"display.jpg", "thumb.jpg", "tiny.jpg", "recipe.yaml", "back/display.jpg", "back/thumb.jpg", "back/tiny.jpg", "back/recipe.yaml"} {
		os.WriteFile(filepath.Join(renditions, name), []byte(name), 0o644)
	}
	var buf bytes.Buffer
	if err := Deposit(&buf, filepath.Join(root, "7"), renditions, 1); err != nil {
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
		if got["bag/"+name] != string(disk) {
			t.Errorf("bag/%s: %q in the tar, %q on disk", name, got["bag/"+name], disk)
		}
	}
	for _, name := range []string{"display.jpg", "recipe.yaml", "back/display.jpg", "back/recipe.yaml"} {
		if got["renditions/1/"+name] != name {
			t.Errorf("renditions/1/%s: %q in the tar", name, got["renditions/1/"+name])
		}
	}
	if len(got) != 14 {
		t.Errorf("%d entries, want 14: %v", len(got), got)
	}
	if err := Deposit(&buf, filepath.Join(root, "7"), filepath.Join(root, "renditions", "8", "1"), 1); err == nil {
		t.Error("a deposit without renditions on disk")
	}
	// a writing has no renditions: its deposit is its bag alone (story 190)
	buf.Reset()
	if err := Deposit(&buf, filepath.Join(root, "7"), "", 1); err != nil {
		t.Fatal(err)
	}
	for tr, n := tar.NewReader(&buf), 0; ; n++ {
		h, err := tr.Next()
		if err == io.EOF {
			if n != 6 {
				t.Errorf("%d entries in a deposit of no renditions, want the bag's 6", n)
			}
			break
		}
		if err != nil || !strings.HasPrefix(h.Name, "bag/") {
			t.Fatalf("entry %v, %v: want the bag alone", h, err)
		}
	}
}
