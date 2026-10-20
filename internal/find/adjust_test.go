package find

import (
	"strings"
	"testing"
)

var theBed = Rect{0, 0, 215.9, 297.2}

func TestMoveAnEdge(t *testing.T) {
	r := Rect{10, 20, 110, 90}
	cases := []struct {
		edge Edge
		by   float64
		want Rect
	}{
		{Top, 0.5, Rect{10, 19.5, 110, 90}},    // outward is up
		{Top, -3, Rect{10, 23, 110, 90}},       // inward is down
		{Bottom, 3, Rect{10, 20, 110, 93}},     // outward is down
		{Left, 0.5, Rect{9.5, 20, 110, 90}},    // outward is left
		{Right, -0.5, Rect{10, 20, 109.5, 90}}, // inward is left
	}
	for _, tc := range cases {
		got, err := r.Move(tc.edge, tc.by, theBed)
		if err != nil || got != tc.want {
			t.Errorf("%s by %+v: %+v, %v; want %+v", tc.edge, tc.by, got, err, tc.want)
		}
	}
}

func TestAMoveOutwardStopsAtTheBed(t *testing.T) {
	r := Rect{1, 20, 110, 90}
	got, err := r.Move(Left, 3, theBed)
	if err != nil || got.X0 != 0 {
		t.Errorf("3 mm out from 1 mm: %+v, %v; want the bed's edge", got, err)
	}
}

// Story 002: no arrow where there is no adjustment.
func TestWhichMovesThereAre(t *testing.T) {
	atCorner := Rect{0, 0, 120, 21}
	cases := []struct {
		r    Rect
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
		{Rect{10, 20, 215.9, 90}, Right, 0.5, false},
		{Rect{10, 20, 215.7, 90}, Right, 3, true}, // stops at the bed
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
