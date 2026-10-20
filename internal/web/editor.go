package web

import (
	"fmt"
	"html"
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
// happen. Below them is the edge's margin to the print.
func edgeArrows(n int, p find.Print, bed find.Rect, e find.Edge, vertical bool) string {
	margin := map[find.Edge]float64{
		find.Top: p.Print.Y0 - p.Region.Y0, find.Bottom: p.Region.Y1 - p.Print.Y1,
		find.Left: p.Print.X0 - p.Region.X0, find.Right: p.Region.X1 - p.Print.X1,
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

// edgeGrid lays the four edges' arrows round middle, posting to action.
func edgeGrid(action string, n int, p find.Print, bed find.Rect, middle string) string {
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
func regionEditor(b *batch.Batch, i int, p find.Print) string {
	n := i + 1
	id := html.EscapeString(b.ID)
	return fmt.Sprintf(`<div class="box" id="print-%d">
<h2 class="title is-5">Print %d: %.1f × %.1f mm</h2>
%s
<p class="help">Close-up of the 75 ppi preview: the region in green, the print as found in blue. Single arrows move an edge %.1f mm, double arrows %.0f mm.</p>
</div>`, n, n, p.Print.X1-p.Print.X0, p.Print.Y1-p.Print.Y0,
		edgeGrid("batches/"+id+"/adjust", n, p, b.Bed(),
			fmt.Sprintf(`<img src="batches/%s/regions/%d.png" alt="Print %d's region, close up">`, id, n, n)),
		smallStep, largeStep)
}

// rescanEditor is the same arrows on the quality page, where each move
// rescans the batch.
func rescanEditor(b *batch.Batch, i int, p find.Print) string {
	return fmt.Sprintf(`<div class="box"><h2 class="title is-6">Margins</h2>
%s
<p class="help">Each arrow moves that edge of the region and rescans the batch at %d ppi.</p></div>`,
		edgeGrid("batches/"+html.EscapeString(b.ID)+"/reshape", i+1, p, b.Bed(),
			fmt.Sprintf(`<p class="has-text-centered">print %d<br>%.1f × %.1f mm</p>`, i+1, p.Print.X1-p.Print.X0, p.Print.Y1-p.Print.Y0)),
		b.ScanPPI)
}
