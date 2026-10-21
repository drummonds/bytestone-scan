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
