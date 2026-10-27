// Package rendition makes the display images of a master: the print
// measured on the master at full resolution, cropped to its edges,
// turned square, its levels set, and scaled to fit a screen. A master is
// never altered; a rendition is thrown away and made again whenever the
// recipe improves.
package rendition

import (
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	"gopkg.in/yaml.v3"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// Version is the recipe: how renditions are made. A change to what this
// package does to a master is a new version, so renditions made before it
// are told apart from those made after.
const Version = 2

// The renditions' sizes, in pixels.
const (
	DisplayW, DisplayH = 1920, 1080 // the display image fits within this
	ThumbEdge          = 200        // the thumbnail's long edge
	TinyEdge           = 64         // the tiny thumbnail's long edge
)

// maxDeskew is the most a print is turned to square, in degrees: beyond
// it the operator straightens the print on the bed instead.
const maxDeskew = 10.0

// The print's edge against the lid is a step down of step, 2% of full
// scale, over about 0.13 mm, after the lid has been seen: a level at or
// above lidLevel. The lid is not clipped on every scanner (99% white on
// the 4990 with colour correction off), and where the print lifts it the
// lid darkens gradually towards the print's edge, so the edge is the
// step and not the first sample short of white; and a print that runs
// to the master's edge, no lid seen outside it, has its edge there,
// whatever steps its own texture holds.
const (
	step     = 0.02 * 65535
	lidLevel = 0.97 * 65535
)

// Size of an image in pixels.
type Size struct {
	W int `yaml:"w"`
	H int `yaml:"h"`
}

// Recipe is recipe.yaml: how one master's renditions were made.
type Recipe struct {
	Version    int      `yaml:"version"`
	Master     string   `yaml:"master"`      // sha256 of the master TIFF
	Print      find.Box `yaml:"print"`       // the print measured on the master, in master pixels
	RegionSkew float64  `yaml:"region_skew"` // the angle the region recorded, degrees
	Skew       float64  `yaml:"skew"`        // the angle the rendition was turned back by; 0 when over 10°
	Black      uint16   `yaml:"black"`       // the master level shown as black
	White      uint16   `yaml:"white"`       // the master level shown as white
	Display    Size     `yaml:"display"`
	Flags      []string `yaml:"flags,omitempty"` // what the operator should look at
}

// Images are the renditions of one master.
type Images struct {
	Display, Thumb, Tiny image.Image
}

// Measurement is the print as measured on the master.
type Measurement struct {
	Print find.Box // in master pixels
	Flags []string
}

// Measure finds the print's edges on the master at full resolution,
// starting from hint, the region's idea of where it lies in master
// pixels, and fits a rectangle to them. A torn or trimmed print is
// measured by its longest straight edge and flagged; so is a print that
// lies far from where the region said.
func Measure(m *master.Image, hint find.Box, ppi int) Measurement {
	mm := float64(ppi) / 25.4
	edges := [4]edgeFit{}
	for k := range edges {
		edges[k] = fitEdge(m, hint, k, 2*mm, 3*mm, max(2, int(math.Round(0.13*mm))))
	}
	// each edge's angle on the bed
	var angles [4]float64
	for k, e := range edges {
		angles[k] = hint.Angle + e.tilt(k)
	}
	weight := func(k int) float64 { return float64(edges[k].inliers) * edges[k].length(hint) }
	var sum, wsum, best float64
	bestK := 0
	for k := range edges {
		w := weight(k)
		sum, wsum = sum+angles[k]*w, wsum+w
		if w > best {
			best, bestK = w, k
		}
	}
	var flags []string
	angle := angles[bestK]
	if wsum > 0 {
		angle = sum / wsum
	}
	rectangular := true
	for k, e := range edges {
		// a straight edge loses the odd profile to a speck; a tear or a
		// deckle loses a run of them
		if e.found < samples/2 || e.found-e.inliers > 2 || math.Abs(angles[k]-angle) > 0.3 {
			rectangular = false
		}
	}
	if !rectangular {
		angle = angles[bestK]
		flags = append(flags, "not rectangular: measured by its longest straight edge")
	}
	angle = math.Round(angle*10) / 10
	if math.Abs(angle-hint.Angle) > 0.5 {
		flags = append(flags, fmt.Sprintf("the print measures %.1f° on the master; the region said %.1f°", angle, hint.Angle))
	}

	// the largest rectangle within the edges, in a frame at the measured angle
	frame := find.Box{CX: hint.CX, CY: hint.CY, W: hint.W, H: hint.H, Angle: angle}
	var inner [4]float64 // top, right, bottom, left, in the frame
	for k, e := range edges {
		along, across := e.inFrame(hint, frame, k)
		a, b := leastSquares(along, across)
		var ends [2]float64
		if k%2 == 0 {
			ends = [2]float64{a, a + b*hint.W}
		} else {
			ends = [2]float64{a, a + b*hint.H}
		}
		switch k {
		case 0, 3:
			inner[k] = max(ends[0], ends[1])
		default:
			inner[k] = min(ends[0], ends[1])
		}
		// no edge found, or one fewer than half the profiles find (a
		// print as bright as the lid, whose creases a few profiles take
		// for its edge): the hint's stands
		if e.inliers < 2 || e.found < samples/2 {
			inner[k] = [4]float64{0, hint.W, hint.H, 0}[k]
		}
	}
	cx, cy := frame.At((inner[3]+inner[1])/2, (inner[0]+inner[2])/2)
	return Measurement{
		Print: find.Box{CX: cx, CY: cy, W: inner[1] - inner[3], H: inner[2] - inner[0], Angle: angle},
		Flags: flags,
	}
}

const samples = 48 // profiles walked across each edge

// edgeFit is one edge of the print: where it was found, in the hint's own
// axes, and the line fitted through it.
type edgeFit struct {
	along, across  []float64 // the inliers: u and v for a horizontal edge, v and u for a vertical
	found, inliers int
	a, b           float64 // across = a + b × along
}

func (e edgeFit) length(hint find.Box) float64 {
	if e.inliers == 0 {
		return 0
	}
	return math.Max(hint.W, hint.H)
}

// tilt is the edge's angle from the hint's own axis, degrees clockwise.
func (e edgeFit) tilt(k int) float64 {
	if e.inliers < 2 {
		return 0
	}
	deg := math.Atan(e.b) * 180 / math.Pi
	if k%2 == 1 { // a vertical edge turned clockwise leans the other way
		deg = -deg
	}
	return deg
}

// inFrame is the edge's inliers in frame's axes.
func (e edgeFit) inFrame(hint, frame find.Box, k int) (along, across []float64) {
	for i := range e.along {
		var u, v float64
		if k%2 == 0 {
			u, v = e.along[i], e.across[i]
		} else {
			u, v = e.across[i], e.along[i]
		}
		x, y := hint.At(u, v)
		fu, fv := frame.In(x, y)
		if k%2 == 0 {
			along, across = append(along, fu), append(across, fv)
		} else {
			along, across = append(along, fv), append(across, fu)
		}
	}
	return along, across
}

// fitEdge walks profiles across edge k of the hint (0 top, 1 right, 2
// bottom, 3 left), from out pixels outside it to in pixels inside, and
// marks the first step down of step over window pixels: the lid ending
// and the print beginning. A line is fitted through the marks, dropping
// outliers: a torn corner, a curl. Marks scattered more than six windows
// about the line, about 0.8 mm, are no edge at all.
func fitEdge(m *master.Image, hint find.Box, k int, out, in float64, window int) edgeFit {
	var e edgeFit
	length := hint.W
	if k%2 == 1 {
		length = hint.H
	}
	level := func(x, y float64) (lv float64, inImage bool) {
		xi, yi := int(math.Floor(x)), int(math.Floor(y))
		if xi < 0 || yi < 0 || xi >= m.W || yi >= m.H {
			return 0, false
		}
		return (float64(m.At(xi, yi, 0)) + float64(m.At(xi, yi, 1)) + float64(m.At(xi, yi, 2))) / 3, true
	}
	// a profile's point in the hint's axes: along the edge at t, across it
	// at d, measured inwards from the edge
	point := func(t, d float64) (u, v float64) {
		switch k {
		case 0:
			return t, d
		case 1:
			return hint.W - d, t
		case 2:
			return t, hint.H - d
		default:
			return d, t
		}
	}
	for i := range samples {
		t := length * (0.1 + 0.8*float64(i)/float64(samples-1))
		var levels, ds []float64 // the profile so far, in the image
		lidSeen := false
		mark := func(edge float64) {
			u, v := point(t, edge)
			across := v
			if k%2 == 1 {
				across = u
			}
			e.along, e.across = append(e.along, t), append(e.across, across)
		}
		marked := false
		for d := -out; d <= in; d++ {
			u, v := point(t, d)
			x, y := hint.At(u, v)
			lv, inImage := level(x, y)
			if !inImage {
				continue // the master is cut at the bed's edge: the print starts where it does
			}
			levels, ds = append(levels, lv), append(ds, d)
			lidSeen = lidSeen || lv >= lidLevel
			n := len(levels)
			if !lidSeen || n <= window || levels[n-1-window]-levels[n-1] < step {
				continue
			}
			// the edge is where the step crosses its midpoint
			edge := ds[n-1]
			for j := n - 1 - window; j < n; j++ {
				if levels[j] <= (levels[n-1-window]+levels[n-1])/2 {
					edge = ds[j]
					break
				}
			}
			mark(edge)
			marked = true
			break
		}
		if !marked && !lidSeen && len(ds) > 0 {
			mark(ds[0]) // print to the master's edge
		}
	}
	e.found = len(e.along)
	if e.found < 2 {
		return e
	}
	for range 3 {
		e.a, e.b = leastSquares(e.along, e.across)
		var ss float64
		for i := range e.along {
			r := e.across[i] - (e.a + e.b*e.along[i])
			ss += r * r
		}
		limit := math.Max(3, 3*math.Sqrt(ss/float64(len(e.along))))
		var along, across []float64
		for i := range e.along {
			if math.Abs(e.across[i]-(e.a+e.b*e.along[i])) <= limit {
				along, across = append(along, e.along[i]), append(across, e.across[i])
			}
		}
		if len(along) == len(e.along) {
			break
		}
		e.along, e.across = along, across
	}
	e.inliers = len(e.along)
	// marks that do not line up are no edge: a print as bright as the lid,
	// each profile stopping at a crease of its own
	var ss float64
	for i := range e.along {
		r := e.across[i] - (e.a + e.b*e.along[i])
		ss += r * r
	}
	if e.inliers > 0 && math.Sqrt(ss/float64(e.inliers)) > float64(6*window) {
		e.inliers = 0
	}
	return e
}

// leastSquares fits y = a + b x.
func leastSquares(xs, ys []float64) (a, b float64) {
	n := float64(len(xs))
	if n < 2 {
		if n == 1 {
			return ys[0], 0
		}
		return 0, 0
	}
	var sx, sy, sxx, sxy float64
	for i := range xs {
		sx, sy, sxx, sxy = sx+xs[i], sy+ys[i], sxx+xs[i]*xs[i], sxy+xs[i]*ys[i]
	}
	d := n*sxx - sx*sx
	if d == 0 {
		return sy / n, 0
	}
	b = (n*sxy - sx*sy) / d
	return (sy - b*sx) / n, b
}

// Levels are the master levels to show as black and white: 0.1% of the
// print's samples lie below the black point and 0.1% above the white,
// counted 1 mm in from the print's edges so the lid and a curled corner
// do not count. A point within one 8-bit level of the master's extreme is
// taken to the extreme, so a print already using the full range is shown
// unchanged.
func Levels(m *master.Image, print find.Box, ppi int) (black, white uint16) {
	mm := float64(ppi) / 25.4
	inset := find.Box{CX: print.CX, CY: print.CY, W: print.W - 2*mm, H: print.H - 2*mm, Angle: print.Angle}
	if inset.W <= 0 || inset.H <= 0 {
		return 0, 65535
	}
	var hist [65536]uint32
	var n uint64
	b := inset.Bounds()
	for y := max(0, int(b.Y0)); y < min(m.H, int(math.Ceil(b.Y1))); y++ {
		for x := max(0, int(b.X0)); x < min(m.W, int(math.Ceil(b.X1))); x++ {
			if !inset.Contains(float64(x)+0.5, float64(y)+0.5) {
				continue
			}
			for c := range 3 {
				hist[m.At(x, y, c)]++
			}
			n += 3
		}
	}
	if n == 0 {
		return 0, 65535
	}
	tail := n / 1000
	var seen uint64
	lo := 0
	for seen+uint64(hist[lo]) < tail && lo < 65535 {
		seen += uint64(hist[lo])
		lo++
	}
	seen = 0
	hi := 65535
	for seen+uint64(hist[hi]) < tail && hi > 0 {
		seen += uint64(hist[hi])
		hi--
	}
	if lo < 256 {
		lo = 0
	}
	if hi > 65535-256 {
		hi = 65535
	}
	if hi <= lo {
		return 0, 65535
	}
	return uint16(lo), uint16(hi)
}

// Develop makes the master's renditions: the print measured from hint,
// turned square unless it lies over 10° (then cropped square to the bed
// within it), its levels set, scaled to fit the display size and never
// up. The recipe says what was done; its Master is left for the caller.
func Develop(m *master.Image, hint find.Box, ppi int) (Recipe, Images) {
	meas := Measure(m, hint, ppi)
	r := Recipe{Version: Version, Print: meas.Print, RegionSkew: hint.Angle, Skew: meas.Print.Angle, Flags: meas.Flags}
	crop := meas.Print
	if math.Abs(crop.Angle) > maxDeskew {
		r.Skew = 0
		r.Flags = append(r.Flags, fmt.Sprintf("turned %.1f°, over 10°: not straightened; straighten the print on the bed", crop.Angle))
		crop = find.BoxOf(meas.Print.Inside())
	}
	r.Black, r.White = Levels(m, meas.Print, ppi)

	fit := func(b find.Box) float64 { return min(DisplayW/b.W, DisplayH/b.H, 1) }
	// the master, levelled and averaged down by a whole factor so the
	// kernel below works within a factor of two
	k := max(1, int(1/fit(crop)))
	// the kernel reaches two source pixels beyond the pixel it makes; the
	// crop comes in that far, so no lid bleeds into its edge
	inset := float64(2*k + 1)
	crop.W, crop.H = max(1, crop.W-2*inset), max(1, crop.H-2*inset)
	scale := fit(crop)
	outW, outH := max(1, int(math.Round(crop.W*scale))), max(1, int(math.Round(crop.H*scale)))
	r.Display = Size{outW, outH}
	src := levelled(m, r.Black, r.White, k)
	ks := float64(k)
	s, c := math.Sincos(crop.Angle * math.Pi / 180)
	// the source's point (x, y) lands at (u, v) in the crop's own axes,
	// scaled to the display: the affine map Transform wants
	ux, uy, vx, vy := c, s, -s, c
	cx, cy := crop.CX/ks, crop.CY/ks
	aff := f64.Aff3{
		scale * ks * ux, scale * ks * uy, scale * ks * (crop.W/ks/2 - cx*ux - cy*uy),
		scale * ks * vx, scale * ks * vy, scale * ks * (crop.H/ks/2 - cx*vx - cy*vy),
	}
	display := image.NewRGBA(image.Rect(0, 0, outW, outH))
	draw.CatmullRom.Transform(display, aff, src, src.Bounds(), draw.Src, nil)
	return r, Images{Display: display, Thumb: fitLongEdge(display, ThumbEdge), Tiny: fitLongEdge(display, TinyEdge)}
}

// levelled is the master with black and white mapped to the extremes,
// each k × k block averaged to one pixel.
func levelled(m *master.Image, black, white uint16, k int) *image.RGBA64 {
	w, h := max(1, m.W/k), max(1, m.H/k)
	out := image.NewRGBA64(image.Rect(0, 0, w, h))
	span := float64(white) - float64(black)
	n := float64(k * k)
	for y := range h {
		for x := range w {
			var sum [3]float64
			for yy := y * k; yy < (y+1)*k; yy++ {
				for xx := x * k; xx < (x+1)*k; xx++ {
					for c := range 3 {
						sum[c] += float64(m.At(xx, yy, c))
					}
				}
			}
			i := out.PixOffset(x, y)
			for c := range 3 {
				v := (sum[c]/n - float64(black)) / span * 65535
				u := uint16(math.Round(min(65535, max(0, v))))
				out.Pix[i+2*c], out.Pix[i+2*c+1] = uint8(u>>8), uint8(u)
			}
			out.Pix[i+6], out.Pix[i+7] = 0xff, 0xff
		}
	}
	return out
}

// fitLongEdge scales img so its long edge is edge pixels.
func fitLongEdge(img *image.RGBA, edge int) image.Image {
	b := img.Bounds()
	long := max(b.Dx(), b.Dy())
	w, h := max(1, int(math.Round(float64(b.Dx()*edge)/float64(long)))), max(1, int(math.Round(float64(b.Dy()*edge)/float64(long))))
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(out, out.Bounds(), img, b, draw.Src, nil)
	return out
}

// Save writes the renditions and recipe.yaml under dir.
func Save(dir string, r Recipe, img Images) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range []struct {
		name    string
		img     image.Image
		quality int
	}{{"display.jpg", img.Display, 90}, {"thumb.jpg", img.Thumb, 85}, {"tiny.jpg", img.Tiny, 80}} {
		if err := writeAtomic(filepath.Join(dir, f.name), func(w io.Writer) error {
			return jpeg.Encode(w, f.img, &jpeg.Options{Quality: f.quality})
		}); err != nil {
			return err
		}
	}
	return writeAtomic(filepath.Join(dir, "recipe.yaml"), func(w io.Writer) error {
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		if err := enc.Encode(r); err != nil {
			return err
		}
		return enc.Close()
	})
}

