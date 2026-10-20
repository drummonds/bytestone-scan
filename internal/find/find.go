// Package find finds the prints on the scanner bed from the preview.
//
// Anything that differs from the background, the lid or black card behind
// the prints, is part of a print, except strips narrower than 6 mm along
// the bed's edges, which are the shadow of its frame. Prints
// less than 5 mm apart are one region, marks under 20 mm are ignored, and a
// print that is skewed or not rectangular is flagged for review. Its skew
// and fill come from the smallest rectangle that encloses it, at any angle.
package find

import (
	"image"
	"math"
	"sort"
)

// Thresholds from story 001; the skew and fill limits are first guesses.
const (
	marginMM   = 3.0
	stripMM    = 6.0
	mergeMM    = 5.0
	minSizeMM  = 20.0
	maxSkewDeg = 1.0
	minFill    = 0.97

	// background is how far, in 8-bit levels on any channel, a pixel must
	// be from the background's colour to count as print.
	backgroundDistance = 14
)

// Rect is a rectangle on the bed in millimetres from the bed's origin, the
// top left of the preview.
type Rect struct{ X0, Y0, X1, Y1 float64 }

// Print is one print, or several too close to separate, found on the bed.
type Print struct {
	Print  Rect    // the print's own extent
	Region Rect    // the print plus the margin, within the bed
	Skew   float64 // degrees from square to the bed
	Fill   float64 // share of its enclosing rectangle the print fills
	Parts  int     // separate prints in the region, when they are under 5 mm apart
	Review bool    // skewed, not rectangular, or more than one print
}

// Prints finds the prints in a preview scanned at ppi, numbered in rows
// from the top, left to right within a row.
func Prints(preview image.Image, ppi int) []Print {
	b := preview.Bounds()
	w, h := b.Dx(), b.Dy()
	mm := 25.4 / float64(ppi)
	mask := printMask(preview)
	ro := int(math.Round(stripMM / 2 / mm))
	opened := dilate(erode(mask, w, h, ro), w, h, ro)
	band := int(math.Round(stripMM / mm))
	for i := range mask {
		x, y := i%w, i/w
		if x < band || y < band || x >= w-band || y >= h-band {
			mask[i] = opened[i]
		}
	}

	r := int(math.Round(mergeMM / 2 / mm))
	labels, n := components(dilate(mask, w, h, r), w, h)

	var prints []Print
	for l := 1; l <= n; l++ {
		bx := image.Rectangle{Min: image.Pt(w, h)}
		for i, on := range mask {
			if on && labels[i] == l {
				x, y := i%w, i/w
				bx = bx.Union(image.Rect(x, y, x+1, y+1))
			}
		}
		if bx.Empty() {
			continue
		}
		p := Rect{float64(bx.Min.X) * mm, float64(bx.Min.Y) * mm, float64(bx.Max.X) * mm, float64(bx.Max.Y) * mm}
		if p.X1-p.X0 < minSizeMM || p.Y1-p.Y0 < minSizeMM {
			continue
		}
		skew, fill, parts := shape(mask, labels, l, w, bx, int(minSizeMM/2/mm))
		prints = append(prints, Print{
			Print: p,
			Region: Rect{
				max(0, p.X0-marginMM), max(0, p.Y0-marginMM),
				min(float64(w)*mm, p.X1+marginMM), min(float64(h)*mm, p.Y1+marginMM),
			},
			Skew:   skew,
			Fill:   fill,
			Parts:  parts,
			Review: skew > maxSkewDeg || fill < minFill || parts > 1,
		})
	}
	return inRows(prints)
}

// printMask marks the pixels that differ from the background.
func printMask(img image.Image) []bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rgb := make([][3]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			rgb[y*w+x] = [3]int{int(r >> 8), int(g >> 8), int(bl >> 8)}
		}
	}
	bg := background(rgb)
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// 3 × 3 mean, to keep scanner noise out of the mask
			var s [3]int
			var n int
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := x+dx, y+dy
					if xx < 0 || yy < 0 || xx >= w || yy >= h {
						continue
					}
					p := rgb[yy*w+xx]
					s[0], s[1], s[2] = s[0]+p[0], s[1]+p[1], s[2]+p[2]
					n++
				}
			}
			for c := 0; c < 3; c++ {
				if abs(s[c]/n-bg[c]) > backgroundDistance {
					mask[y*w+x] = true
					break
				}
			}
		}
	}
	return mask
}

