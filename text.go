package divoom

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// TextOptions configures ShowText. Zero values mean white text, black
// background, 100ms per frame, and the built-in bitmap font.
type TextOptions struct {
	Color      [3]uint8
	Background [3]uint8
	FrameTime  time.Duration

	// FontPath, if set, is a TTF/OTF file loaded and rendered instead of the
	// built-in bitmap font (basicfont.Face7x13). It is resolved on whichever
	// process actually renders the frames: when a command routes through
	// `divoom serve`, that's the daemon's host, not necessarily the machine
	// running the CLI.
	//
	// Empty (the default) preserves the exact pre-existing behavior: no font
	// file is read, and the built-in face is used.
	FontPath string

	// FontSize is the point size to render FontPath at. Zero uses
	// defaultFontSize. Ignored when FontPath is empty.
	FontSize float64
}

// defaultTextFrameTime sets the scroll pace: one pixel every 62ms is ~16px/sec,
// a bit over two characters per second, which reads briskly without blurring.
// A 1px step is what makes this speed legible — the same rate at the 2px step
// this code used to take reads as a judder.
const defaultTextFrameTime = 62 * time.Millisecond

// defaultFontSize is used when FontSize is zero. 16pt at fontDPI (72 — see
// below) renders glyphs with roughly a 16px em box: comfortably close to half
// the 32px panel's height, leaving headroom above and below for ascenders and
// descenders without the text looking cramped the way a 10-12pt face would.
const defaultFontSize = 16

// fontDPI is the resolution loadFace assumes when turning FontSize (points)
// into a rasterization scale. At 72 DPI, 1 point == 1 pixel (72 points and 72
// dots both per inch) — the same "a point is a pixel" convention most
// on-screen (as opposed to print) rendering stacks use. That lets FontSize be
// read directly as an approximate glyph pixel height on the 32px panel,
// rather than requiring a points-to-pixels conversion for a display whose
// physical DPI nobody actually knows or cares about.
const fontDPI = 72

// loadFace parses the TTF/OTF file at path and returns a font.Face rendering
// it at the given point size (see fontDPI). size <= 0 uses defaultFontSize.
// Errors are wrapped so a bad, missing, or corrupt font file surfaces
// clearly instead of the caller having to guess which path failed to load.
func loadFace(path string, size float64) (font.Face, error) {
	if size <= 0 {
		size = defaultFontSize
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("divoom: load font %s: %w", path, err)
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("divoom: load font %s: %w", path, err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     fontDPI,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("divoom: load font %s: %w", path, err)
	}
	return face, nil
}

// ValidateFont reports whether path parses as a TTF/OTF file at the given
// size, without retaining the resulting face. path == "" (no font
// requested) is always valid.
//
// This exists so a caller that holds a long-lived, reusable device
// connection — namely the HTTP daemon's handleText — can check a font path
// before ever touching the device. Without it, a bad -font value surfaces as
// an error from inside ShowText indistinguishable, to a generic
// error-means-drop-the-connection handler, from a genuine transport
// failure: it would tear down and force a reconnect of a perfectly healthy
// Bluetooth link over what is actually just a client input mistake
// (reproduced against real hardware).
func ValidateFont(path string, size float64) error {
	if path == "" {
		return nil
	}
	face, err := loadFace(path, size)
	if err != nil {
		return err
	}
	return face.Close()
}

// ShowText scrolls text across the display as an animation.
func (d *Device) ShowText(text string, o TextOptions) error {
	if o.Color == [3]uint8{} {
		o.Color = [3]uint8{255, 255, 255}
	}
	if o.FrameTime == 0 {
		o.FrameTime = defaultTextFrameTime
	}

	face := font.Face(basicfont.Face7x13)
	if o.FontPath != "" {
		loaded, err := loadFace(o.FontPath, o.FontSize)
		if err != nil {
			return err
		}
		defer loaded.Close()
		face = loaded
	}

	frames := renderTextFrames(text, d.p.ScreenSize, face, o.Color, o.Background)
	return d.SendAnimation(frames, o.FrameTime)
}

// maxAnimationFrames bounds how many frames one scrolling message uses.
//
// It is NOT a device limit. The reference implementation warns that animations
// over 60 frames are "very likely cut off", and this code inherited that number
// on faith. Measured on real hardware (a counter animation whose frames display
// their own index, so truncation is directly observable): with paced writes the
// device plays back 240 frames / ~34KB intact — every truncation previously
// blamed on a frame cap was actually the receive-buffer overrun that chunkPacing
// now prevents. See docs/superpowers/specs/hardware-smoke.md.
//
// So this is a budget, not a ceiling: it caps upload time (each frame is a
// paced write) and keeps the scroll step at 1px for any sane message length.
// Longer messages step wider rather than taking forever to upload.
const maxAnimationFrames = 240

// scrollFrames is how many frames it takes to walk the text fully off the
// screen at the given step. Frames run 0..count-1, so the last one sits at
// offset (count-1)*speed: without the +1 the scroll stops a step short and
// clips the tail of the message mid-character.
func scrollFrames(width, size, speed int) int {
	return (width-size+speed-1)/speed + 1
}

// renderTextFrames renders text into scroll animation frames of size x size,
// drawing glyphs with face.
func renderTextFrames(text string, size int, face font.Face, fg, bg [3]uint8) []image.Image {
	textWidth := font.MeasureString(face, text).Ceil()

	// Canvas: blank screen, text, blank screen.
	width := size + textWidth + size
	canvas := image.NewRGBA(image.Rect(0, 0, width, size))
	bgColor := color.RGBA{bg[0], bg[1], bg[2], 255}
	fgColor := color.RGBA{fg[0], fg[1], fg[2], 255}
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(bgColor), image.Point{}, draw.Src)
	dot := fixed.P(size, size/2+face.Metrics().Ascent.Ceil()/2)
	drawTextThresholded(canvas, face, dot, text, fgColor)

	// Scroll a pixel at a time for smoothness, stepping wider only when the
	// frame count would exceed what the device stores. Longer messages are
	// therefore jumpier, but only past a few hundred pixels of travel.
	speed := 1
	for scrollFrames(width, size, speed) > maxAnimationFrames {
		speed *= 2
	}

	// Round up, so the last frame lands past the end of the text rather than
	// clipping its final pixels: floor division here left the tail of the
	// message cut off mid-character.
	count := max(scrollFrames(width, size, speed), 1)
	frames := make([]image.Image, 0, count)
	for i := 0; i < count; i++ {
		f := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.Draw(f, f.Bounds(), canvas, image.Pt(i*speed, 0), draw.Src)
		frames = append(frames, f)
	}
	return frames
}

