package batch

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// annotate draws each print's region on the preview with its number:
// green when it looks right, red when it needs review.
func annotate(preview image.Image, prints []find.Print, ppi int) *image.RGBA {
	out := image.NewRGBA(preview.Bounds())
	draw.Draw(out, out.Bounds(), preview, preview.Bounds().Min, draw.Src)
	mm := 25.4 / float64(ppi)
	px := func(v float64) int { return int(v / mm) }
	for i, p := range prints {
		col := color.RGBA{0, 160, 0, 255}
		if p.Review {
			col = color.RGBA{220, 0, 0, 255}
		}
		corners := p.Region.Corners()
		for k := range corners {
			a, b := corners[k], corners[(k+1)%len(corners)]
			line(out, a[0]/mm, a[1]/mm, b[0]/mm, b[1]/mm, col)
		}
		bounds := p.Region.Bounds()
		r := image.Rect(px(bounds.X0), px(bounds.Y0), px(bounds.X1), px(bounds.Y1)).Intersect(out.Bounds())
		label := strconv.Itoa(i + 1)
		if p.Review {
			label += " review"
		}
		d := font.Drawer{Dst: out, Src: image.NewUniform(col), Face: basicfont.Face7x13,
			Dot: fixed.P(r.Min.X+6, r.Min.Y+16)}
		d.DrawString(label)
	}
	return out
}

// line draws a line 2 px wide from x0, y0 to x1, y1, in pixels.
func line(img *image.RGBA, x0, y0, x1, y1 float64, col color.RGBA) {
	n := int(max(math.Abs(x1-x0), math.Abs(y1-y0))) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x, y := int(x0+t*(x1-x0)), int(y0+t*(y1-y0))
		for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
			if image.Pt(x+d[0], y+d[1]).In(img.Bounds()) {
				img.Set(x+d[0], y+d[1], col)
			}
		}
	}
}

// cutMM is the margin round the regions the preview is cut to once the
// prints are confirmed (#64).
const cutMM = 10.0

// cut is the annotated preview as the scan page shows it: the whole bed
// until the prints are confirmed, since the operator is judging what the
// finder missed as much as what it found; from confirm on, the rectangle
// that covers every region with cutMM round it, square to the bed, so the
// page is not mostly blank bed. The stored preview is untouched.
func (b *Scan) cut(img *image.RGBA, prints []find.Print) image.Image {
	if !b.Confirmed() || len(prints) == 0 {
		return img
	}
	cover := prints[0].Region.Bounds()
	for _, p := range prints[1:] {
		r := p.Region.Bounds()
		cover = find.Rect{X0: min(cover.X0, r.X0), Y0: min(cover.Y0, r.Y0), X1: max(cover.X1, r.X1), Y1: max(cover.Y1, r.Y1)}
	}
	mm := 25.4 / float64(PreviewPPI)
	px := func(v float64) int { return int(v / mm) }
	return img.SubImage(image.Rect(px(cover.X0-cutMM), px(cover.Y0-cutMM), px(cover.X1+cutMM), px(cover.Y1+cutMM)).Intersect(img.Bounds()))
}

// Preview is the batch's preview of the bed.
func (b *Scan) Preview() (image.Image, error) { return readPNG(b.path("preview.png")) }

// Annotated is the preview with each print's region as it stands, cut to
// the regions once they are confirmed.
func (b *Scan) Annotated() (image.Image, error) {
	img, err := b.Preview()
	if err != nil {
		return nil, err
	}
	prints, _, err := b.Found()
	if err != nil {
		return nil, err
	}
	return b.cut(annotate(img, prints, PreviewPPI), prints), nil
}

// AnnotatedBacks is the backs preview with the fronts' regions drawn on
// it, where each back is scanned, and each back as found in blue; cut to
// the regions like the preview.
func (b *Scan) AnnotatedBacks() (image.Image, error) {
	img, err := readPNG(b.path("backs.png"))
	if err != nil {
		return nil, err
	}
	prints, _, err := b.Found()
	if err != nil {
		return nil, err
	}
	out := annotate(img, prints, PreviewPPI)
	if backs, err := b.BacksFound(); err == nil {
		mm := 25.4 / float64(PreviewPPI)
		for _, s := range backs {
			corners := s.Print.Corners()
			for k := range corners {
				p, q := corners[k], corners[(k+1)%len(corners)]
				line(out, p[0]/mm, p[1]/mm, q[0]/mm, q[1]/mm, color.RGBA{0, 90, 220, 255})
			}
		}
	}
	return b.cut(out, prints), nil
}

