package batch

import (
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
