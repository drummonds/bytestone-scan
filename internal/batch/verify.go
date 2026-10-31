package batch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Verdict is how one photo stands with the archive when asked afresh.
type Verdict string

const (
	// Verified: the archive holds the item with every checksum as receipted
	// and, while the bag is still in staging, as the bag carries.
	Verified Verdict = "verified"
	// Pending: not deposited yet, so nothing to verify.
	Pending Verdict = "pending"
	// Rejected: refused as a duplicate, so no item of its own to verify.
	Rejected Verdict = "rejected"
	// Unknown: the archive no longer knows the item.
	Unknown Verdict = "unknown"
	// NotHeld: the archive has the id reserved but holds no bag for it.
	NotHeld Verdict = "not held"
	// Differs: a checksum the archive holds is not the one receipted, or not
	// the bag's.
	Differs Verdict = "differs"
)

// Check is the verdict on one of a scan's items: a photo, or the page an
// import was cut from.
type Check struct {
	Photo   pid.ID
	Page    bool
	Verdict Verdict
	Detail  string // what differs, or which item holds the duplicate
	Files   int    // the files the archive holds
}

// Verify asks the archive afresh for each of the scan's items and checks
// what it holds against the deposit record taken at the time and, while
// the bag is still in staging, against the bag's own manifest; nothing is
// downloaded. The checks are in item order. A service that cannot be
// asked is an error: nothing is verified.
func (l *Scan) Verify(svc archive.Service) ([]Check, error) {
	ids, err := l.items()
	if err != nil {
		return nil, err
	}
	page, err := l.PageItem()
	if err != nil {
		return nil, err
	}
	checks := make([]Check, 0, len(ids))
	for _, id := range ids {
		c, err := l.verify(id, svc)
		if err != nil {
			return nil, err
		}
		c.Page = page != nil && *page == id
		checks = append(checks, c)
	}
	return checks, nil
}

func (l *Scan) verify(id pid.ID, svc archive.Service) (Check, error) {
	c := Check{Photo: id}
	d, err := DepositOf(l.root, id)
	switch {
	case err != nil:
		return c, err
	case d == nil:
		c.Verdict = Pending
		return c, nil
	case d.Status == "duplicate":
		c.Verdict, c.Detail = Rejected, fmt.Sprintf("a duplicate of item %s", d.DuplicateOf)
		return c, nil
	}
	rec, err := svc.Receipt(id)
	if errors.Is(err, archive.ErrUnknown) {
		c.Verdict = Unknown
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if rec.Status != archive.Held {
		c.Verdict, c.Detail = NotHeld, string(rec.Status)
		return c, nil
	}
	c.Files = len(rec.Payload)
	if file := differs(rec.Payload, d.Payload); file != "" {
		c.Verdict, c.Detail = Differs, file+": the archive's checksum is not the one receipted"
		return c, nil
	}
	manifest, err := bag.Manifest(filepath.Join(l.bags(), id.String()))
	if errors.Is(err, os.ErrNotExist) {
		c.Verdict = Verified // purged: the record alone speaks for the bag
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if file := differs(rec.Payload, manifest); file != "" {
		c.Verdict, c.Detail = Differs, file+": the archive's checksum is not the bag's"
		return c, nil
	}
	c.Verdict = Verified
	return c, nil
}

// differs is the first file, in name order, whose checksum the two
// manifests do not agree on, or that one has and the other lacks; "" when
// they are the same.
func differs(a, b map[string]string) string {
	files := make([]string, 0, len(a)+len(b))
	for f := range a {
		files = append(files, f)
	}
	for f := range b {
		if _, ok := a[f]; !ok {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	for _, f := range files {
		if a[f] != b[f] {
			return f
		}
	}
	return ""
}
