package find

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

var theBed = Rect{0, 0, 215.9, 297.2}

// square is a region square to the bed, as a box.
func square(x0, y0, x1, y1 float64) Box { return BoxOf(Rect{x0, y0, x1, y1}) }

func checkBox(t *testing.T, what string, got, want Box) {
	t.Helper()
	const eps = 1e-9
	if math.Abs(got.CX-want.CX) > eps || math.Abs(got.CY-want.CY) > eps || math.Abs(got.W-want.W) > eps ||
		math.Abs(got.H-want.H) > eps || math.Abs(got.Angle-want.Angle) > eps {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

func TestMoveAnEdge(t *testing.T) {
	r := square(10, 20, 110, 90)
	cases := []struct {
		edge Edge
		by   float64
		want Box
	}{
		{Top, 0.5, square(10, 19.5, 110, 90)},    // outward is up
		{Top, -3, square(10, 23, 110, 90)},       // inward is down
		{Bottom, 3, square(10, 20, 110, 93)},     // outward is down
		{Left, 0.5, square(9.5, 20, 110, 90)},    // outward is left
		{Right, -0.5, square(10, 20, 109.5, 90)}, // inward is left
	}
	for _, tc := range cases {
		got, err := r.Move(tc.edge, tc.by, theBed)
		if err != nil {
			t.Errorf("%s by %+v: %v", tc.edge, tc.by, err)
		}
		checkBox(t, fmt.Sprintf("%s by %+v", tc.edge, tc.by), got, tc.want)
	}
}

func TestAMoveOutwardStopsAtTheBed(t *testing.T) {
	got, err := square(1, 20, 110, 90).Move(Left, 3, theBed)
	if err != nil {
		t.Fatal(err)
	}
	checkBox(t, "3 mm out from 1 mm", got, square(0, 20, 110, 90))
}

// Story 004: a turned region's edges are the print's own.
func TestATurnedRegionMovesAlongThePrintsAxes(t *testing.T) {
	r := Box{CX: 100, CY: 100, W: 120, H: 80, Angle: 30}
	got, err := r.Move(Right, 3, theBed)
	if err != nil {
		t.Fatal(err)
	}
	s, c := math.Sincos(30 * math.Pi / 180)
	checkBox(t, "right edge out 3 mm", got, Box{CX: 100 + 1.5*c, CY: 100 + 1.5*s, W: 123, H: 80, Angle: 30})
	got, err = r.Move(Top, -2, theBed)
	if err != nil {
		t.Fatal(err)
	}
	// the top edge's inward normal is the print's down, (-sin, cos) on the bed
	checkBox(t, "top edge in 2 mm", got, Box{CX: 100 - 1*s, CY: 100 + 1*c, W: 120, H: 78, Angle: 30})
}

func TestATurnedRegionsBoundsCoverItsCorners(t *testing.T) {
	r := Box{CX: 100, CY: 100, W: 120, H: 80, Angle: 30}
	s, c := math.Sincos(30 * math.Pi / 180)
	hw, hh := 60*c+40*s, 60*s+40*c
	b := r.Bounds()
	want := Rect{100 - hw, 100 - hh, 100 + hw, 100 + hh}
	if math.Abs(b.X0-want.X0)+math.Abs(b.Y0-want.Y0)+math.Abs(b.X1-want.X1)+math.Abs(b.Y1-want.Y1) > 1e-9 {
		t.Errorf("bounds %+v, want %+v", b, want)
	}
	corners := r.Corners()
	// top left, then clockwise; the top edge runs along the print's x axis
	if math.Abs(corners[1][0]-corners[0][0]-120*c) > 1e-9 || math.Abs(corners[1][1]-corners[0][1]-120*s) > 1e-9 {
		t.Errorf("top edge %v → %v is not 120 mm along 30°", corners[0], corners[1])
	}
	if !r.Contains(100, 100) || r.Contains(want.X0+0.1, want.Y0+0.1) {
		t.Error("Contains: the centre is in, the bounds' corner is not")
	}
	edge := square(0, 0, 20, 20).Bounds()
	if edge != (Rect{0, 0, 20, 20}) {
		t.Errorf("a square box's bounds are its rectangle, got %+v", edge)
	}
	if got := (Box{CX: 5, CY: 5, W: 20, H: 20, Angle: 10}).On(theBed); got.X0 != 0 || got.Y0 != 0 {
		t.Errorf("bounds on the bed stop at its edge, got %+v", got)
	}
}

func TestAPrintsExtentWithinItsRegion(t *testing.T) {
	region := Box{CX: 100, CY: 100, W: 121, H: 81, Angle: 5}
	s, c := math.Sincos(5 * math.Pi / 180)
	print := Box{CX: 100 + 0.25*c, CY: 100 + 0.25*s, W: 120, H: 80, Angle: 5} // 0.25 mm towards the print's right
	got := print.Within(region)
	want := Rect{0.75, 0.5, 120.75, 80.5}
	if math.Abs(got.X0-want.X0)+math.Abs(got.Y0-want.Y0)+math.Abs(got.X1-want.X1)+math.Abs(got.Y1-want.Y1) > 1e-9 {
		t.Errorf("within = %+v, want %+v", got, want)
	}
}

// Story 002: no arrow where there is no adjustment.
func TestWhichMovesThereAre(t *testing.T) {
	atCorner := square(0, 0, 120, 21)
	cases := []struct {
		r    Box
		edge Edge
		by   float64
		can  bool
	}{
		{atCorner, Top, 0.5, false}, // already at the bed's edge
		{atCorner, Left, 3, false},  // likewise
		{atCorner, Right, 3, true},
		{atCorner, Bottom, -0.5, true}, // 20.5 mm high is still enough
		{atCorner, Bottom, -3, false},  // 18 mm is under the 20 mm minimum
		{atCorner, Top, -3, false},
		{square(10, 20, 215.9, 90), Right, 0.5, false},
		{square(10, 20, 215.7, 90), Right, 3, true}, // stops at the bed
	}
	for _, tc := range cases {
		if got := tc.r.CanMove(tc.edge, tc.by, theBed); got != tc.can {
			t.Errorf("%+v %s by %+v: can = %v, want %v", tc.r, tc.edge, tc.by, got, tc.can)
		}
	}
	if _, err := atCorner.Move(Bottom, -3, theBed); err == nil || !strings.Contains(err.Error(), "20 mm") {
		t.Errorf("move under the minimum: %v", err)
	}
	if _, err := atCorner.Move(Top, 0.5, theBed); err == nil {
		t.Error("move past the bed's edge accepted")
	}
}

func TestParseEdge(t *testing.T) {
	for _, e := range []Edge{Top, Bottom, Left, Right} {
		if got, err := ParseEdge(e.String()); err != nil || got != e {
			t.Errorf("ParseEdge(%q) = %v, %v", e, got, err)
		}
	}
	if _, err := ParseEdge("middle"); err == nil {
		t.Error("ParseEdge(middle) succeeded")
	}
}

// Inside is what the quality check measures within a turned print.
func TestInsideIsSquareToTheBedWithinTheBox(t *testing.T) {
	b := Box{CX: 100, CY: 100, W: 120, H: 80, Angle: 8}
	in := b.Inside()
	for _, c := range [][2]float64{{in.X0, in.Y0}, {in.X1, in.Y0}, {in.X1, in.Y1}, {in.X0, in.Y1}} {
		if !b.Contains(c[0]+1e-9*(100-c[0]), c[1]+1e-9*(100-c[1])) {
			t.Errorf("corner %v of %+v is outside the box", c, in)
		}
	}
	if w, h := in.X1-in.X0, in.Y1-in.Y0; w < 100 || h < 60 {
		t.Errorf("inside %.1f × %.1f mm; want most of the 120 × 80 box", w, h)
	}
	if sq := square(10, 20, 110, 90); sq.Inside() != sq.Bounds() {
		t.Errorf("a box square to the bed is all inside")
	}
}
