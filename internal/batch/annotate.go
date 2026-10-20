package batch

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
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
	px := func(mm float64) int { return int(mm / 25.4 * float64(ppi)) }
	for i, p := range prints {
		col := color.RGBA{0, 160, 0, 255}
		if p.Review {
			col = color.RGBA{220, 0, 0, 255}
		}
		r := image.Rect(px(p.Region.X0), px(p.Region.Y0), px(p.Region.X1), px(p.Region.Y1)).Intersect(out.Bounds())
		for t := 0; t < 2; t++ {
			rr := r.Inset(t)
			for x := rr.Min.X; x < rr.Max.X; x++ {
				out.Set(x, rr.Min.Y, col)
				out.Set(x, rr.Max.Y-1, col)
			}
			for y := rr.Min.Y; y < rr.Max.Y; y++ {
				out.Set(rr.Min.X, y, col)
				out.Set(rr.Max.X-1, y, col)
			}
		}
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

// CloseUp is print n's region on the preview with 6 mm round it,
// enlarged pixel for pixel: the region outlined in green, the print as
// found in blue.
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
	px := func(mm float64) int { return int(mm / 25.4 * PreviewPPI) }
	p := prints[n]
	const context = 6.0
	view := image.Rect(px(p.Region.X0-context), px(p.Region.Y0-context), px(p.Region.X1+context)+1, px(p.Region.Y1+context)+1).Intersect(img.Bounds())
	k := max(1, min(4, closeUpWidth/max(1, view.Dx())))
	out := image.NewRGBA(image.Rect(0, 0, view.Dx()*k, view.Dy()*k))
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			out.Set(x, y, img.At(view.Min.X+x/k, view.Min.Y+y/k))
		}
	}
	outline := func(r find.Rect, col color.RGBA) {
		box := image.Rect((px(r.X0)-view.Min.X)*k, (px(r.Y0)-view.Min.Y)*k, (px(r.X1)-view.Min.X)*k, (px(r.Y1)-view.Min.Y)*k).Intersect(out.Bounds())
		for x := box.Min.X; x < box.Max.X; x++ {
			out.Set(x, box.Min.Y, col)
			out.Set(x, box.Max.Y-1, col)
		}
		for y := box.Min.Y; y < box.Max.Y; y++ {
			out.Set(box.Min.X, y, col)
			out.Set(box.Max.X-1, y, col)
		}
	}
	outline(p.Print, color.RGBA{30, 90, 220, 255})
	outline(p.Region, color.RGBA{0, 170, 0, 255})
	return out, nil
}