// closeUpWidth is the width a close-up is enlarged towards, at most 4×.
const closeUpWidth = 480

// closeUpContext is how far round the region a close-up shows, mm.
const closeUpContext = 6.0

// closeUpView is the box of the bed a close-up of p shows, the
// enlargement k, and the view's size in preview pixels.
func closeUpView(p find.Print, bed find.Rect) (view find.Box, k, w, h int) {
	const mm = 25.4 / PreviewPPI
	view = p.Region
	view.W += 2 * closeUpContext
	view.H += 2 * closeUpContext
	if view.Angle == 0 {
		view = find.BoxOf(view.On(bed)) // square to the bed, it stops at the bed as before
	}
	w, h = int(view.W/mm), int(view.H/mm)
	k = max(1, min(4, closeUpWidth/max(1, w)))
	return view, k, w, h
}

// CloseUpFrame is where print n's close-up lies in the region's own
// frame: the image's size in pixels, its scale in pixels per mm, and the
// region-frame coordinates (u along the region's width from its left
// edge, v down from its top) at the image's top left corner.
type CloseUpFrame struct {
	W, H   int
	Scale  float64
	U0, V0 float64
}

// CloseUpFrame is the frame of print n's close-up, for drawing on it.
func (b *Scan) CloseUpFrame(n int) (CloseUpFrame, error) {
	prints, _, err := b.Found()
	if err != nil {
		return CloseUpFrame{}, err
	}
	if n < 0 || n >= len(prints) {
		return CloseUpFrame{}, fmt.Errorf("scan %s has no print %d", b.Key(), n+1)
	}
	const mm = 25.4 / PreviewPPI
	p := prints[n]
	view, k, w, h := closeUpView(p, b.Bed())
	f := CloseUpFrame{W: w * k, H: h * k, Scale: float64(k) / mm, U0: -closeUpContext, V0: -closeUpContext}
	if p.Region.Angle == 0 {
		// clamped to the bed: the image may start short of the context
		f.U0 = view.CX - view.W/2 - (p.Region.CX - p.Region.W/2)
		f.V0 = view.CY - view.H/2 - (p.Region.CY - p.Region.H/2)
	}
	return f, nil
}

// CloseUp is print n's region on the preview with 6 mm round it, turned
// so the print sits square and enlarged pixel for pixel: the region
// outlined in green, the print as found in blue.
func (b *Scan) CloseUp(n int) (image.Image, error) {
	img, err := b.Preview()
	if err != nil {
		return nil, err
	}
	prints, _, err := b.Found()
	if err != nil {
		return nil, err
	}
	if n < 0 || n >= len(prints) {
		return nil, fmt.Errorf("scan %s has no print %d", b.Key(), n+1)
	}
	const mm = 25.4 / PreviewPPI
	p := prints[n]
	view, k, w, h := closeUpView(p, b.Bed())
	out := image.NewRGBA(image.Rect(0, 0, w*k, h*k))
	off := color.RGBA{200, 200, 200, 255} // beyond the bed
	for y := range h {
		for x := range w {
			bx, by := view.At((float64(x)+0.5)*mm, (float64(y)+0.5)*mm)
			at := image.Pt(int(math.Floor(bx/mm)), int(math.Floor(by/mm)))
			var c color.Color = off
			if at.In(img.Bounds()) {
				c = img.At(at.X, at.Y)
			}
			for dy := range k {
				for dx := range k {
					out.Set(x*k+dx, y*k+dy, c)
				}
			}
		}
	}
	outline := func(r find.Rect, col color.RGBA) {
		px := func(v float64) int { return int(v / mm * float64(k)) }
		box := image.Rect(px(r.X0), px(r.Y0), px(r.X1), px(r.Y1)).Intersect(out.Bounds())
		for x := box.Min.X; x < box.Max.X; x++ {
			out.Set(x, box.Min.Y, col)
			out.Set(x, box.Max.Y-1, col)
		}
		for y := box.Min.Y; y < box.Max.Y; y++ {
			out.Set(box.Min.X, y, col)
			out.Set(box.Max.X-1, y, col)
		}
	}
	outline(p.Print.Within(view), color.RGBA{30, 90, 220, 255})
	outline(p.Region.Within(view), color.RGBA{0, 170, 0, 255})
	return out, nil
}
