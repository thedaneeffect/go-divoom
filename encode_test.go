package divoom

import (
	"bytes"
	"image"
	"image/color"
	"testing"
	"time"
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
	for y := range 16 {
		for x := range 16 {
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

// Semi-transparent pixels above the alpha threshold must keep their straight
// (non-premultiplied) color, matching the Python reference which reads raw
// RGBA channels. Premultiplied conversion would yield {100,0,0} here.
func TestPaletteImageStraightAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.Set(x, y, color.NRGBA{255, 0, 0, 255})
		}
	}
	img.Set(5, 5, color.NRGBA{200, 0, 0, 128}) // alpha 128 > 32 threshold

	palette, pixels, err := paletteImage(img)
	if err != nil {
		t.Fatal(err)
	}
	if len(palette) != 2 || palette[1] != [3]uint8{200, 0, 0} {
		t.Fatalf("palette = %v, want second entry {200 0 0}", palette)
	}
	if pixels[5*16+5] != 1 {
		t.Errorf("semi-transparent pixel index = %d, want 1", pixels[5*16+5])
	}
}

func TestPaletteImageRejectsBadSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	if _, _, err := paletteImage(img); err == nil {
		t.Error("expected error for 20x20 image")
	}
}

func fill16(c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.Set(x, y, c)
		}
	}
	return img
}

func checker32() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	return img
}

// Goldens from testdata/gen_goldens.py.
func TestFrameData16Red(t *testing.T) {
	palette, pixels, err := paletteImage(fill16(color.RGBA{255, 0, 0, 255}))
	if err != nil {
		t.Fatal(err)
	}
	got := frameData(0, false, false, palette, pixels)
	want := mustHex(t, "00000001ff00000000000000000000000000000000000000000000000000000000000000000000")
	if !bytes.Equal(got, want) {
		t.Errorf("got  %x\nwant %x", got, want)
	}
}

func TestFrameData32Checker(t *testing.T) {
	palette, pixels, err := paletteImage(checker32())
	if err != nil {
		t.Fatal(err)
	}
	got := frameData(0, false, true, palette, pixels)
	want := mustHex(t, "0000030200000000ffffffaaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555")
	if !bytes.Equal(got, want) {
		t.Errorf("got  %x\nwant %x", got, want)
	}
}

func TestMakeFrame(t *testing.T) {
	palette, pixels, _ := paletteImage(fill16(color.RGBA{255, 0, 0, 255}))
	frame, length := makeFrame(frameData(0, false, false, palette, pixels))
	if length != 42 {
		t.Errorf("length field = %d, want 42", length)
	}
	want := mustHex(t, "aa2a0000000001ff00000000000000000000000000000000000000000000000000000000000000000000")
	if !bytes.Equal(frame, want) {
		t.Errorf("got  %x\nwant %x", frame, want)
	}
}

func TestImageMessages16Red(t *testing.T) {
	msgs, err := PixooMax.imageMessages(fill16(color.RGBA{255, 0, 0, 255}))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := mustHex(t, "01310044000a0a04aa2a0000000001ff00000000000000000000000000000000000000000000000000000000000000000000610202")
	if !bytes.Equal(msgs[0], want) {
		t.Errorf("got  %x\nwant %x", msgs[0], want)
	}
}

func TestAnimationMessages(t *testing.T) {
	frames := []image.Image{
		fill16(color.RGBA{255, 0, 0, 255}),
		fill16(color.RGBA{0, 0, 0, 255}),
	}
	msgs, err := PixooMax.animationMessages(frames, 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	want := mustHex(t, "015d0049540000000000aa2a00f4010001ff00000000000000000000000000000000000000000000000000000000000000000000aa2a00f401000100000000000000000000000000000000000000000000000000000000000000000000008d0502")
	if !bytes.Equal(msgs[0], want) {
		t.Errorf("got  %x\nwant %x", msgs[0], want)
	}
}

func fill32(c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.Set(x, y, c)
		}
	}
	return img
}

// Golden from testdata/gen_goldens.py (hass-divoom reference): 2-frame 32x32
// animation (checkerboard + solid red, 250ms) -> flag frames prepended,
// u32LE total 298, two 200-byte chunks.
func TestAnimationMessages32Wide(t *testing.T) {
	frames := []image.Image{checker32(), fill32(color.RGBA{255, 0, 0, 255})}
	msgs, err := PixooMax.animationMessages(frames, 250*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	want0 := mustHex(t, "01d100492a0100000000aa08000000050000aa0900000006000000aa8e00fa00030200000000ffffffaaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aaaaaaaa55555555aa8b00fa00030100ff0000000000000000000000000000000000000000000000000000000000000000db4a02")
	want1 := mustHex(t, "016b00492a01000001000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000e00002")
	if !bytes.Equal(msgs[0], want0) {
		t.Errorf("chunk 0:\ngot  %x\nwant %x", msgs[0], want0)
	}
	if !bytes.Equal(msgs[1], want1) {
		t.Errorf("chunk 1:\ngot  %x\nwant %x", msgs[1], want1)
	}
}

func TestFitImageResizes(t *testing.T) {
	big := image.NewRGBA(image.Rect(0, 0, 300, 200))
	got := PixooMax.fitImage(big)
	if b := got.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("resized to %dx%d, want 32x32", b.Dx(), b.Dy())
	}
	small := image.NewRGBA(image.Rect(0, 0, 10, 10))
	got = PixooMax.fitImage(small)
	if b := got.Bounds(); b.Dx() != 16 {
		t.Errorf("small image resized to %d, want 16", b.Dx())
	}
}
