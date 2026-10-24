// Package master holds a master image and writes it as the format
// statement requires: baseline TIFF, little-endian, 16-bit RGB with no
// alpha, Deflate with the horizontal predictor, exact resolution, and the
// capture facts as embedded XMP.
package master

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
)

// Image is a 16-bit RGB image, samples interleaved R, G, B, row by row.
type Image struct {
	W, H int
	Pix  []uint16
}

func NewImage(w, h int) *Image {
	return &Image{W: w, H: h, Pix: make([]uint16, w*h*3)}
}

func (m *Image) At(x, y, c int) uint16     { return m.Pix[(y*m.W+x)*3+c] }
func (m *Image) Set(x, y, c int, v uint16) { m.Pix[(y*m.W+x)*3+c] = v }

// Options are what a master records beside its pixels.
type Options struct {
	PPI   int    // resolution, pixels per inch
	XMP   []byte // XMP packet to embed; none if empty
	Depth int    // bits per sample: 16 (the default) for a front, 8 for a back
}

// stripBytes is the target uncompressed size of a strip.
const stripBytes = 256 << 10

const (
	tShort    = 3
	tLong     = 4
	tRational = 5
	tByte     = 1
)

type ifdEntry struct {
	tag, typ uint16
	count    uint32
	data     []byte // the value, little-endian, before placement
}

// Encode writes img to w as a master TIFF.
func Encode(w io.Writer, img *Image, opts Options) error {
	if opts.PPI <= 0 {
		return errors.New("master: no resolution given")
	}
	if img.W <= 0 || img.H <= 0 || len(img.Pix) != img.W*img.H*3 {
		return fmt.Errorf("master: bad image %d × %d with %d samples", img.W, img.H, len(img.Pix))
	}
	depth := opts.Depth
	if depth == 0 {
		depth = 16
	}
	if depth != 8 && depth != 16 {
		return fmt.Errorf("master: %d bits per sample, want 8 or 16", depth)
	}
	le := binary.LittleEndian

	bytesPerSample := depth / 8
	rowBytes := img.W * 3 * bytesPerSample
	rowsPerStrip := max(1, stripBytes/rowBytes)
	var strips [][]byte
	row := make([]byte, rowBytes)
	for y0 := 0; y0 < img.H; y0 += rowsPerStrip {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		for y := y0; y < min(y0+rowsPerStrip, img.H); y++ {
			s := img.Pix[y*img.W*3 : (y+1)*img.W*3]
			for i, v := range s {
				if depth == 8 {
					b := uint8(v >> 8)
					if i >= 3 {
						b -= uint8(s[i-3] >> 8) // horizontal differencing, modulo 2^8
					}
					row[i] = b
					continue
				}
				if i >= 3 {
					v -= s[i-3] // horizontal differencing, modulo 2^16
				}
				le.PutUint16(row[2*i:], v)
			}
			if _, err := zw.Write(row); err != nil {
				return err
			}
		}
		if err := zw.Close(); err != nil {
			return err
		}
		strips = append(strips, buf.Bytes())
	}

	shorts := func(vs ...uint16) []byte {
		b := make([]byte, 2*len(vs))
		for i, v := range vs {
			le.PutUint16(b[2*i:], v)
		}
		return b
	}
	longs := func(vs ...uint32) []byte {
		b := make([]byte, 4*len(vs))
		for i, v := range vs {
			le.PutUint32(b[4*i:], v)
		}
		return b
	}
	n := uint32(len(strips))
	counts := make([]uint32, n)
	for i, s := range strips {
		counts[i] = uint32(len(s))
	}
	ppi := longs(uint32(opts.PPI), 1)
	entries := []*ifdEntry{
		{256, tLong, 1, longs(uint32(img.W))},
		{257, tLong, 1, longs(uint32(img.H))},
		{258, tShort, 3, shorts(uint16(depth), uint16(depth), uint16(depth))},
		{259, tShort, 1, shorts(8)},
		{262, tShort, 1, shorts(2)},
		{273, tLong, n, nil}, // StripOffsets, filled in below
		{277, tShort, 1, shorts(3)},
		{278, tLong, 1, longs(uint32(rowsPerStrip))},
		{279, tLong, n, longs(counts...)},
		{282, tRational, 1, ppi},
		{283, tRational, 1, ppi},
		{284, tShort, 1, shorts(1)},
		{296, tShort, 1, shorts(2)},
		{317, tShort, 1, shorts(2)},
	}
	if len(opts.XMP) > 0 {
		entries = append(entries, &ifdEntry{700, tByte, uint32(len(opts.XMP)), opts.XMP})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].tag < entries[j].tag })

	// Layout: header, strips, out-of-line values, then the IFD, each on a
	// word boundary.
	even := func(o uint32) uint32 { return o + o%2 }
	off := uint32(8)
	offsets := make([]uint32, n)
	for i, s := range strips {
		offsets[i] = off
		off = even(off + uint32(len(s)))
	}
	for _, e := range entries {
		if e.tag == 273 {
			e.data = longs(offsets...)
		}
	}
	valueOff := make(map[uint16]uint32)
	for _, e := range entries {
		if len(e.data) > 4 {
			valueOff[e.tag] = off
			off = even(off + uint32(len(e.data)))
		}
	}
	ifdOff := off

	var out bytes.Buffer
	pad := func() {
		if out.Len()%2 == 1 {
			out.WriteByte(0)
		}
	}
	out.WriteString("II*\x00")
	out.Write(longs(ifdOff))
	for _, s := range strips {
		out.Write(s)
		pad()
	}
	for _, e := range entries {
		if len(e.data) > 4 {
			out.Write(e.data)
			pad()
		}
	}
	out.Write(shorts(uint16(len(entries))))
	for _, e := range entries {
		out.Write(shorts(e.tag, e.typ))
		out.Write(longs(e.count))
		if len(e.data) > 4 {
			out.Write(longs(valueOff[e.tag]))
		} else {
			v := make([]byte, 4)
			copy(v, e.data)
			out.Write(v)
		}
	}
	out.Write(longs(0)) // no further IFD
	_, err := w.Write(out.Bytes())
	return err
}
