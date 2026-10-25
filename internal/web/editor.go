package web

import (
	"fmt"
	"math"
	"strconv"
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

// regionEditor is one print's close-up with rulers along its top and
// left and arrows on its edges, at confirm. Adjusting ends on the last
// print, so that box offers Confirm and scan beside its own buttons.
func regionEditor(b *batch.Scan, i int, p find.Print, last bool) string {
	n := i + 1
	id := scanURL(b.Key())
	img := fmt.Sprintf(`<img src="%s/regions/%d.png" alt="Print %d's region, close up" style="display:block">`, id, n, n)
	if f, err := b.CloseUpFrame(i); err == nil && !p.Straighten() {
		img = ruled(n, p, b.Bed(), f, img)
	}
	return fmt.Sprintf(`<div class="box" id="print-%d">
<h2 class="title is-5">Print %d: %.1f × %.1f mm</h2>
%s
<p class="help">Close-up of the 150 ppi preview%s: the region in green, the print as found in blue. A tick on a ruler sets the nearer edge to it, every %.0f mm; single arrows move an edge %.1f mm, double arrows %.0f mm.</p>
<div class="buttons mt-3">%s%s%s</div>
</div>`, n, n, p.Print.W, p.Print.H,
		edgeGrid(id+"/adjust", n, p, b.Bed(), img),
		turnedBack(p.Region), tickMM, smallStep, largeStep, confirmButton(id, last), splitButton(id, n, p), regionButton(id, "remove", n, "Remove this region", "is-danger is-light"))
}

// confirmButton is the last print's Confirm and scan, the same step as
// the state bar's, where the adjusting ends.
func confirmButton(url string, last bool) string {
	if !last {
		return ""
	}
	return fmt.Sprintf(`<form action="%s/confirm" method="post" style="display:inline"><button class="button is-small is-primary" type="submit">Confirm and scan ›</button></form> `, url)
}

// Rulers: a tick every tickMM, labelled every labelMM, in a band rulerPx
// wide along the close-up's top and left.
const (
	tickMM  = 5.0
	labelMM = 10.0
	rulerPx = 26
)

// ruled lays rulers along the top and left of print n's close-up img: a
// tick every tickMM in mm of the bed (of the region's own frame for a
// turned print), each a button that sets the nearer edge on that axis to
// it, or a bare mark where the edge cannot go; the region's own edges
// are marked in green.
func ruled(n int, p find.Print, bed find.Rect, f batch.CloseUpFrame, img string) string {
	r := p.Region
	// the ruler's coordinate c and the region frame's u: c = u + c0
	cu, cv := 0.0, 0.0
	if r.Angle == 0 {
		cu, cv = r.CX-r.W/2, r.CY-r.H/2
	}
	var top, left strings.Builder
	ruler := func(b *strings.Builder, vertical bool, c0, u0, size float64, px int, near, far find.Edge) {
		first := math.Ceil((u0+c0)/tickMM) * tickMM
		last := math.Floor((u0+c0+float64(px)/f.Scale)/tickMM) * tickMM
		for c := first; c <= last+1e-9; c += tickMM {
			u := c - c0
			at := int(math.Round((u - u0) * f.Scale))
			e, mm := near, -u
			if u > size/2 {
				e, mm = far, u-size
			}
			label := ""
			if math.Mod(math.Abs(c), labelMM) < 1e-9 {
				label = strconv.FormatFloat(c, 'f', -1, 64)
			}
			pos := fmt.Sprintf("left:%dpx;top:0", at-5)
			if vertical {
				pos = fmt.Sprintf("top:%dpx;left:0", at-5)
			}
			if r.CanMove(e, mm, bed) {
				fmt.Fprintf(b, `<button class="button is-small is-white" style="position:absolute;%s;height:%dpx;width:10px;padding:0;font-size:9px;line-height:1" name="move" value="%d %s =%g" title="%s edge to %g">|<br>%s</button>`,
					pos, rulerPx, n, e, u, e, c, label)
			} else {
				fmt.Fprintf(b, `<span style="position:absolute;%s;height:%dpx;width:10px;font-size:9px;line-height:1;color:#999;text-align:center">|<br>%s</span>`, pos, rulerPx, label)
			}
		}
		// the region's edges
		for _, u := range []float64{0, size} {
			at := int(math.Round((u - u0) * f.Scale))
			pos := fmt.Sprintf("left:%dpx;top:0;width:2px;height:%dpx", at-1, rulerPx)
			if vertical {
				pos = fmt.Sprintf("top:%dpx;left:0;height:2px;width:%dpx", at-1, rulerPx)
			}
			fmt.Fprintf(b, `<span style="position:absolute;%s;background:#00a000"></span>`, pos)
		}
	}
	ruler(&top, false, cu, f.U0, r.W, f.W, find.Left, find.Right)
	ruler(&left, true, cv, f.V0, r.H, f.H, find.Top, find.Bottom)
	return fmt.Sprintf(`<div style="display:grid;grid-template-columns:%dpx %dpx;grid-template-rows:%dpx %dpx">
<div></div><div style="position:relative">%s</div>
<div style="position:relative">%s</div><div>%s</div></div>`, rulerPx, f.W, rulerPx, f.H, top.String(), left.String(), img)
}

// turnedBack says how far a close-up is turned to sit the print square.
func turnedBack(region find.Box) string {
	if region.Angle == 0 {
		return ""
	}
	return fmt.Sprintf(", turned back %.1f° so the print sits square", math.Abs(region.Angle))
}

// regionButton posts an edit of print n to the scan at url.
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
// rescans the bed.
func rescanEditor(b *batch.Scan, i int, p find.Print) string {
	return fmt.Sprintf(`<div class="box"><h2 class="title is-6">Margins</h2>
%s
<p class="help">Each arrow moves that edge of the region and rescans the bed at %d ppi.</p></div>`,
		edgeGrid(scanURL(b.Key())+"/reshape", i+1, p, b.Bed(),
			fmt.Sprintf(`<p class="has-text-centered">print %d<br>%.1f × %.1f mm</p>`, i+1, p.Print.W, p.Print.H)),
		b.Sitting().ScanPPI)
}
