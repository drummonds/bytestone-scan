package master

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/tiff"
)

func TestDecodeReadsBackWhatEncodeWrote(t *testing.T) {
	img := testImage(37, 23)
	got, err := Decode(bytes.NewReader(encode(t, img, Options{PPI: 600})))
	if err != nil {
		t.Fatal(err)
	}
	if got.W != img.W || got.H != img.H {
		t.Fatalf("size %d × %d", got.W, got.H)
	}
	for i := range img.Pix {
		if got.Pix[i] != img.Pix[i] {
			t.Fatalf("sample %d = %d, want %d", i, got.Pix[i], img.Pix[i])
		}
	}
}

// Story 039: a back is kept at 8 bits per channel; it reads back with each
// sample widened to 16 bits.
func TestEightBitMastersReadBackWidened(t *testing.T) {
	img := NewImage(3, 2)
	for i := range img.Pix {
		img.Pix[i] = uint16(i*0x1111) + 0x80 // the low byte is dropped at 8 bits
	}
	var buf bytes.Buffer
	if err := Encode(&buf, img, Options{PPI: 300, Depth: 8}); err != nil {
		t.Fatal(err)
	}
	got, err := Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range img.Pix {
		if want := v&^0xff | v>>8; got.Pix[i] != want {
			t.Fatalf("sample %d: %04x, want %04x", i, got.Pix[i], want)
		}
	}
	// any other depth is refused
	if err := Encode(&buf, img, Options{PPI: 300, Depth: 12}); err == nil {
		t.Error("12-bit master encoded")
	}
}

// A plain 8-bit TIFF from elsewhere decodes too, as the back's does.
func TestDecodeReadsAnEightBitImage(t *testing.T) {
	var buf bytes.Buffer
	rgba := image.NewRGBA(image.Rect(0, 0, 2, 2))
	rgba.Set(0, 0, color.White)
	rgba.Set(1, 1, color.RGBA{0x12, 0x34, 0x56, 0xff})
	tiff.Encode(&buf, rgba, nil)
	m, err := Decode(&buf)
	if err != nil || m.At(0, 0, 0) != 0xffff || m.At(1, 1, 0) != 0x1212 || m.At(1, 1, 2) != 0x5656 {
		t.Errorf("8-bit TIFF: %+v, %v", m, err)
	}
}
