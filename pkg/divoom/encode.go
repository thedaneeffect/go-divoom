package divoom

import (
	"fmt"
	"image"
	"math/bits"
)

// paletteImage converts a square 16x16 or 32x32 image into a first-seen-order
// color palette and per-pixel palette indices (row-major).
// Mostly-transparent pixels (alpha <= 32) render as black, matching the
// reference implementation.
func paletteImage(img image.Image) (palette [][3]uint8, pixels []uint16, err error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w != h || (w != 16 && w != 32) {
		return nil, nil, fmt.Errorf("divoom: image must be 16x16 or 32x32, got %dx%d", w, h)
	}

	index := map[[3]uint8]uint16{}
	pixels = make([]uint16, 0, w*h)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			c := [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
			if a>>8 <= 32 {
				c = [3]uint8{0, 0, 0}
			}
			i, ok := index[c]
			if !ok {
				i = uint16(len(palette))
				index[c] = i
				palette = append(palette, c)
			}
			pixels = append(pixels, i)
		}
	}
	return palette, pixels, nil
}

// packPixels bit-packs palette indices using the minimal number of bits per
// pixel, accumulating little-endian within each byte.
func packPixels(pixels []uint16, paletteLen int) []byte {
	bpp := bits.Len(uint(paletteLen - 1))
	if bpp == 0 {
		bpp = 1
	}

	out := make([]byte, 0, (len(pixels)*bpp+7)/8)
	var acc uint32
	var nbits int
	for _, p := range pixels {
		acc |= uint32(p&(1<<bpp-1)) << nbits
		nbits += bpp
		for nbits >= 8 {
			out = append(out, byte(acc))
			acc >>= 8
			nbits -= 8
		}
	}
	if nbits > 0 {
		out = append(out, byte(acc))
	}
	return out
}
