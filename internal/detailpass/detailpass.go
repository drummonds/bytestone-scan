// Package detailpass turns the detail pass, one high-resolution scan
// covering every print on the bed, into one master image per print.
// Rows are averaged down as they stream from the scanner, so the scan's
// full resolution is never held in memory.
package detailpass

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"

	"git.bytestone.uk/hum3/bytestone-scan/internal/master"
)

// Read reads a 16-bit colour PNM scan from r, averages each factor ×
// factor block of pixels to one, rounding to the nearest count, and cuts
// the result into one image per region. Regions are in averaged pixels,
// relative to the scan's top left. Blocks cut short by the scan's right or
// bottom edge are dropped.
func Read(r io.Reader, factor int, regions []image.Rectangle) ([]*master.Image, error) {
	if factor != 1 && factor != 2 && factor != 4 {
		return nil, fmt.Errorf("detail pass: averaging factor %d, want 1, 2 or 4", factor)
	}
	br := bufio.NewReaderSize(r, 1<<20)
	w, h, err := readHeader(br)
	if err != nil {
		return nil, err
	}
	grid := image.Rect(0, 0, w/factor, h/factor)
	out := make([]*master.Image, len(regions))
	for i, rg := range regions {
		if rg.Empty() {
			return nil, fmt.Errorf("detail pass: region %d is empty", i+1)
		}
		if !rg.In(grid) {
			return nil, fmt.Errorf("detail pass: region %d %v is outside the pass %v", i+1, rg, grid)
		}
		out[i] = master.NewImage(rg.Dx(), rg.Dy())
	}

	row := make([]byte, w*6)
	sums := make([]uint32, grid.Dx()*3)
	avg := make([]uint16, grid.Dx()*3)
	n := uint32(factor * factor)
	readRow := func(y int) error {
		if _, err := io.ReadFull(br, row); err != nil {
			return fmt.Errorf("detail pass: scan short at row %d of %d: %w", y+1, h, err)
		}
		return nil
	}
	for my := 0; my < grid.Dy(); my++ {
		clear(sums)
		for dy := 0; dy < factor; dy++ {
			if err := readRow(my*factor + dy); err != nil {
				return nil, err
			}
			for i := range sums {
				mx, c := i/3, i%3
				for dx := 0; dx < factor; dx++ {
					sums[i] += uint32(binary.BigEndian.Uint16(row[((mx*factor+dx)*3+c)*2:]))
				}
			}
		}
		for i, s := range sums {
			avg[i] = uint16((s + n/2) / n)
		}
		for i, rg := range regions {
			if my < rg.Min.Y || my >= rg.Max.Y {
				continue
			}
			dst := out[i].Pix[(my-rg.Min.Y)*rg.Dx()*3:]
			copy(dst[:rg.Dx()*3], avg[rg.Min.X*3:rg.Max.X*3])
		}
	}
	for y := grid.Dy() * factor; y < h; y++ {
		if err := readRow(y); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// readHeader reads a P6 header with maxval 65535.
func readHeader(br *bufio.Reader) (w, h int, err error) {
	var fields [4]string
	for i := range fields {
		tok, err := token(br)
		if err != nil {
			return 0, 0, fmt.Errorf("detail pass: reading PNM header: %w", err)
		}
		fields[i] = tok
	}
	if fields[0] != "P6" {
		return 0, 0, fmt.Errorf("detail pass: PNM type %s, want P6 (colour)", fields[0])
	}
	var maxval int
	if _, err := fmt.Sscan(fields[1]+" "+fields[2]+" "+fields[3], &w, &h, &maxval); err != nil {
		return 0, 0, fmt.Errorf("detail pass: PNM header: %w", err)
	}
	if maxval != 65535 {
		return 0, 0, fmt.Errorf("detail pass: maxval %d, want a 16-bit scan (65535)", maxval)
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("detail pass: scan is %d × %d", w, h)
	}
	return w, h, nil
}

// token reads one whitespace-separated header token, skipping comments,
// and consumes the single whitespace byte that ends it.
func token(br *bufio.Reader) (string, error) {
	var tok []byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			if err == io.EOF {
				err = errors.New("header ends early")
			}
			return "", err
		}
		switch {
		case b == '#' && len(tok) == 0:
			if _, err := br.ReadString('\n'); err != nil {
				return "", err
			}
		case b == ' ' || b == '\t' || b == '\n' || b == '\r':
			if len(tok) > 0 {
				return string(tok), nil
			}
		default:
			tok = append(tok, b)
		}
	}
}
