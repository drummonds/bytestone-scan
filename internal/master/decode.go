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
	switch img := img.(type) {
	case *image.RGBA64:
		b := img.Bounds()
		m := NewImage(b.Dx(), b.Dy())
		for y := 0; y < m.H; y++ {
			row := img.Pix[y*img.Stride:]
			for x := 0; x < m.W; x++ {
				for c := range 3 {
					m.Pix[(y*m.W+x)*3+c] = uint16(row[x*8+c*2])<<8 | uint16(row[x*8+c*2+1])
				}
			}
		}
		return m, nil
	case *image.RGBA:
		// an 8-bit master, a back: each sample widened so that 0xff is full scale
		b := img.Bounds()
		m := NewImage(b.Dx(), b.Dy())
		for y := 0; y < m.H; y++ {
			row := img.Pix[y*img.Stride:]
			for x := 0; x < m.W; x++ {
				for c := range 3 {
					v := uint16(row[x*4+c])
					m.Pix[(y*m.W+x)*3+c] = v<<8 | v
				}
			}
		}
		return m, nil
	}
	return nil, fmt.Errorf("master: decoded as %T, want 8- or 16-bit RGB", img)
}
