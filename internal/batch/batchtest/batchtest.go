// Package batchtest is a stand-in scanner for tests of batches and what
// is built on them.
package batchtest

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"

	"git.bytestone.uk/hum3/bytestone-scan/internal/find"
)

// Print is a print on the fake bed: where it lies in mm before it is
// turned Angle degrees clockwise about its centre, and the value every
// sample of it scans as.
type Print struct {
	X0, Y0, X1, Y1 float64
	Angle          float64
	Value          uint16
}

// Scanner serves a bed with prints on the white lid.
type Scanner struct {
	Prints      []Print
	Previews    int
	Details     int
	FailDetails int   // fail this many detail passes first
	CloseErr    error // the scan's stream fails as it closes, as scanimage exiting badly does
	LastPPI     int   // the ppi of the last detail pass
}

func (f *Scanner) Name() string       { return "Fake Scanner" }
func (f *Scanner) Settings() []string { return []string{"colour correction off"} }

func (f *Scanner) valueAt(xmm, ymm float64) uint16 {
	for _, p := range f.Prints {
		// into the print's frame, unturned about its centre
		cx, cy := (p.X0+p.X1)/2, (p.Y0+p.Y1)/2
		s, c := math.Sincos(p.Angle * math.Pi / 180)
		dx, dy := xmm-cx, ymm-cy
		x, y := cx+dx*c+dy*s, cy-dx*s+dy*c
		if x >= p.X0 && x < p.X1 && y >= p.Y0 && y < p.Y1 {
			return p.Value
		}
	}
	return 65535
}

func (f *Scanner) Preview(context.Context) (image.Image, error) {
	f.Previews++
	const ppi = 150
	w, h := 1272, 1755 // the bed at 150 ppi, in whole 8 px across as the 4990 scans it
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(f.valueAt((float64(x)+0.5)*25.4/ppi, (float64(y)+0.5)*25.4/ppi) >> 8)
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img, nil
}

type closeFails struct {
	io.Reader
	err error
}

func (c closeFails) Close() error { return c.err }

func (f *Scanner) Detail(_ context.Context, area find.Rect, ppi int) (io.ReadCloser, error) {
	f.Details++
	f.LastPPI = ppi
	if f.Details <= f.FailDetails {
		return nil, errors.New("scanner: USB disconnected")
	}
	px := func(mm float64) int { return int(math.Round(mm / 25.4 * float64(ppi))) }
	w, h := px(area.X1-area.X0), px(area.Y1-area.Y0)
	var b bytes.Buffer
	fmt.Fprintf(&b, "P6\n%d %d\n65535\n", w, h)
	row := make([]byte, w*6)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := f.valueAt(area.X0+(float64(x)+0.5)*25.4/float64(ppi), area.Y0+(float64(y)+0.5)*25.4/float64(ppi))
			for c := 0; c < 3; c++ {
				binary.BigEndian.PutUint16(row[(x*3+c)*2:], v)
			}
		}
		b.Write(row)
	}
	return closeFails{&b, f.CloseErr}, nil
}