// Load reads the recipe under dir.
func Load(dir string) (Recipe, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "recipe.yaml"))
	if err != nil {
		return Recipe{}, err
	}
	var r Recipe
	if err := yaml.Unmarshal(raw, &r); err != nil {
		return Recipe{}, fmt.Errorf("%s: %w", dir, err)
	}
	return r, nil
}

func writeAtomic(path string, write func(io.Writer) error) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// Whole is the renditions of a page as sent: the whole image fit to the
// screen, nothing measured, turned or levelled, since the page is kept
// as the evidence of what was received. The caller sets Master.
func Whole(img image.Image) (Recipe, Images) {
	b := img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	r := Recipe{Version: Version, Print: find.Box{CX: w / 2, CY: h / 2, W: w, H: h}, White: 65535}
	scale := min(DisplayW/w, DisplayH/h, 1)
	outW, outH := max(1, int(math.Round(w*scale))), max(1, int(math.Round(h*scale)))
	r.Display = Size{outW, outH}
	display := image.NewRGBA(image.Rect(0, 0, outW, outH))
	draw.CatmullRom.Scale(display, display.Bounds(), img, b, draw.Src, nil)
	return r, Images{Display: display, Thumb: fitLongEdge(display, ThumbEdge), Tiny: fitLongEdge(display, TinyEdge)}
}
