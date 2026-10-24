// Package plan decides how a batch is scanned: each print's master ppi
// and averaging factor, and the one detail pass that covers them all.
package plan

import (
	"errors"
	"fmt"
	"image"
	"math"

	"git.bytestone.uk/hum3/bytestone-scan/internal/detailpass"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

const mmPerInch = 25.4

// Master is how one print's master is made.
type Master struct {
	PPI     int
	Factor  int    // scan pixels averaged per master pixel, each way
	Warning string // set when the scan ppi is too low for the print
}

// MasterFor is the master for a print scanned at scanPPI: 600 ppi, or 1200
// ppi for a print smaller than 6 × 4 inches, averaged down from the scan.
// Prints sold as 6 × 4 are often cut a little small, so a print counts as
// smaller only when a side is more than 3 mm short. The print's own size
// counts, however it is turned on the bed.
func MasterFor(print find.Box, scanPPI int) (Master, error) {
	if scanPPI != 600 && scanPPI != 1200 && scanPPI != 2400 {
		return Master{}, fmt.Errorf("scan ppi %d, want 600, 1200 or 2400", scanPPI)
	}
	short, long := print.W, print.H
	if short > long {
		short, long = long, short
	}
	want := 600
	const allowance = 3.0
	if short < 4*mmPerInch-allowance || long < 6*mmPerInch-allowance {
		want = 1200
	}
	m := Master{PPI: min(want, scanPPI)}
	m.Factor = scanPPI / m.PPI
	if m.PPI < want {
		m.Warning = fmt.Sprintf("print is smaller than 6 × 4 inches: wanted %d ppi, scanned at %d", want, scanPPI)
	}
	return m, nil
}

// Pass is the detail pass for a batch.
type Pass struct {
	ScanPPI int
	Area    find.Rect           // what the scanner is asked for, in mm on the bed
	Regions []detailpass.Region // each region's bounds on the bed, in scan pixels
	Masters []Master            // each print's master
}

// New plans one detail pass at scanPPI over every print's region: its
// bounds within the bed, since the scanner scans only rectangles square
// to the bed.
func New(prints []find.Print, bed find.Rect, scanPPI int) (Pass, error) {
	return pass(prints, bed, scanPPI, func(pr find.Print) (Master, error) { return MasterFor(pr.Print, scanPPI) })
}

// BackPPI is the resolution a back is scanned and kept at: the format
// statement's level for the handwriting, stamps and backprint on it.
const BackPPI = 300

// Backs is the pass over the backs of prints turned over in place: each
// back is scanned where its front's region lies, at BackPPI, and kept
// unaveraged.
func Backs(prints []find.Print, bed find.Rect) (Pass, error) {
	return pass(prints, bed, BackPPI, func(find.Print) (Master, error) { return Master{PPI: BackPPI, Factor: 1}, nil })
}

// pass covers every print's region on the bed in one scan at scanPPI,
// each print's master as masterFor says.
func pass(prints []find.Print, bed find.Rect, scanPPI int, masterFor func(find.Print) (Master, error)) (Pass, error) {
	if len(prints) == 0 {
		return Pass{}, errors.New("no prints to scan")
	}
	p := Pass{ScanPPI: scanPPI, Area: prints[0].Region.On(bed)}
	for _, pr := range prints[1:] {
		r := pr.Region.On(bed)
		p.Area = find.Rect{X0: min(p.Area.X0, r.X0), Y0: min(p.Area.Y0, r.Y0), X1: max(p.Area.X1, r.X1), Y1: max(p.Area.Y1, r.Y1)}
	}
	px := func(mm float64) int { return int(math.Round(mm / mmPerInch * float64(scanPPI))) }
	for _, pr := range prints {
		m, err := masterFor(pr)
		if err != nil {
			return Pass{}, err
		}
		r := pr.Region.On(bed)
		p.Masters = append(p.Masters, m)
		p.Regions = append(p.Regions, detailpass.Region{
			Rect:   image.Rect(px(r.X0-p.Area.X0), px(r.Y0-p.Area.Y0), px(r.X1-p.Area.X0), px(r.Y1-p.Area.Y0)),
			Factor: m.Factor,
		})
	}
	return p, nil
}
