package web

import (
	"fmt"
	"math"
	"strings"

	"git.bytestone.uk/hum3/bytestone-scan/internal/batch"
	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// Arrow steps, in mm.
const (
	smallStep = 0.5
	largeStep = 3.0
)

// arrows are each edge's buttons, outermost first: large out, small out,
// small in, large in.
var arrows = map[find.Edge][4]string{
	find.Top:    {"⇈", "↑", "↓", "⇊"},
	find.Bottom: {"⇊", "↓", "↑", "⇈"},
	find.Left:   {"⇇", "←", "→", "⇉"},
	find.Right:  {"⇉", "→", "←", "⇇"},
}

// edgeArrows are the arrows on one edge of print n's region, each a
// button naming the move; an arrow is left out where its move cannot
// happen. Below them is the edge's margin to the print. Edges are the
// print's own, however it is turned.
func edgeArrows(n int, p find.Print, bed find.Rect, e find.Edge, vertical bool) string {
	in := p.Print.Within(p.Region)
	margin := map[find.Edge]float64{
		find.Top: in.Y0, find.Bottom: p.Region.H - in.Y1,
		find.Left: in.X0, find.Right: p.Region.W - in.X1,
	}
	var s strings.Builder
	cls := "buttons are-small has-addons"
	if vertical {
		cls += " is-flex-direction-column"
	}
	fmt.Fprintf(&s, `<div class="%s" style="justify-content:center">`, cls)
	for k, by := range []float64{largeStep, smallStep, -smallStep, -largeStep} {
		if !p.Region.CanMove(e, by, bed) {
			continue
		}
		dir := "out"
		if by < 0 {
			dir = "in"
		}
		fmt.Fprintf(&s, `<button class="button" name="move" value="%d %s %g" title="%s edge %s %g mm">%s</button>`,
			n, e, by, e, dir, max(by, -by), arrows[e][k])
	}
	fmt.Fprintf(&s, `</div><p class="help has-text-centered">margin %.1f mm</p>`, margin[e])
	return s.String()
}

// edgeGrid lays the four edges' arrows round middle, posting to action;
// a print turned too far to deskew has none, but a note to straighten it.
func edgeGrid(action string, n int, p find.Print, bed find.Rect, middle string) string {
	if p.Straighten() {
		return fmt.Sprintf(`%s<p class="notification is-warning is-light mt-2">Print %d is turned %.1f°, too far to deskew: straighten it on the bed and rescan the preview.</p>`,
			middle, n, p.Skew)
	}
	return fmt.Sprintf(`<form action="%s" method="post">
<div style="display:grid;grid-template-columns:auto auto auto;align-items:center;justify-content:start;gap:.5rem">
<div></div><div>%s</div><div></div>
<div>%s</div><div>%s</div><div>%s</div>
<div></div><div>%s</div><div></div>
</div></form>`, action,
		edgeArrows(n, p, bed, find.Top, false), edgeArrows(n, p, bed, find.Left, true), middle,
		edgeArrows(n, p, bed, find.Right, true), edgeArrows(n, p, bed, find.Bottom, false))
}

// regionEditor is one print's close-up with arrows on its edges, at confirm.
func regionEditor(b *batch.Load, i int, p find.Print) string {
	n := i + 1
	id := loadURL(b.Key())
	return fmt.Sprintf(`<div class="box" id="print-%d">
<h2 class="title is-5">Print %d: %.1f × %.1f mm</h2>
%s
<p class="help">Close-up of the 150 ppi preview%s: the region in green, the print as found in blue. Single arrows move an edge %.1f mm, double arrows %.0f mm.</p>
<div class="buttons mt-3">%s%s</div>
</div>`, n, n, p.Print.W, p.Print.H,
		edgeGrid(id+"/adjust", n, p, b.Bed(),
			fmt.Sprintf(`<img src="%s/regions/%d.png" alt="Print %d's region, close up">`, id, n, n)),
		turnedBack(p.Region), smallStep, largeStep, splitButton(id, n, p), regionButton(id, "remove", n, "Remove this region", "is-danger is-light"))
}

// turnedBack says how far a close-up is turned to sit the print square.
func turnedBack(region find.Box) string {
	if region.Angle == 0 {
		return ""
	}
	return fmt.Sprintf(", turned back %.1f° so the print sits square", math.Abs(region.Angle))
}

// regionButton posts an edit of print n to the load at url.
func regionButton(url, action string, n int, label, class string) string {
	return fmt.Sprintf(`<form action="%s/%s" method="post" style="display:inline"><input type="hidden" name="print" value="%d"><button class="button is-small %s" type="submit">%s</button></form> `,
		url, action, n, class, label)
}

// splitButton is offered only on a region holding more than one print.
func splitButton(url string, n int, p find.Print) string {
	if p.Parts < 2 {
		return ""
	}
	return regionButton(url, "split", n, fmt.Sprintf("Split into %d", p.Parts), "is-warning")
}

// rescanEditor is the same arrows on the quality page, where each move
// rescans the load.
func rescanEditor(b *batch.Load, i int, p find.Print) string {
	return fmt.Sprintf(`<div class="box"><h2 class="title is-6">Margins</h2>
%s
<p class="help">Each arrow moves that edge of the region and rescans the load at %d ppi.</p></div>`,
		edgeGrid(loadURL(b.Key())+"/reshape", i+1, p, b.Bed(),
			fmt.Sprintf(`<p class="has-text-centered">print %d<br>%.1f × %.1f mm</p>`, i+1, p.Print.W, p.Print.H)),
		b.Sitting().ScanPPI)
}
