package divoom

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math/bits"
	"time"
	xdraw "golang.org/x/image/draw"
)

// paletteImage converts a square 16x16 or 32x32 image into a first-seen-order
// color palette and per-pixel palette indices (row-major).
// Colors are read as straight (non-premultiplied) alpha via NRGBAModel, since
// the reference implementation reads raw RGBA channels; Color.RGBA() would
// return premultiplied values and darken semi-transparent pixels.
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
			p := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			c := [3]uint8{p.R, p.G, p.B}
			if p.A <= 32 {
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

// frameData assembles one frame body: time code, palette flag, color count,
// palette, packed pixels. wide selects the 32x32 variant (flag 0x03,
// u16LE color count).
func frameData(timeMs int, multiFrame, wide bool, palette [][3]uint8, pixels []uint16) []byte {
	out := make([]byte, 0, 4+len(palette)*3+len(pixels)/2)
	if multiFrame {
		out = binary.LittleEndian.AppendUint16(out, uint16(timeMs))
	} else {
		out = append(out, 0x00, 0x00)
	}

	colorCount := len(palette)
	if colorCount >= len(pixels) {
		colorCount = 0 // reference implementation signals "max palette" as 0
	}
	if wide {
		out = append(out, 0x03)
		out = binary.LittleEndian.AppendUint16(out, uint16(colorCount))
	} else {
		out = append(out, 0x00, byte(colorCount))
	}
	for _, c := range palette {
		out = append(out, c[0], c[1], c[2])
	}
	return append(out, packPixels(pixels, len(palette))...)
}

// makeFrame wraps frame data in its container: 0xAA + u16LE(len+3) + data.
// The returned length field value is what animation size counters sum.
func makeFrame(data []byte) ([]byte, int) {
	length := len(data) + 3
	out := make([]byte, 0, length)
	out = append(out, 0xAA)
	out = binary.LittleEndian.AppendUint16(out, uint16(length))
	return append(out, data...), length
}

// fitImage resizes img to the profile's small size when the source is small,
// otherwise to the full screen size, using nearest-neighbor to keep pixel art crisp.
func (p Profile) fitImage(img image.Image) image.Image {
	b := img.Bounds()
	target := p.ScreenSize
	if p.SmallSize > 0 && b.Dx() <= p.SmallSize && b.Dy() <= p.SmallSize {
		target = p.SmallSize
	}
	if b.Dx() == target && b.Dy() == target {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, target, target))
	xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}

// imageMessages encodes a static image as wire messages (command 0x44).
func (p Profile) imageMessages(img image.Image) ([][]byte, error) {
	img = p.fitImage(img)
	palette, pixels, err := paletteImage(img)
	if err != nil {
		return nil, err
	}
	wide := img.Bounds().Dx() == 32
	frame, _ := makeFrame(frameData(0, false, wide, palette, pixels))
	args := append([]byte{0x00, 0x0A, 0x0A, 0x04}, frame...)
	return [][]byte{makeCommand(0x44, args)}, nil
}

// animationMessages encodes frames as chunked animation wire messages
// (command 0x49). All frames are resized to a common resolution.
func (p Profile) animationMessages(frames []image.Image, frameTime time.Duration) ([][]byte, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("divoom: animation needs at least one frame")
	}
	timeMs := int(frameTime.Milliseconds())

	// The whole animation must use one resolution: 32 if any frame is large.
	wide := false
	for _, f := range frames {
		fitted := p.fitImage(f)
		if fitted.Bounds().Dx() == p.ScreenSize && p.ScreenSize == 32 {
			wide = true
		}
	}

	var stream []byte
	var total int
	if wide {
		// Pixoo Max expects two flag frames before 32x32 animation data.
		for _, flag := range [][]byte{{0x00, 0x00, 0x05, 0x00, 0x00}, {0x00, 0x00, 0x06, 0x00, 0x00, 0x00}} {
			b, l := makeFrame(flag)
			stream = append(stream, b...)
			total += l
		}
	}
	for _, f := range frames {
		fitted := p.fitImage(f)
		if wide && fitted.Bounds().Dx() != p.ScreenSize {
			fitted = forceSize(fitted, p.ScreenSize)
		}
		palette, pixels, err := paletteImage(fitted)
		if err != nil {
			return nil, err
		}
		b, l := makeFrame(frameData(timeMs, len(frames) > 1, wide, palette, pixels))
		stream = append(stream, b...)
		total += l
	}

	var msgs [][]byte
	for i := 0; i*p.ChunkSize < len(stream); i++ {
		chunk := stream[i*p.ChunkSize : min((i+1)*p.ChunkSize, len(stream))]
		var args []byte
		if p.WideCounters {
			args = binary.LittleEndian.AppendUint32(nil, uint32(total))
			args = binary.LittleEndian.AppendUint16(args, uint16(i))
		} else {
			args = binary.LittleEndian.AppendUint16(nil, uint16(total))
			args = append(args, byte(i))
		}
		msgs = append(msgs, makeCommand(0x49, append(args, chunk...)))
	}
	return msgs, nil
}

// forceSize resizes img to exactly size x size.
func forceSize(img image.Image, size int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Src, nil)
	return dst
}
