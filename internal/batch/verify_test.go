package batch

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wf "git.bytestone.uk/hum3/gobank-workflow"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// #118: a completed scan is verified against the archive afresh: the
// archive is asked to recompute each photo's checksums from its disk
// (life-weaver#87), or for its receipt when it is too old to, which must
// say held with the checksums the deposit record took and the bag
// carries, nothing being downloaded. A photo rejected as a duplicate has no item to verify; one
// the archive has lost, has reserved but not held, or holds with another
// checksum is reported, file by file; a service that is down verifies
// nothing.
func TestAScanIsVerifiedAgainstTheArchive(t *testing.T) {
	root, store := t.TempDir(), wf.NewMemStore()
	_, l := through(t, root, store, 7) // the first photo a duplicate of item 7, the second held
	_, ids, _ := l.Found()
	dup, held := ids[0], ids[1]
	svc := service(root)

	checks, err := l.Verify(svc)
	if err != nil {
		t.Fatal(err)
	}
	want := []Check{{Photo: dup, Verdict: Rejected, Detail: "a duplicate of item 7"}, {Photo: held, Verdict: Verified, Files: 3, Detail: "checksums recomputed by the archive"}}
	if len(checks) != 2 || checks[0] != want[0] || checks[1] != want[1] {
		t.Errorf("checks %+v, want %+v", checks, want)
	}

	svc.Rotten[held] = []string{"data/front.tif"}
	if c := check(t, l, svc, held); c.Verdict != Differs || !strings.Contains(c.Detail, "at the archive, data/front.tif: sha256 does not match") {
		t.Errorf("a master decayed at the archive: %+v, want differs with the archive's fault", c)
	}
	delete(svc.Rotten, held)
	svc.NoFixity = true // an archive from before life-weaver#87
	if c := check(t, l, svc, held); c.Verdict != Verified || !strings.Contains(c.Detail, "receipt only") {
		t.Errorf("an archive without fixity: %+v, want verified by its receipt only", c)
	}
	svc.NoFixity = false

	rec, _ := svc.Receipt(held)
	altered := map[string]string{}
	maps.Copy(altered, rec.Payload)
	altered["data/front.tif"] = strings.Repeat("0", 64)
	svc.Hold(held, altered)
	if c := check(t, l, svc, held); c.Verdict != Differs || !strings.Contains(c.Detail, "data/front.tif") || !strings.Contains(c.Detail, "receipted") {
		t.Errorf("a checksum changed at the archive: %+v, want differs on data/front.tif from the one receipted", c)
	}

	svc.Hold(held, rec.Payload)
	manifest := filepath.Join(root, "bags", held.String(), "manifest-sha256.txt")
	os.WriteFile(manifest, []byte(strings.Repeat("1", 64)+"  data/back.tif\n"), 0o644)
	if c := check(t, l, svc, held); c.Verdict != Differs || !strings.Contains(c.Detail, "data/back.tif") || !strings.Contains(c.Detail, "bag") {
		t.Errorf("the bag's manifest differs from the archive's: %+v, want differs on data/back.tif from the bag's", c)
	}
	os.RemoveAll(filepath.Join(root, "bags", held.String())) // purged: the record alone is checked
	if c := check(t, l, svc, held); c.Verdict != Verified {
		t.Errorf("after the purge: %+v, want verified from the deposit record", c)
	}

	svc.Hold(held, nil)
	if c := check(t, l, svc, held); c.Verdict != NotHeld {
		t.Errorf("reserved, no bag: %+v, want not held", c)
	}
	svc.Forget(held)
	if c := check(t, l, svc, held); c.Verdict != Unknown {
		t.Errorf("forgotten by the archive: %+v, want unknown", c)
	}

	svc.Down = true
	if _, err := l.Verify(svc); err == nil {
		t.Error("verified against a service that is down")
	}
	svc.Down = false

	// a scan not yet through: nothing deposited, nothing to verify
	open := describe(t, root)
	run(t, open, store, twoSided())
	open.Confirm()
	scanned(t, open, store, twoSided())
	checks, err = open.Verify(svc)
	if err != nil || len(checks) != 2 || checks[0].Verdict != Pending || checks[1].Verdict != Pending {
		t.Errorf("an open scan: %+v %v, want two pending", checks, err)
	}
}

// check is the verdict on photo id among the scan's.
func check(t *testing.T, l *Scan, svc archive.Service, id pid.ID) Check {
	t.Helper()
	checks, err := l.Verify(svc)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	for _, c := range checks {
		if c.Photo == id {
			return c
		}
	}
	t.Fatalf("verify: no check of %s in %+v", id, checks)
	return Check{}
}
