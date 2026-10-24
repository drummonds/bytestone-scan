package bag

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeString(s string) func(io.Writer) error {
	return func(w io.Writer) error { _, err := io.WriteString(w, s); return err }
}

var info = Info{
	Batch:       "20261020-1",
	Description: "SS Bonita at her berth",
	Bagged:      time.Date(2026, 10, 20, 14, 30, 0, 0, time.UTC),
}

func makePhoto(t *testing.T, root string, id pid.ID) {
	t.Helper()
	b, err := Create(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Add("front.tif", writeString("TIFF bytes")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add("photo.yaml", writeString("id: a\n")); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(info); err != nil {
		t.Fatal(err)
	}
}

func TestBagLayout(t *testing.T) {
	root := t.TempDir()
	makePhoto(t, root, 10)
	dir := filepath.Join(root, "a")

	if got, want := read(t, filepath.Join(dir, "bagit.txt")), "BagIt-Version: 1.0\nTag-File-Character-Encoding: UTF-8\n"; got != want {
		t.Errorf("bagit.txt = %q, want %q", got, want)
	}
	if got := read(t, filepath.Join(dir, "data/front.tif")); got != "TIFF bytes" {
		t.Errorf("front.tif = %q", got)
	}
	bagInfo := read(t, filepath.Join(dir, "bag-info.txt"))
	for _, line := range []string{
		"Bagging-Date: 2026-10-20",
		"External-Description: SS Bonita at her berth",
		"External-Identifier: a",
		"Bytestone-Batch: 20261020-1",
		"Payload-Oxum: 16.2", // 10 + 6 bytes in 2 files
	} {
		if !strings.Contains(bagInfo, line+"\n") {
			t.Errorf("bag-info.txt lacks %q:\n%s", line, bagInfo)
		}
	}

	wantManifest := sum([]byte("TIFF bytes")) + "  data/front.tif\n" +
		sum([]byte("id: a\n")) + "  data/photo.yaml\n"
	if got := read(t, filepath.Join(dir, "manifest-sha256.txt")); got != wantManifest {
		t.Errorf("manifest = %q, want %q", got, wantManifest)
	}

	tags := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(read(t, filepath.Join(dir, "tagmanifest-sha256.txt"))), "\n") {
		h, name, _ := strings.Cut(line, "  ")
		if got := sum([]byte(read(t, filepath.Join(dir, name)))); got != h {
			t.Errorf("tagmanifest: %s has %s, file sums to %s", name, h, got)
		}
		tags[name] = true
	}
	for _, name := range []string{"bagit.txt", "bag-info.txt", "manifest-sha256.txt"} {
		if !tags[name] {
			t.Errorf("tagmanifest lacks %s", name)
		}
	}
}

func TestPhotoAppearsOnlyWhenComplete(t *testing.T) {
	root := t.TempDir()
	b, err := Create(root, 3)
	if err != nil {
		t.Fatal(err)
	}
	b.Add("front.tif", writeString("x"))
	if Complete(root, 3) {
		t.Error("photo complete before Close")
	}
	if _, err := os.Stat(filepath.Join(root, "3")); !os.IsNotExist(err) {
		t.Error("photo directory visible before Close")
	}
	if err := b.Close(info); err != nil {
		t.Fatal(err)
	}
	if !Complete(root, 3) {
		t.Error("photo not complete after Close")
	}
	entries, _ := os.ReadDir(root)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "3" {
		t.Errorf("root holds %v, want only the photo", names)
	}
}

func TestCreateRefusesACompletePhoto(t *testing.T) {
	root := t.TempDir()
	makePhoto(t, root, 3)
	if _, err := Create(root, 3); err == nil {
		t.Error("Create over a complete photo succeeded")
	}
}

func TestCreateRestartsAnUnfinishedPhoto(t *testing.T) {
	root := t.TempDir()
	b, _ := Create(root, 3)
	b.Add("front.tif", writeString("half a scan"))
	// crash: no Close
	makePhoto(t, root, 3)
	if got := read(t, filepath.Join(root, "3", "data/front.tif")); got != "TIFF bytes" {
		t.Errorf("front.tif = %q, want the second attempt's", got)
	}
	if _, err := os.Stat(filepath.Join(root, "3", "data")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "3", "data"))
	if len(entries) != 2 {
		t.Errorf("data holds %d files, want 2", len(entries))
	}
}

func TestReserveTakesTheNextID(t *testing.T) {
	root := t.TempDir()
	var got []string
	for range 3 {
		id, err := Reserve(root)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, id.String())
	}
	if strings.Join(got, ",") != "0,1,2" {
		t.Errorf("first ids %v, want 0,1,2", got)
	}

	makePhoto(t, root, 31) // "z"
	os.Mkdir(filepath.Join(root, "notes"), 0o755)
	os.WriteFile(filepath.Join(root, "README"), nil, 0o644)
	id, err := Reserve(root)
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "10" {
		t.Errorf("after photo z, reserved %s, want 10 (32)", id)
	}
}

func TestReservedIDsAreCreatable(t *testing.T) {
	root := t.TempDir()
	id, _ := Reserve(root)
	makePhoto(t, root, id)
	if !Complete(root, id) {
		t.Error("reserved id did not become a complete photo")
	}
	next, _ := Reserve(root)
	if next == id {
		t.Error("a complete photo's id was reserved again")
	}
}

func TestAddRefusesUnsafeNames(t *testing.T) {
	b, _ := Create(t.TempDir(), 1)
	for _, name := range []string{"", "../x", "a/b", ".hidden"} {
		if err := b.Add(name, writeString("x")); err == nil {
			t.Errorf("Add(%q) succeeded", name)
		}
	}
}

func TestManifestIsSorted(t *testing.T) {
	root := t.TempDir()
	b, _ := Create(root, 1)
	for _, n := range []string{"photo.yaml", "back.tif", "front.tif"} {
		b.Add(n, writeString(n))
	}
	b.Close(info)
	lines := strings.Split(strings.TrimSpace(read(t, filepath.Join(root, "1", "manifest-sha256.txt"))), "\n")
	if !sort.SliceIsSorted(lines, func(i, j int) bool {
		return strings.SplitN(lines[i], "  ", 2)[1] < strings.SplitN(lines[j], "  ", 2)[1]
	}) || len(lines) != 3 {
		t.Errorf("manifest not sorted by path: %v", lines)
	}
}

func TestReleaseFreesAReservationButNeverAPhoto(t *testing.T) {
	root := t.TempDir()
	id, _ := Reserve(root)
	if err := Release(root, id); err != nil {
		t.Fatal(err)
	}
	if again, _ := Reserve(root); again != id {
		t.Errorf("after release, reserved %s, want %s again", again, id)
	}
	makePhoto(t, root, 5)
	if err := Release(root, 5); err == nil {
		t.Error("released a complete photo")
	}
	if !Complete(root, 5) {
		t.Error("complete photo gone")
	}
	if err := Release(root, 9); err != nil {
		t.Errorf("releasing what was never reserved: %v", err)
	}
}
