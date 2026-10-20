package master

import (
	"fmt"
	"image"
	"io"

	"golang.org/x/image/tiff"
)

// Decode reads a master TIFF.
func Decode(r io.Reader) (*Image, error) {
	img, err := tiff.Decode(r)
	if err != nil {
		return nil, err
	}
	rgba, ok := img.(*image.RGBA64)
	if !ok {
		return nil, fmt.Errorf("master: decoded as %T, want 16-bit RGB", img)
	}
	b := rgba.Bounds()
	m := NewImage(b.Dx(), b.Dy())
	for y := 0; y < m.H; y++ {
		row := rgba.Pix[y*rgba.Stride:]
		for x := 0; x < m.W; x++ {
			for c := 0; c < 3; c++ {
				m.Pix[(y*m.W+x)*3+c] = uint16(row[x*8+c*2])<<8 | uint16(row[x*8+c*2+1])
			}
		}
	}
	return m, nil
}
