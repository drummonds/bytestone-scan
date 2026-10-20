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

// regionEditor is one print's close-up with arrows on its edges, each an
// edge moved by a step; an arrow is left out where its move cannot happen.
func regionEditor(b *batch.Batch, i int, p find.Print) string {
	n := i + 1
	bed := b.Bed()
	margin := map[find.Edge]float64{
		find.Top: p.Print.Y0 - p.Region.Y0, find.Bottom: p.Region.Y1 - p.Print.Y1,
		find.Left: p.Print.X0 - p.Region.X0, find.Right: p.Region.X1 - p.Print.X1,
	}
	buttons := func(e find.Edge, vertical bool) string {
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
	return fmt.Sprintf(`<div class="box" id="print-%d">
<h2 class="title is-5">Print %d: %.1f × %.1f mm</h2>
<form action="batches/%s/adjust" method="post">
<div style="display:grid;grid-template-columns:auto auto auto;align-items:center;justify-content:start;gap:.5rem">
<div></div><div>%s</div><div></div>
<div>%s</div><div><img src="batches/%s/regions/%d.png" alt="Print %d's region, close up"></div><div>%s</div>
<div></div><div>%s</div><div></div>
</div></form>
<p class="help">Close-up of the 75 ppi preview: the region in green, the print as found in blue. Single arrows move an edge %.1f mm, double arrows %.0f mm.</p>
</div>`, n, n, p.Print.X1-p.Print.X0, p.Print.Y1-p.Print.Y0, html.EscapeString(b.ID),
		buttons(find.Top, false), buttons(find.Left, true), html.EscapeString(b.ID), n, n, buttons(find.Right, true),
		buttons(find.Bottom, false), smallStep, largeStep)
}
