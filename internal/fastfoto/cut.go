package fastfoto

import (
	"math"
	"sort"

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// The feeder scans the full width of its path and stops at the paper's
// end; the driver fills the rest of the length asked for with pure
// white. A side is cut to the sheet before it is a master (story 177,
// the statement's Letters section).
const (
	surroundMM = 3.0 // path kept either side of the sheet
	edgeMM     = 1.0 // left out of the search at each edge of the path: the real feed has white columns there

	flatSD   = 4.0  // a column of path varies less than this down its length, in 8-bit levels
	nearGrey = 14.0 // and is within this of the path's grey
)

// cut is side without the driver's fill below the paper's end, and
// across only the sheet and surroundMM of path either side of it, or to
// the path's edge where that is nearer. A column is path when it is
// flat down its length and the path's grey; the cut runs from the
// outermost column that is not, so a doubt errs wide and the sheet is
// never cut into. Where no sheet is told from the path the full width
// is kept.
func cut(side *master.Image, ppi int) *master.Image {
	rows := scanned(side)
	x0, x1 := 0, side.W
	if first, last, ok := sheet(side, rows, int(edgeMM/25.4*float64(ppi))); ok {
		surround := int(math.Round(surroundMM / 25.4 * float64(ppi)))
		x0, x1 = max(first-surround, 0), min(last+1+surround, side.W)
	}
	if x0 == 0 && x1 == side.W && rows == side.H {
		return side
	}
	out := master.NewImage(x1-x0, rows)
	for y := range rows {
		copy(out.Pix[y*out.W*3:(y+1)*out.W*3], side.Pix[(y*side.W+x0)*3:(y*side.W+x1)*3])
	}
	return out
}

// scanned is the number of rows before the driver's fill: the rows
// left once those of nothing but full white are taken from the foot.
func scanned(side *master.Image) int {
	stride := side.W * 3
	rows := side.H
	for ; rows > 0; rows-- {
		for _, v := range side.Pix[(rows-1)*stride : rows*stride] {
			if v != 0xffff {
				return rows
			}
		}
	}
	return side.H // all white: no telling fill from sheet
}

// sheet is the first and last column of the top rows of side that are
// not path, edge columns at each side of the path left out.
func sheet(side *master.Image, rows, edge int) (first, last int, ok bool) {
	w := side.W
	if rows == 0 || w <= 2*edge {
		return 0, 0, false
	}
	sum, sq := make([]float64, w*3), make([]float64, w*3)
	for y := range rows {
		for i, v := range side.Pix[y*w*3 : (y+1)*w*3] {
			l := float64(v >> 8)
			sum[i] += l
			sq[i] += l * l
		}
	}
	n := float64(rows)
	flat := make([]bool, w)
	var grey [3][]float64 // the flat columns' levels, by channel
	for x := range w {
		flat[x] = true
		for c := range 3 {
			m := sum[x*3+c] / n
			sum[x*3+c] = m // now the column's mean
			if math.Sqrt(max(sq[x*3+c]/n-m*m, 0)) >= flatSD {
				flat[x] = false
			}
		}
		if flat[x] {
			for c := range 3 {
				grey[c] = append(grey[c], sum[x*3+c])
			}
		}
	}
	if len(grey[0]) == 0 {
		return 0, 0, false
	}
	var path [3]float64
	for c := range 3 {
		sort.Float64s(grey[c])
		path[c] = grey[c][len(grey[c])/2]
	}
	isPath := func(x int) bool {
		if !flat[x] {
			return false
		}
		for c := range 3 {
			if math.Abs(sum[x*3+c]-path[c]) >= nearGrey {
				return false
			}
		}
		return true
	}
	first, last = -1, -1
	for x := edge; x < w-edge; x++ {
		if !isPath(x) {
			if first < 0 {
				first = x
			}
			last = x
		}
	}
	return first, last, first >= 0
}
