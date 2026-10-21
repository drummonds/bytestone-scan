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
func annotate(preview image.Image, prints []find.Print, ppi int) image.Image {
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

// Preview is the batch's preview of the bed.
func (b *Batch) Preview() (image.Image, error) { return readPNG(b.path("preview.png")) }

// Annotated is the preview with each print's region as it stands.
func (b *Batch) Annotated() (image.Image, error) {
	img, err := b.Preview()
	if err != nil {
		return nil, err
	}
	prints, _, err := b.Found()
	if err != nil {
		return nil, err
	}
	return annotate(img, prints, PreviewPPI), nil
}

// closeUpWidth is the width a close-up is enlarged towards, at most 4×.
const closeUpWidth = 480

// CloseUp is print n's region on the preview with 6 mm round it, turned
// so the print sits square and enlarged pixel for pixel: the region
// outlined in green, the print as found in blue.
func (b *Batch) CloseUp(n int) (image.Image, error) {
	img, err := b.Preview()
	if err != nil {
		return nil, err
	}
	prints, _, err := b.Found()
	if err != nil {
		return nil, err
	}
	if n < 0 || n >= len(prints) {
		return nil, fmt.Errorf("batch %s has no print %d", b.ID, n+1)
	}
	const mm = 25.4 / PreviewPPI
	p := prints[n]
	const context = 6.0
	view := p.Region
	view.W += 2 * context
	view.H += 2 * context
	if view.Angle == 0 {
		view = find.BoxOf(view.On(b.Bed())) // square to the bed, it stops at the bed as before
	}
	w, h := int(view.W/mm), int(view.H/mm)
	k := max(1, min(4, closeUpWidth/max(1, w)))
	out := image.NewRGBA(image.Rect(0, 0, w*k, h*k))
	off := color.RGBA{200, 200, 200, 255} // beyond the bed
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			bx, by := view.At((float64(x)+0.5)*mm, (float64(y)+0.5)*mm)
			at := image.Pt(int(math.Floor(bx/mm)), int(math.Floor(by/mm)))
			var c color.Color = off
			if at.In(img.Bounds()) {
				c = img.At(at.X, at.Y)
			}
			for dy := 0; dy < k; dy++ {
				for dx := 0; dx < k; dx++ {
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
