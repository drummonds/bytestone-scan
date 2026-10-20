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

// Print is a print on the fake bed: where it lies in mm, and the value
// every sample of it scans as.
type Print struct {
	X0, Y0, X1, Y1 float64
	Value          uint16
}

// Scanner serves a bed with prints on the white lid.
type Scanner struct {
	Prints      []Print
	Previews    int
	Details     int
	FailDetails int   // fail this many detail passes first
	CloseErr    error // the scan's stream fails as it closes, as scanimage exiting badly does
}

func (f *Scanner) Name() string       { return "Fake Scanner" }
func (f *Scanner) Settings() []string { return []string{"colour correction off"} }

func (f *Scanner) valueAt(xmm, ymm float64) uint16 {
	for _, p := range f.Prints {
		if xmm >= p.X0 && xmm < p.X1 && ymm >= p.Y0 && ymm < p.Y1 {
			return p.Value
		}
	}
	return 65535
}

func (f *Scanner) Preview(context.Context) (image.Image, error) {
	f.Previews++
	const ppi = 75
	w, h := 637, 877 // the bed at 75 ppi
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
