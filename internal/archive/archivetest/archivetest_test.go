package archivetest

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// bagOf is a deposit of a bag holding the payload files given, its
// manifest right, with the renditions a photo's deposit carries (story
// 055): the front's at recipe 1 and, when the bag has a back.tif, the
// back's under back/.
func bagOf(id string, files map[string]string) *bytes.Buffer {
	renditions := map[string]string{}
	for _, side := range []string{"", "back/"} {
		master := "front.tif"
		if side != "" {
			if _, ok := files["back.tif"]; !ok {
				continue
			}
			master = "back.tif"
		}
		sum := sha256.Sum256([]byte(files[master]))
		renditions[side+"recipe.yaml"] = "version: 1\nmaster: " + hex.EncodeToString(sum[:]) + "\n"
		for _, n := range []string{"display.jpg", "thumb.jpg", "tiny.jpg"} {
			renditions[side+n] = "jpeg of " + master
		}
	}
	return depositOf(id, files, renditions)
}

// depositOf is a deposit of the bag's payload files and the renditions
// given, under renditions/1/.
func depositOf(id string, files, renditions map[string]string) *bytes.Buffer {
	var manifest strings.Builder
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sum := sha256.Sum256([]byte(files[n]))
		fmt.Fprintf(&manifest, "%s  data/%s\n", hex.EncodeToString(sum[:]), n)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	write := func(name, body string) {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	write("bag/bagit.txt", "BagIt-Version: 1.0\n")
	write("bag/bag-info.txt", "External-Identifier: "+id+"\n")
	write("bag/manifest-sha256.txt", manifest.String())
	for _, n := range names {
		write("bag/data/"+n, files[n])
	}
	for n, body := range renditions {
		write("renditions/1/"+n, body)
	}
	_ = tw.Close()
	return &buf
}

// The fake follows life-weaver's decision table for depositing an item.
func TestTheFakeFollowsTheDepositTable(t *testing.T) {
	f := Example()
	if _, err := f.Reserve(12); err == nil {
		t.Error("reserved for a person not in the register")
	}
	one, err := f.Reserve(10)
	if err != nil {
		t.Fatal(err)
	}
	two, _ := f.Reserve(14)
	if one == two {
		t.Fatal("the same id twice")
	}
	if rec, err := f.Receipt(one); err != nil || rec.Status != archive.Reserved || rec.Home != 10 {
		t.Errorf("reserved receipt: %+v %v", rec, err)
	}
	var ref *archive.Refusal
	if _, err := f.Deposit(99, bagOf("99", nil)); err == nil || errors.As(err, &ref) {
		t.Errorf("deposit of an id never reserved: %v, want a plain error", err)
	}
	if _, err := f.Deposit(one, bagOf(one.String(), nil)); err == nil || errors.As(err, &ref) {
		t.Errorf("deposit of an empty bag: %v, want a plain error", err)
	}
	if _, err := f.Deposit(one, bagOf(two.String(), map[string]string{"front.tif": "pixels"})); err == nil || errors.As(err, &ref) {
		t.Errorf("deposit of a bag identified as another item: %v, want a plain error", err)
	}
	rec, err := f.Deposit(one, bagOf(one.String(), map[string]string{"front.tif": "pixels", "photo.yaml": "batch: x"}))
	if err != nil || rec.Status != archive.Held || len(rec.Payload) != 2 {
		t.Fatalf("held: %+v %v", rec, err)
	}
	if rec, err := f.Deposit(one, bagOf(one.String(), map[string]string{"front.tif": "pixels", "photo.yaml": "batch: x"})); err != nil || rec.Status != archive.Held {
		t.Errorf("the same bag twice: %+v %v, want held", rec, err)
	}
	if _, err := f.Deposit(one, bagOf(one.String(), map[string]string{"front.tif": "other"})); !errors.As(err, &ref) || ref.Status != "conflict" {
		t.Errorf("other contents under a held id: %v, want a conflict", err)
	}
	// a picture held already, under another id: a duplicate; the record beside it does not count
	_, err = f.Deposit(two, bagOf(two.String(), map[string]string{"front.tif": "pixels", "photo.yaml": "batch: y"}))
	if !errors.As(err, &ref) || ref.Status != "duplicate" || ref.DuplicateOf != one || ref.Home != 10 || ref.File != "data/front.tif" {
		t.Errorf("a picture held already: %v, want a duplicate refusal naming %s", err, one)
	}
	if rec, _ := f.Receipt(two); rec.Status != archive.Reserved {
		t.Errorf("after a refusal the id is still reserved: %+v", rec)
	}
	// a test can make the fake reject a deposit as a duplicate outright
	three, _ := f.Reserve(10)
	f.RejectAs[three] = one
	if _, err := f.Deposit(three, bagOf(three.String(), map[string]string{"front.tif": "new"})); !errors.As(err, &ref) || ref.DuplicateOf != one {
		t.Errorf("RejectAs: %v", err)
	}
	// story 055: a deposit carries the renditions, or is refused
	four, _ := f.Reserve(14)
	pixels := map[string]string{"front.tif": "four", "back.tif": "four's back", "photo.yaml": "batch: z"}
	whole := bagOf(four.String(), pixels)
	withSum := func(master string) string {
		sum := sha256.Sum256([]byte(master))
		return "version: 1\nmaster: " + hex.EncodeToString(sum[:]) + "\n"
	}
	for name, dep := range map[string]*bytes.Buffer{
		"no renditions":           depositOf(four.String(), pixels, nil),
		"no thumb":                depositOf(four.String(), pixels, map[string]string{"display.jpg": "j", "tiny.jpg": "j", "recipe.yaml": withSum("four"), "back/display.jpg": "j", "back/thumb.jpg": "j", "back/tiny.jpg": "j", "back/recipe.yaml": withSum("four's back")}),
		"no back":                 depositOf(four.String(), pixels, map[string]string{"display.jpg": "j", "thumb.jpg": "j", "tiny.jpg": "j", "recipe.yaml": withSum("four")}),
		"another master's recipe": depositOf(four.String(), pixels, map[string]string{"display.jpg": "j", "thumb.jpg": "j", "tiny.jpg": "j", "recipe.yaml": withSum("other"), "back/display.jpg": "j", "back/thumb.jpg": "j", "back/tiny.jpg": "j", "back/recipe.yaml": withSum("four's back")}),
	} {
		if _, err := f.Deposit(four, dep); !errors.As(err, &ref) || ref.Status != "renditions" || ref.File == "" {
			t.Errorf("%s: %v, want a renditions refusal naming the file", name, err)
		}
	}
	if rec, _ := f.Receipt(four); rec.Status != archive.Reserved {
		t.Errorf("after a renditions refusal the id is still reserved: %+v", rec)
	}
	if rec, err := f.Deposit(four, whole); err != nil || rec.Status != archive.Held || len(rec.Payload) != 3 {
		t.Fatalf("whole: %+v %v", rec, err)
	}
	if got := f.Renditions[four]; len(got) != 8 {
		t.Errorf("renditions placed for %s: %v, want the front's four and the back's four", four, got)
	}
	// held already with the same manifest: held, whatever the renditions
	if rec, err := f.Deposit(four, depositOf(four.String(), pixels, nil)); err != nil || rec.Status != archive.Held {
		t.Errorf("the same bag again without renditions: %+v %v, want held", rec, err)
	}
	// a bag at the root of the tar is not a deposit
	if _, err := f.Deposit(four, oldBag(four.String(), pixels)); err == nil || errors.As(err, &ref) {
		t.Errorf("a bare bag: %v, want a plain error", err)
	}
	f.Down = true
	if _, err := f.Reserve(10); err == nil {
		t.Error("the fake answered while down")
	}
}

// oldBag is the story 041 deposit: the bag at the root of the tar.
func oldBag(id string, files map[string]string) *bytes.Buffer {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	write := func(name, body string) {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	write("bagit.txt", "BagIt-Version: 1.0\n")
	write("bag-info.txt", "External-Identifier: "+id+"\n")
	write("manifest-sha256.txt", "")
	for n, body := range files {
		write("data/"+n, body)
	}
	_ = tw.Close()
	return &buf
}

// The fake follows life-weaver's table for a writing (its stories 288,
// 331, 336): its djot and its record alone, no renditions wanted, the
// djot checked as the site's renderer will read it, and a duplicate by
// the djot.
func TestTheFakeHoldsAWritingAsItsDjotAndItsRecordAlone(t *testing.T) {
	f := Example()
	ids := make([]string, 6)
	for i := range ids {
		id, err := f.Reserve(10)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id.String()
	}
	deposit := func(i int, files map[string]string) (archive.Receipt, error) {
		return f.Deposit(pid.ID(i), depositOf(ids[i], files, nil))
	}
	rec, err := deposit(0, map[string]string{"writing.yaml": "title: A", "writing.dj": "Some words.\n"})
	if err != nil || rec.Status != archive.Held || rec.Kind != "writing" || len(rec.Payload) != 2 {
		t.Fatalf("a writing: %+v, %v, want held as a writing", rec, err)
	}
	if len(f.Renditions[0]) != 0 {
		t.Errorf("renditions kept for a writing: %v", f.Renditions[0])
	}
	var ref *archive.Refusal
	for name, files := range map[string]map[string]string{
		"another file":      {"writing.yaml": "title: B", "writing.dj": "Other words.\n", "writing.docx": "PK"},
		"no djot":           {"writing.yaml": "title: B", "writing.md": "Other words.\n"},
		"nothing to read":   {"writing.yaml": "title: B", "writing.dj": "``` =html\n<b>bold</b>\n```\n"},
		"not UTF-8":         {"writing.yaml": "title: B", "writing.dj": "caf\xe9\n"},
		"an account's bag":  {"account.yaml": "title: B", "account.dj": "Other words.\n"},
		"a writing's alone": {"writing.dj": "Other words.\n"},
	} {
		if _, err := deposit(1, files); err == nil || errors.As(err, &ref) {
			t.Errorf("%s: %v, want a plain error", name, err)
		}
	}
	if _, err := deposit(2, map[string]string{"writing.yaml": "title: C", "writing.dj": "Some words.\n"}); !errors.As(err, &ref) ||
		ref.Status != "duplicate" || ref.DuplicateOf != 0 || ref.File != "data/writing.dj" {
		t.Errorf("the same djot again: %v, want a duplicate of item 0 by its djot", err)
	}
	// a record that reads the same is no duplicate: only the djot is compared
	if rec, err := deposit(3, map[string]string{"writing.yaml": "title: A", "writing.dj": "Words of another.\n"}); err != nil || rec.Kind != "writing" {
		t.Errorf("another writing under the same record: %+v, %v", rec, err)
	}
}
