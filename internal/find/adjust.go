package find

import "fmt"

// Edge is one side of a region.
type Edge int

const (
	Top Edge = iota
	Bottom
	Left
	Right
)

var edgeNames = [...]string{"top", "bottom", "left", "right"}

func (e Edge) String() string { return edgeNames[e] }

func ParseEdge(s string) (Edge, error) {
	for i, n := range edgeNames {
		if n == s {
			return Edge(i), nil
		}
	}
	return 0, fmt.Errorf("no edge %q", s)
}

// Move moves one edge of the print's region by mm, as Box.Move does, and
// the print's own edge with it, the margin inside the region's: an edge is
// moved because the finder put the print's edge in the wrong place, in
// when it took bed for print, out when it cut the print short. A print
// turned past its region, over 10°, keeps the finder's extent: it is
// flagged to be straightened on the bed.
func (p Print) Move(e Edge, mm float64, bed Rect) (Print, error) {
	r, err := p.Region.Move(e, mm, bed)
	if err != nil {
		return p, err
	}
	p.Region = r
	if p.Print.Angle != r.Angle {
		return p, nil
	}
	ext := p.Print.Within(r)
	switch e {
	case Top:
		ext.Y0 = marginMM
	case Bottom:
		ext.Y1 = r.H - marginMM
	case Left:
		ext.X0 = marginMM
	case Right:
		ext.X1 = r.W - marginMM
	}
	cx, cy := r.At((ext.X0+ext.X1)/2, (ext.Y0+ext.Y1)/2)
	p.Print = Box{CX: cx, CY: cy, W: ext.X1 - ext.X0, H: ext.Y1 - ext.Y0, Angle: r.Angle}
	return p, nil
}
