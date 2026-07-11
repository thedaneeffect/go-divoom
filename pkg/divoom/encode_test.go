package divoom

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// Goldens from testdata/gen_goldens.py (hass-divoom reference).
func TestPackPixels(t *testing.T) {
	cases := []struct {
		name    string
		pixels  []uint16
		colors  int
		wantHex string
	}{
		{"1 color 16px", make([]uint16, 16), 1, "0000"},
		{"2 colors alternating 16px", []uint16{0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1}, 2, "aaaa"},
		{"3 colors 8px", []uint16{0, 1, 2, 0, 1, 2, 0, 1}, 3, "2449"},
		{"5 colors 8px", []uint16{0, 1, 2, 3, 4, 0, 1, 2}, 5, "884644"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := packPixels(c.pixels, c.colors)
			want := mustHex(t, c.wantHex)
			if !bytes.Equal(got, want) {
				t.Errorf("got %x, want %x", got, want)
			}
		})
	}
}

func TestPaletteImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	red := color.RGBA{255, 0, 0, 255}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, red)
		}
	}
	// One transparent pixel must become black, appended second in the palette.
	img.Set(3, 2, color.RGBA{0, 0, 0, 0})

	palette, pixels, err := paletteImage(img)
	if err != nil {
		t.Fatal(err)
	}
	if len(palette) != 2 || palette[0] != [3]uint8{255, 0, 0} || palette[1] != [3]uint8{0, 0, 0} {
		t.Fatalf("palette = %v", palette)
	}
	if len(pixels) != 256 {
		t.Fatalf("len(pixels) = %d", len(pixels))
	}
	if pixels[2*16+3] != 1 {
		t.Errorf("transparent pixel index = %d, want 1", pixels[2*16+3])
	}
	if pixels[0] != 0 {
		t.Errorf("first pixel index = %d, want 0", pixels[0])
	}
}

func TestPaletteImageRejectsBadSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	if _, _, err := paletteImage(img); err == nil {
		t.Error("expected error for 20x20 image")
	}
}
