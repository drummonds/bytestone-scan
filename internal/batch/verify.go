package batch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"git.bytestone.uk/hum3/bytestone-scan/internal/archive"
	"git.bytestone.uk/hum3/bytestone-scan/internal/bag"
	"git.bytestone.uk/hum3/bytestone-scan/internal/pid"
)

// Verdict is how one photo stands with the archive when asked afresh.
type Verdict string

const (
	// Verified: the archive holds the item with every master's checksum as
	// receipted and, while the bag is still in staging, as the bag
	// carries; its files recomputed by the archive when it offers that,
	// else by its receipt. Its record may have changed since (#191).
	Verified Verdict = "verified"
	// Pending: not deposited yet, so nothing to verify.
	Pending Verdict = "pending"
	// Rejected: refused as a duplicate, so no item of its own to verify.
	Rejected Verdict = "rejected"
	// Unknown: the archive no longer knows the item.
	Unknown Verdict = "unknown"
	// NotHeld: the archive has the id reserved but holds no bag for it.
	NotHeld Verdict = "not held"
	// Differs: a master's checksum at the archive is not the one receipted,
	// or not the bag's; or the archive holds another set of files.
	Differs Verdict = "differs"
)

// Check is the verdict on one of a scan's items: a photo, or the page an
// import was cut from.
type Check struct {
	Photo pid.ID
	// Home is the home now: the person the archive says the item is homed
	// with, when it was asked and holds it; else the home as sent.
	Home pid.ID
	// Sent is the home as sent: the one the archive's answer named at the
	// time of the deposit.
	Sent    pid.ID
	Page    bool
	Verdict Verdict
	Detail  string // how it was verified, what differs, or which item holds the duplicate
	Files   int    // the files the archive holds
	// Changed: the record has changed at the archive since the deposit,
	// a correction or a renumber; no fault.
	Changed bool
}

// records are the files of a bag that are its record, which the archive
// may rewrite; every other file is a master, which it never does.
var records = []string{"data/photo.yaml", "data/page.yaml", "data/film.yaml", "data/letter.yaml"}

const changedSince = "the record has changed at the archive since the deposit"

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
	}
	c.Home, c.Sent = d.Home, d.Home
	if d.Status == "duplicate" {
		c.Verdict, c.Detail = Rejected, fmt.Sprintf("a duplicate of item %s", d.DuplicateOf)
		if d.DuplicateOf == 0 {
			// an older answer: 0 is no id (#191)
			c.Detail = "a duplicate of an item whose id is gone"
		}
		return c, nil
	}
	// the archive's own check of its disk first (life-weaver#87); an
	// archive from before it answers as for an unknown item, and its
	// receipt, what its index says, has to do
	var held map[string]string
	fx, err := svc.Fixity(id)
	switch {
	case err == nil && fx.Status != archive.Held:
		c.Verdict, c.Detail = NotHeld, string(fx.Status)
		return c, nil
	case err == nil && !fx.Sound:
		c.Home = fx.Home
		c.Verdict, c.Detail = Differs, "at the archive, "+joinFailures(fx.Failures)
		c.Files = len(fx.Payload)
		return c, nil
	case err == nil:
		c.Home = fx.Home
		held, c.Detail = fx.Payload, "checksums recomputed by the archive"
	case errors.Is(err, archive.ErrUnknown):
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
		c.Home = rec.Home
		held, c.Detail = rec.Payload, "the archive's receipt only: it offers no fixity check"
	default:
		return c, err
	}
	c.Files = len(held)
	for _, file := range union(held, d.Payload) {
		at, receipted := held[file], d.Payload[file]
		switch {
		case at == receipted:
		case at == "":
			c.Verdict, c.Detail = Differs, file+": receipted, and not held by the archive"
			return c, nil
		case receipted == "":
			c.Verdict, c.Detail = Differs, file+": held by the archive, and not receipted"
			return c, nil
		case slices.Contains(records, file):
			c.Changed = true
		default:
			c.Verdict, c.Detail = Differs, file+": the archive's checksum is not the one receipted"
			return c, nil
		}
	}
	if c.Changed {
		c.Detail += "; " + changedSince
	}
	manifest, err := bag.Manifest(filepath.Join(l.bags(), id.String()))
	if errors.Is(err, os.ErrNotExist) {
		c.Verdict = Verified // purged: the answer alone speaks for the bag
		return c, nil
	}
	if err != nil {
		return c, err
	}
	// the bag is as sent: its record is not the archive's to follow
	for _, file := range union(held, manifest) {
		if !slices.Contains(records, file) && held[file] != manifest[file] {
			c.Verdict, c.Detail = Differs, file+": the archive's checksum is not the bag's"
			return c, nil
		}
	}
	c.Verdict = Verified
	return c, nil
}

// joinFailures names each fault the archive found, in its order.
func joinFailures(fs []archive.Failure) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = f.String()
	}
	return strings.Join(parts, "; ")
}

// union is every file either manifest names, in name order.
func union(a, b map[string]string) []string {
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
	return files
}
