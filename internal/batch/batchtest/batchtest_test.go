package batchtest

import (
	"context"
	"testing"
)

// Story 039: the fake serves its Prints until turned to the back, then its
// Backs; with no Backs given, the back of each print looks like its front.
func TestTheFakeServesTheSideTurnedToTheGlass(t *testing.T) {
	f := &Scanner{Prints: []Print{{X0: 20, Y0: 20, X1: 60, Y1: 50, Value: 20000}}, Backs: []Print{{X0: 25, Y0: 20, X1: 65, Y1: 50, Value: 50000}}}
	at := func(xmm, ymm float64) uint16 { return f.valueAt(xmm, ymm) }
	if at(22, 30) != 20000 || at(62, 30) != 65535 {
		t.Errorf("front: %d at 22 mm, %d at 62 mm", at(22, 30), at(62, 30))
	}
	f.TurnTo("back")
	if at(22, 30) != 65535 || at(62, 30) != 50000 {
		t.Errorf("back: %d at 22 mm, %d at 62 mm", at(22, 30), at(62, 30))
	}
	f.TurnTo("front")
	if at(22, 30) != 20000 {
		t.Errorf("front again: %d at 22 mm", at(22, 30))
	}
	g := &Scanner{Prints: f.Prints}
	g.TurnTo("back")
	if g.valueAt(22, 30) != 20000 {
		t.Errorf("no backs given: %d at 22 mm, want the front's value", g.valueAt(22, 30))
	}
	if _, err := g.Preview(context.Background()); err != nil {
		t.Error(err)
	}
}
