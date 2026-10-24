// Package quality shows what a master is like, so the operator can judge
// the capture: its histogram with clipping, the whole master scaled to
// fit a screen, and 1:1 crops. Display images are the master's top 8 bits,
// with no colour management: masters are in the scanner's own colour.
package quality

import (
	"image"
	"image/color"

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// Histogram counts each channel's samples in 256 bins of 256 levels, and
// the share of samples clipped at 0 and at 65535.
type Histogram struct {
	Bins       [3][256]int
	Shadows    [3]float64
	Highlights [3]float64
}

// Analyse is the histogram of the master within r, normally the print
// itself: the margin around it is the lid, which clips by design.
func Analyse(m *master.Image, r image.Rectangle) Histogram {
	var h Histogram
	var lo, hi [3]int
	r = r.Intersect(image.Rect(0, 0, m.W, m.H))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for i, v := range m.Pix[(y*m.W+r.Min.X)*3 : (y*m.W+r.Max.X)*3] {
			c := i % 3
			h.Bins[c][v>>8]++
			switch v {
			case 0:
				lo[c]++
			case 65535:
				hi[c]++
			}
		}
	}
	n := float64(max(1, r.Dx()*r.Dy()))
	for c := range 3 {
		h.Shadows[c] = float64(lo[c]) / n
		h.Highlights[c] = float64(hi[c]) / n
	}
	return h
}

// Fit is the master scaled down to at most width pixels wide, never up,
// each pixel the mean of the master pixels it covers.
func Fit(m *master.Image, width int) image.Image {
	if m.W <= width {
		return Crop(m, image.Pt(m.W/2, m.H/2), max(m.W, m.H))
	}
	w, h := width, max(1, m.H*width/m.W)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := y*m.H/h, max((y+1)*m.H/h, y*m.H/h+1)
		for x := range w {
			x0, x1 := x*m.W/w, max((x+1)*m.W/w, x*m.W/w+1)
			var s [3]int
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					for c := range 3 {
						s[c] += int(m.At(xx, yy, c))
					}
				}
			}
			n := (y1 - y0) * (x1 - x0)
			out.SetRGBA(x, y, color.RGBA{uint8(s[0] / n >> 8), uint8(s[1] / n >> 8), uint8(s[2] / n >> 8), 255})
		}
	}
	return out
}

// Crop is size × size master pixels centred on at, moved to lie within the
// master, at 1:1.
func Crop(m *master.Image, at image.Point, size int) image.Image {
	w, h := min(size, m.W), min(size, m.H)
	x0 := min(max(at.X-size/2, 0), m.W-w)
	y0 := min(max(at.Y-size/2, 0), m.H-h)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			out.SetRGBA(x, y, color.RGBA{
				uint8(m.At(x0+x, y0+y, 0) >> 8), uint8(m.At(x0+x, y0+y, 1) >> 8), uint8(m.At(x0+x, y0+y, 2) >> 8), 255})
		}
	}
	return out
}
