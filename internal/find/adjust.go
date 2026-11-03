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

// Divide cuts the print's region into across × down equal parts along
// its own axes, numbered in rows, for the prints the finder cannot tell
// apart: a contact print, a strip of frames, prints butted together.
// Each part's region is its cell; its print keeps the original print's
// edge on the outside and sits the margin inside a cut edge, as a moved
// edge does. The parts keep the skew and are not for review. One part,
// or a part under the smallest print, is refused.
func (p Print) Divide(across, down int) ([]Print, error) {
	if across < 1 || down < 1 || across*down < 2 {
		return nil, fmt.Errorf("split into %d across and %d down: nothing to split", across, down)
	}
	r := p.Region
	cw, ch := r.W/float64(across), r.H/float64(down)
	if cw < minSizeMM || ch < minSizeMM {
		return nil, fmt.Errorf("split into %d across and %d down: each part would be %.0f × %.0f mm, under %.0f mm", across, down, cw, ch, minSizeMM)
	}
	// the print's extent in the region's frame; a print turned past its
	// region keeps nothing of the finder's extent but the margin
	ext := Rect{marginMM, marginMM, r.W - marginMM, r.H - marginMM}
	if p.Print.Angle == r.Angle {
		ext = p.Print.Within(r)
	}
	var parts []Print
	for j := range down {
		for i := range across {
			cell := Rect{float64(i) * cw, float64(j) * ch, float64(i+1) * cw, float64(j+1) * ch}
			in := cell
			if i == 0 {
				in.X0 = max(ext.X0, cell.X0)
			} else {
				in.X0 += marginMM
			}
			if i == across-1 {
				in.X1 = min(ext.X1, cell.X1)
			} else {
				in.X1 -= marginMM
			}
			if j == 0 {
				in.Y0 = max(ext.Y0, cell.Y0)
			} else {
				in.Y0 += marginMM
			}
			if j == down-1 {
				in.Y1 = min(ext.Y1, cell.Y1)
			} else {
				in.Y1 -= marginMM
			}
			part := p
			part.Region = boxIn(r, cell)
			part.Print = boxIn(r, in)
			part.Parts, part.Review = 0, false
			parts = append(parts, part)
		}
	}
	return parts, nil
}

// boxIn is the rectangle e, in frame's own axes, as a box on the bed at
// frame's angle.
func boxIn(frame Box, e Rect) Box {
	cx, cy := frame.At((e.X0+e.X1)/2, (e.Y0+e.Y1)/2)
	return Box{CX: cx, CY: cy, W: e.X1 - e.X0, H: e.Y1 - e.Y0, Angle: frame.Angle}
}
