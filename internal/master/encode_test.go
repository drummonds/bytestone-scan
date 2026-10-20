package master

import (
	"bytes"
	"encoding/binary"
	"image"
	"testing"

	"golang.org/x/image/tiff"
)

// testImage fills an image with values that exercise the predictor: both
// extremes, and neighbours whose difference wraps around 16 bits.
func testImage(w, h int) *Image {
	img := NewImage(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for c := 0; c < 3; c++ {
				v := uint16((x*7919 + y*104729 + c*31337) % 65536)
				switch {
				case x == 0 && y == 0:
					v = 0
				case x == 1 && y == 0:
					v = 65535
				}
				img.Set(x, y, c, v)
			}
		}
	}
	return img
}

func encode(t *testing.T, img *Image, opts Options) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Encode(&buf, img, opts); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return buf.Bytes()
}

func TestEncodeReadsBackIdentical(t *testing.T) {
	// 300 × 500 spans several strips.
	for _, size := range []image.Point{{1, 1}, {37, 23}, {300, 500}} {
		img := testImage(size.X, size.Y)
		data := encode(t, img, Options{PPI: 600})

		got, err := tiff.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%v: decode: %v", size, err)
		}
		rgba, ok := got.(*image.RGBA64)
		if !ok {
			t.Fatalf("%v: decoded as %T, want 16-bit RGB", size, got)
		}
		if rgba.Bounds().Size() != size {
			t.Fatalf("decoded size %v, want %v", rgba.Bounds().Size(), size)
		}
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				p := rgba.RGBA64At(x, y)
				for c, v := range []uint16{p.R, p.G, p.B} {
					if want := img.At(x, y, c); v != want {
						t.Fatalf("%v: pixel (%d,%d) channel %d = %d, want %d", size, x, y, c, v, want)
					}
				}
			}
		}
	}
}

func TestEncodeTags(t *testing.T) {
	xmp := []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"></x:xmpmeta>`)
	data := encode(t, testImage(37, 23), Options{PPI: 600, XMP: xmp})

	if string(data[:4]) != "II*\x00" {
		t.Fatalf("header %q, want little-endian TIFF", data[:4])
	}
	tags, order := readIFD(t, data)

	want := map[uint16][]uint32{
		256: {37},         // ImageWidth
		257: {23},         // ImageLength
		258: {16, 16, 16}, // BitsPerSample
		259: {8},          // Compression: Deflate
		262: {2},          // PhotometricInterpretation: RGB
		277: {3},          // SamplesPerPixel
		282: {600, 1},     // XResolution
		283: {600, 1},     // YResolution
		284: {1},          // PlanarConfiguration: chunky
		296: {2},          // ResolutionUnit: inch
		317: {2},          // Predictor: horizontal differencing
	}
	for tag, w := range want {
		got, ok := tags[tag]
		if !ok {
			t.Errorf("tag %d missing", tag)
			continue
		}
		if !equal(got.values, w) {
			t.Errorf("tag %d = %v, want %v", tag, got.values, w)
		}
	}
	if _, ok := tags[338]; ok {
		t.Error("ExtraSamples present: a master has no alpha channel")
	}
	if got := tags[700]; !bytes.Equal(got.raw, xmp) {
		t.Errorf("XMP = %q, want %q", got.raw, xmp)
	}
	for i := 1; i < len(order); i++ {
		if order[i] <= order[i-1] {
			t.Errorf("tags out of order: %d after %d", order[i], order[i-1])
		}
	}
}

func TestEncodeWithoutXMPHasNoXMPTag(t *testing.T) {
	tags, _ := readIFD(t, encode(t, testImage(2, 2), Options{PPI: 1200}))
	if _, ok := tags[700]; ok {
		t.Error("XMP tag present with no XMP given")
	}
	if got := tags[282].values; !equal(got, []uint32{1200, 1}) {
		t.Errorf("XResolution = %v, want 1200/1", got)
	}
}

func TestEncodeRefusesNoResolution(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, testImage(2, 2), Options{}); err == nil {
		t.Error("Encode with no PPI succeeded; a master must record its resolution")
	}
}

type entry struct {
	values []uint32 // SHORT, LONG and RATIONAL values
	raw    []byte   // BYTE and UNDEFINED values
}

// readIFD reads the first image file directory of a little-endian TIFF.
func readIFD(t *testing.T, data []byte) (map[uint16]entry, []uint16) {
	t.Helper()
	le := binary.LittleEndian
	off := le.Uint32(data[4:8])
	n := int(le.Uint16(data[off:]))
	tags := map[uint16]entry{}
	var order []uint16
	for i := 0; i < n; i++ {
		e := data[int(off)+2+12*i:]
		tag, typ, count := le.Uint16(e[0:]), le.Uint16(e[2:]), le.Uint32(e[4:])
		size := map[uint16]uint32{1: 1, 3: 2, 4: 4, 5: 8, 7: 1}[typ]
		if size == 0 {
			t.Fatalf("tag %d has unexpected type %d", tag, typ)
		}
		val := e[8:12]
		if size*count > 4 {
			p := le.Uint32(e[8:])
			val = data[p : p+size*count]
		}
		var en entry
		switch typ {
		case 1, 7:
			en.raw = append([]byte(nil), val[:count]...)
		case 3:
			for j := uint32(0); j < count; j++ {
				en.values = append(en.values, uint32(le.Uint16(val[2*j:])))
			}
		case 4:
			for j := uint32(0); j < count; j++ {
				en.values = append(en.values, le.Uint32(val[4*j:]))
			}
		case 5:
			for j := uint32(0); j < 2*count; j++ {
				en.values = append(en.values, le.Uint32(val[4*j:]))
			}
		}
		tags[tag] = en
		order = append(order, tag)
	}
	return tags, order
}

func equal(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