// alphaThreshold is the coverage cutoff drawTextThresholded uses to decide a
// glyph pixel is foreground: 0x8000 is 50% of the 16-bit range color.Color's
// RGBA method returns alpha in.
const alphaThreshold = 0x8000

// drawTextThresholded draws s onto dst starting at dot, painting each glyph
// pixel EITHER fg OR whatever was already in dst (bg, since renderTextFrames
// pre-fills the canvas) — never a blend of the two.
//
// This exists because vector fonts anti-alias glyph edges: golang.org/x/image/
// font.Drawer.DrawString composites that coverage mask onto dst with
// draw.Over, turning edge pixels into a gradient of in-between colors. On
// this device that gradient is expensive, not pretty: colors in a frame
// determine bits per pixel (paletteImage/packPixels in encode.go), so a
// handful of antialiased grays turns a 2-color, ~142-byte frame into an
// 8ish-color, ~416-byte one — three times the wire cost for a blurrier
// result on a 32x32 panel that has no subpixels to spare. Thresholding
// coverage at 50% keeps every frame exactly 2 colors regardless of which
// font is loaded, matching the built-in bitmap font's (already binary) mask
// exactly, so this is also safe to use unconditionally rather than only for
// loaded fonts.
func drawTextThresholded(dst draw.Image, face font.Face, dot fixed.Point26_6, s string, fg color.Color) {
	prev := rune(-1)
	for _, r := range s {
		if prev >= 0 {
			dot.X += face.Kern(prev, r)
		}
		dr, mask, maskp, advance, _ := face.Glyph(dot, r)
		if !dr.Empty() {
			for y := dr.Min.Y; y < dr.Max.Y; y++ {
				for x := dr.Min.X; x < dr.Max.X; x++ {
					_, _, _, a := mask.At(maskp.X+(x-dr.Min.X), maskp.Y+(y-dr.Min.Y)).RGBA()
					if a >= alphaThreshold {
						dst.Set(x, y, fg)
					}
				}
			}
		}
		dot.X += advance
		prev = r
	}
}
