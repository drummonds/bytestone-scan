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

func TestDecodeRefusesAnEightBitImage(t *testing.T) {
	var buf bytes.Buffer
	rgba := image.NewRGBA(image.Rect(0, 0, 2, 2))
	rgba.Set(0, 0, color.White)
	tiff.Encode(&buf, rgba, nil)
	if _, err := Decode(&buf); err == nil {
		t.Error("an 8-bit TIFF decoded as a master")
	}
}