// background is the commonest colour on the bed, which is the lid or the
// black card behind the prints: the mean of the pixels in the fullest
// cell of a 16-level colour grid.
func background(rgb [][3]int) [3]int {
	cell := func(p [3]int) int { return p[0]>>4<<8 | p[1]>>4<<4 | p[2]>>4 }
	var counts [4096]int
	for _, p := range rgb {
		counts[cell(p)]++
	}
	best := 0
	for c, n := range counts {
		if n > counts[best] {
			best = c
		}
	}
	var sum [3]int
	for _, p := range rgb {
		if cell(p) == best {
			sum[0], sum[1], sum[2] = sum[0]+p[0], sum[1]+p[1], sum[2]+p[2]
		}
	}
	n := counts[best]
	return [3]int{sum[0] / n, sum[1] / n, sum[2] / n}
}

// dilate grows the mask by r pixels in every direction (a square).
func dilate(mask []bool, w, h, r int) []bool {
	rows := make([]bool, w*h)
	for y := 0; y < h; y++ {
		last := -r - 1
		for x := 0; x < w+r; x++ {
			if x < w && mask[y*w+x] {
				last = x
			}
			if xx := x - r; xx >= 0 && xx < w && x-last <= 2*r {
				rows[y*w+xx] = last >= 0
			}
		}
	}
	out := make([]bool, w*h)
	for x := 0; x < w; x++ {
		last := -r - 1
		for y := 0; y < h+r; y++ {
			if y < h && rows[y*w+x] {
				last = y
			}
			if yy := y - r; yy >= 0 && yy < h && y-last <= 2*r {
				out[yy*w+x] = last >= 0
			}
		}
	}
	return out
}

// erode keeps the pixels whose square of radius r lies wholly within the
// mask and the image, so a print against the edge loses nothing once the
// erosion is dilated back, while a strip along the edge goes.
func erode(mask []bool, w, h, r int) []bool {
	run := func(get func(i int) bool, n int, set func(i int, v bool)) {
		// on[i] counts the set cells before i
		on := make([]int, n+1)
		for i := 0; i < n; i++ {
			on[i+1] = on[i]
			if get(i) {
				on[i+1]++
			}
		}
		for i := 0; i < n; i++ {
			lo, hi := i-r, i+r+1
			set(i, lo >= 0 && hi <= n && on[hi]-on[lo] == 2*r+1)
		}
	}
	rows := make([]bool, w*h)
	for y := 0; y < h; y++ {
		run(func(x int) bool { return mask[y*w+x] }, w, func(x int, v bool) { rows[y*w+x] = v })
	}
	out := make([]bool, w*h)
	for x := 0; x < w; x++ {
		run(func(y int) bool { return rows[y*w+x] }, h, func(y int, v bool) { out[y*w+x] = v })
	}
	return out
}

// components labels the 4-connected parts of mask from 1.
func components(mask []bool, w, h int) ([]int, int) {
	labels := make([]int, w*h)
	n := 0
	var stack []int
	for i, on := range mask {
		if !on || labels[i] != 0 {
			continue
		}
		n++
		labels[i] = n
		stack = append(stack[:0], i)
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x := j % w
			for _, k := range [4]int{j - 1, j + 1, j - w, j + w} {
				if k < 0 || k >= len(mask) || (k == j-1 && x == 0) || (k == j+1 && x == w-1) {
					continue
				}
				if mask[k] && labels[k] == 0 {
					labels[k] = n
					stack = append(stack, k)
				}
			}
		}
	}
	return labels, n
}

