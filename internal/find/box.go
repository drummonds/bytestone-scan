package find

import (
	"errors"
	"fmt"
	"math"
)

// Box is a rectangle square to a print rather than to the bed: its centre
// on the bed and its size along the print's own axes, in mm, and the angle
// in degrees its top edge is turned clockwise from the bed's, as the
// preview is seen.
type Box struct {
	CX, CY float64
	W, H   float64
	Angle  float64
}

// BoxOf is r as a box square to the bed.
func BoxOf(r Rect) Box {
	return Box{CX: (r.X0 + r.X1) / 2, CY: (r.Y0 + r.Y1) / 2, W: r.X1 - r.X0, H: r.Y1 - r.Y0}
}

// axes are the print's x and y axes on the bed.
func (b Box) axes() (ux, uy, vx, vy float64) {
	s, c := math.Sincos(b.Angle * math.Pi / 180)
	return c, s, -s, c
}

// At is the point on the bed u, v mm from the box's top left corner along
// its own axes.
func (b Box) At(u, v float64) (x, y float64) {
	ux, uy, vx, vy := b.axes()
	u, v = u-b.W/2, v-b.H/2
	return b.CX + u*ux + v*vx, b.CY + u*uy + v*vy
}

// In is the bed point x, y in the box's own axes, from its top left corner:
// the inverse of At.
func (b Box) In(x, y float64) (u, v float64) {
	ux, uy, vx, vy := b.axes()
	dx, dy := x-b.CX, y-b.CY
	return dx*ux + dy*uy + b.W/2, dx*vx + dy*vy + b.H/2
}

// Corners are the box's corners on the bed: top left, then clockwise.
func (b Box) Corners() [4][2]float64 {
	var c [4][2]float64
	for i, uv := range [4][2]float64{{0, 0}, {b.W, 0}, {b.W, b.H}, {0, b.H}} {
		c[i][0], c[i][1] = b.At(uv[0], uv[1])
	}
	return c
}

// Bounds is the smallest rectangle square to the bed that covers the box.
func (b Box) Bounds() Rect {
	if b.Angle == 0 {
		return Rect{b.CX - b.W/2, b.CY - b.H/2, b.CX + b.W/2, b.CY + b.H/2}
	}
	r := Rect{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, c := range b.Corners() {
		r = Rect{min(r.X0, c[0]), min(r.Y0, c[1]), max(r.X1, c[0]), max(r.Y1, c[1])}
	}
	return r
}

// On is the box's bounds within the bed: what a scan of it covers.
func (b Box) On(bed Rect) Rect {
	r := b.Bounds()
	return Rect{max(r.X0, bed.X0), max(r.Y0, bed.Y0), min(r.X1, bed.X1), min(r.Y1, bed.Y1)}
}

// Inside is a rectangle square to the bed within the box: the box's own
// shape about its centre, as large as fits.
func (b Box) Inside() Rect {
	s, c := math.Sincos(b.Angle * math.Pi / 180)
	s, c = math.Abs(s), math.Abs(c)
	k := min(1, b.W/(b.W*c+b.H*s), b.H/(b.W*s+b.H*c))
	return Rect{b.CX - k*b.W/2, b.CY - k*b.H/2, b.CX + k*b.W/2, b.CY + k*b.H/2}
}

// Contains reports whether the bed point x, y lies within the box.
func (b Box) Contains(x, y float64) bool {
	u, v := b.In(x, y)
	return u >= 0 && u <= b.W && v >= 0 && v <= b.H
}

// InImage is b in the pixels of an image covering frame at ppi, from the
// image's top left corner.
func (b Box) InImage(frame Rect, ppi int) Box {
	k := float64(ppi) / 25.4
	return Box{CX: (b.CX - frame.X0) * k, CY: (b.CY - frame.Y0) * k, W: b.W * k, H: b.H * k, Angle: b.Angle}
}

// Within is b's extent in frame's own axes, from frame's top left corner;
// b is taken to lie at frame's angle.
func (b Box) Within(frame Box) Rect {
	u, v := frame.In(b.CX, b.CY)
	return Rect{u - b.W/2, v - b.H/2, u + b.W/2, v + b.H/2}
}

// Move moves one edge of b by mm along the print's axis, outwards when mm
// is positive; the opposite edge stays. Outwards it stops when the edge's
// midpoint reaches the bed's edge. It refuses a move that cannot happen:
// outwards from the bed's edge, or inwards to leave a side under the
// smallest print.
func (b Box) Move(e Edge, mm float64, bed Rect) (Box, error) {
	ux, uy, vx, vy := b.axes()
	// the edge's outward normal on the bed, and its midpoint
	var nx, ny, half float64
	switch e {
	case Top:
		nx, ny, half = -vx, -vy, b.H/2
	case Bottom:
		nx, ny, half = vx, vy, b.H/2
	case Left:
		nx, ny, half = -ux, -uy, b.W/2
	case Right:
		nx, ny, half = ux, uy, b.W/2
	}
	mx, my := b.CX+half*nx, b.CY+half*ny
	if mm > 0 {
		mm = min(mm, reach(mx, nx, bed.X0, bed.X1), reach(my, ny, bed.Y0, bed.Y1))
	}
	if mm <= 0 && mm > -1e-9 {
		return b, fmt.Errorf("the %s edge is at the bed's edge", e)
	}
	out := b
	out.CX, out.CY = b.CX+mm/2*nx, b.CY+mm/2*ny
	if e == Top || e == Bottom {
		out.H += mm
	} else {
		out.W += mm
	}
	if out.W < minSizeMM || out.H < minSizeMM {
		return b, errors.New("a region must be at least 20 mm each way")
	}
	return out, nil
}

// reach is how far a point at p can go along direction d before leaving
// lo..hi.
func reach(p, d, lo, hi float64) float64 {
	const tiny = 1e-12
	switch {
	case d > tiny:
		return max(0, (hi-p)/d)
	case d < -tiny:
		return max(0, (lo-p)/d)
	}
	return math.Inf(1)
}

// CanMove reports whether Move would move the edge.
func (b Box) CanMove(e Edge, mm float64, bed Rect) bool {
	_, err := b.Move(e, mm, bed)
	return err == nil
}
