package compress

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPNGKeepsDimensions(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 3), B: 128, A: 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		t.Fatal(err)
	}

	out, err := PNG(raw.Bytes())
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if decoded.Bounds() != src.Bounds() {
		t.Fatalf("bounds = %v, want %v", decoded.Bounds(), src.Bounds())
	}
}

func TestPNGRejectsInvalidData(t *testing.T) {
	if _, err := PNG([]byte("not a png")); err == nil {
		t.Fatal("expected error for invalid PNG")
	}
}
