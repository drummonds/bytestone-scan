package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive/archivetest"
	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

// #118: verify prints one line per item, the scan's key, the photo and
// its verdict, and fails when the archive does not hold an item as it
// was receipted; a photo pending or rejected is said but is not a failure.
func TestVerifyReportsEachItem(t *testing.T) {
	var out bytes.Buffer
	checks := []batch.Check{
		{Photo: 17, Verdict: batch.Verified, Files: 3, Detail: "checksums recomputed by the archive"},
		{Photo: 18, Verdict: batch.Rejected, Detail: "a duplicate of item 7"},
		{Photo: 19, Page: true, Verdict: batch.Differs, Detail: "data/page.jpg: the archive's checksum is not the one receipted"},
		{Photo: 20, Verdict: batch.Pending},
	}
	failed := report(&out, "20261027-4/1", checks)
	want := "20261027-4/1 photo h: verified, 3 files, checksums recomputed by the archive\n" +
		"20261027-4/1 photo j: rejected, a duplicate of item 7\n" +
		"20261027-4/1 page k: differs, data/page.jpg: the archive's checksum is not the one receipted\n" +
		"20261027-4/1 photo m: pending\n"
	if out.String() != want {
		t.Errorf("report:\n%s\nwant:\n%s", out.String(), want)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1: the item that differs", failed)
	}
	out.Reset()
	if failed := report(&out, "20261027-4/2", nil); failed != 0 || !strings.Contains(out.String(), "nothing to verify") {
		t.Errorf("an empty scan: %q, %d", out.String(), failed)
	}

	root, svc := t.TempDir(), archivetest.Example()
	b, err := batch.Describe(root, svc, nil, batch.Description{Text: "a shoebox", Person: 10, ScanPPI: 600, Credit: "CC BY 4.0"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := verify(root, b.ID, svc, &out); err != nil || !strings.Contains(out.String(), b.ID+": no scans") {
		t.Errorf("a batch with no scans: %q, %v", out.String(), err)
	}
	if err := verify(root, "20000101-1", svc, &out); err == nil {
		t.Error("a batch that does not exist verified")
	}
}