// shape measures the print labelled l within bx: its skew, how much of its
// smallest enclosing rectangle it fills, holes (light patches of the
// picture) counted as print, and how many separate parts it has of at
// least minSide pixels square.
func shape(mask []bool, labels []int, l, w int, bx image.Rectangle, minSide int) (skew, fill float64, parts int) {
	bw, bh := bx.Dx(), bx.Dy()
	in := func(x, y int) bool {
		i := (bx.Min.Y+y)*w + bx.Min.X + x
		return mask[i] && labels[i] == l
	}
	// Flood the outside from the box's border; what it cannot reach is print.
	outside := make([]bool, bw*bh)
	var stack []int
	push := func(x, y int) {
		if x < 0 || y < 0 || x >= bw || y >= bh || outside[y*bw+x] || in(x, y) {
			return
		}
		outside[y*bw+x] = true
		stack = append(stack, y*bw+x)
	}
	for x := 0; x < bw; x++ {
		push(x, 0)
		push(x, bh-1)
	}
	for y := 0; y < bh; y++ {
		push(0, y)
		push(bw-1, y)
	}
	for len(stack) > 0 {
		j := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := j%bw, j/bw
		push(x-1, y)
		push(x+1, y)
		push(x, y-1)
		push(x, y+1)
	}

	var pts [][2]float64
	area := 0
	for y := 0; y < bh; y++ {
		lo, hi := -1, -1
		for x := 0; x < bw; x++ {
			if !outside[y*bw+x] {
				area++
				if lo < 0 {
					lo = x
				}
				hi = x
			}
		}
		if lo >= 0 {
			fy := float64(y)
			pts = append(pts, [2]float64{float64(lo), fy}, [2]float64{float64(lo), fy + 1},
				[2]float64{float64(hi + 1), fy}, [2]float64{float64(hi + 1), fy + 1})
		}
	}
	inner := make([]bool, bw*bh)
	for i, o := range outside {
		inner[i] = !o
	}
	partLabels, n := components(inner, bw, bh)
	sizes := make([]int, n+1)
	for _, pl := range partLabels {
		sizes[pl]++
	}
	for _, sz := range sizes[1:] {
		if sz >= minSide*minSide {
			parts++
		}
	}

	rectArea, angle := minAreaRect(hull(pts))
	a := math.Mod(angle*180/math.Pi, 90)
	if a < 0 {
		a += 90
	}
	if a > 45 {
		a -= 90
	}
	return math.Abs(a), float64(area) / rectArea, parts
}

// hull is the convex hull of pts, by the monotone chain.
func hull(pts [][2]float64) [][2]float64 {
	sort.Slice(pts, func(i, j int) bool {
		if pts[i][0] != pts[j][0] {
			return pts[i][0] < pts[j][0]
		}
		return pts[i][1] < pts[j][1]
	})
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	var h [][2]float64
	for _, p := range pts {
		for len(h) >= 2 && cross(h[len(h)-2], h[len(h)-1], p) <= 0 {
			h = h[:len(h)-1]
		}
		h = append(h, p)
	}
	lower := len(h) + 1
	for i := len(pts) - 2; i >= 0; i-- {
		p := pts[i]
		for len(h) >= lower && cross(h[len(h)-2], h[len(h)-1], p) <= 0 {
			h = h[:len(h)-1]
		}
		h = append(h, p)
	}
	return h[:len(h)-1]
}

// minAreaRect is the area and angle of the smallest rectangle enclosing a
// convex polygon; one of its sides lies along an edge of the polygon.
func minAreaRect(poly [][2]float64) (area, angle float64) {
	area = math.Inf(1)
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		th := math.Atan2(q[1]-p[1], q[0]-p[0])
		s, c := math.Sincos(th)
		u0, u1, v0, v1 := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, r := range poly {
			u, v := r[0]*c+r[1]*s, -r[0]*s+r[1]*c
			u0, u1, v0, v1 = min(u0, u), max(u1, u), min(v0, v), max(v1, v)
		}
		if a := (u1 - u0) * (v1 - v0); a < area {
			area, angle = a, th
		}
	}
	return area, angle
}

// inRows orders prints in rows from the top, left to right within a row;
// a print joins a row when its top is above the bottom of the row's first.
func inRows(ps []Print) []Print {
	sort.Slice(ps, func(i, j int) bool { return ps[i].Print.Y0 < ps[j].Print.Y0 })
	var out []Print
	for i := 0; i < len(ps); {
		j := i + 1
		for j < len(ps) && ps[j].Print.Y0 < ps[i].Print.Y1 {
			j++
		}
		row := ps[i:j]
		sort.Slice(row, func(a, b int) bool { return row[a].Print.X0 < row[b].Print.X0 })
		out = append(out, row...)
		i = j
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
