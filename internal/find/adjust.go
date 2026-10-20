package find

import (
	"errors"
	"fmt"
)

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

// Move moves one edge of r by mm, outwards when mm is positive, stopping at
// the bed. It refuses a move that cannot happen: outwards from the bed's
// edge, or inwards to leave a side under the smallest print.
func (r Rect) Move(e Edge, mm float64, bed Rect) (Rect, error) {
	out := r
	switch e {
	case Top:
		out.Y0 = max(bed.Y0, r.Y0-mm)
	case Bottom:
		out.Y1 = min(bed.Y1, r.Y1+mm)
	case Left:
		out.X0 = max(bed.X0, r.X0-mm)
	case Right:
		out.X1 = min(bed.X1, r.X1+mm)
	}
	if out == r {
		return r, fmt.Errorf("the %s edge is at the bed's edge", e)
	}
	if out.X1-out.X0 < minSizeMM || out.Y1-out.Y0 < minSizeMM {
		return r, errors.New("a region must be at least 20 mm each way")
	}
	return out, nil
}

// CanMove reports whether Move would move the edge.
func (r Rect) CanMove(e Edge, mm float64, bed Rect) bool {
	_, err := r.Move(e, mm, bed)
	return err == nil
}
