package main

import (
	"bytes"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
)

// #118: verify prints one line per item, the scan's key, the photo and
// its verdict, and fails when the archive does not hold an item as it
// was receipted; a photo pending or rejected is said but is not a failure.
func TestVerifyReportsEachItem(t *testing.T) {
	var out bytes.Buffer
	checks := []batch.Check{
		{Photo: 17, Verdict: batch.Verified, Files: 3},
		{Photo: 18, Verdict: batch.Rejected, Detail: "a duplicate of item 7"},
		{Photo: 19, Page: true, Verdict: batch.Differs, Detail: "data/page.jpg: the archive's checksum is not the one receipted"},
		{Photo: 20, Verdict: batch.Pending},
	}
	failed := report(&out, "20261027-4/1", checks)
	want := "20261027-4/1 photo h: verified, 3 files\n" +
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
}
