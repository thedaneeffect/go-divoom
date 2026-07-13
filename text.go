package divoom

import (
	"image"
	"image/color"
	"image/draw"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// TextOptions configures ShowText. Zero values mean white text, black
// background, and 100ms per frame.
type TextOptions struct {
	Color      [3]uint8
	Background [3]uint8
	FrameTime  time.Duration
}

// defaultTextFrameTime sets the scroll pace. The step size is not ours to
// choose for longer messages — the device's 60-frame buffer forces wider steps
// as text grows — so frame time is the only lever on readability. At 250ms a
// typical message crosses the screen at ~8px/sec, roughly a character per
// second. The reference implementation's 50ms is five times faster and reads as
// a blur.
const defaultTextFrameTime = 250 * time.Millisecond

// ShowText scrolls text across the display as an animation.
func (d *Device) ShowText(text string, o TextOptions) error {
	if o.Color == [3]uint8{} {
		o.Color = [3]uint8{255, 255, 255}
	}
	if o.FrameTime == 0 {
		o.FrameTime = defaultTextFrameTime
	}
	frames := renderTextFrames(text, d.p.ScreenSize, o.Color, o.Background)
	return d.SendAnimation(frames, o.FrameTime)
}

// maxAnimationFrames is how many frames the device will store for one
// animation. Uploads longer than this play back truncated.
const maxAnimationFrames = 60

// scrollFrames is how many frames it takes to walk the text fully off the
// screen at the given step. Frames run 0..count-1, so the last one sits at
// offset (count-1)*speed: without the +1 the scroll stops a step short and
// clips the tail of the message mid-character.
func scrollFrames(width, size, speed int) int {
	return (width-size+speed-1)/speed + 1
}

// renderTextFrames renders text into scroll animation frames of size x size.
func renderTextFrames(text string, size int, fg, bg [3]uint8) []image.Image {
	face := basicfont.Face7x13
	textWidth := font.MeasureString(face, text).Ceil()

	// Canvas: blank screen, text, blank screen.
	width := size + textWidth + size
	canvas := image.NewRGBA(image.Rect(0, 0, width, size))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{bg[0], bg[1], bg[2], 255}), image.Point{}, draw.Src)
	dr := font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(color.RGBA{fg[0], fg[1], fg[2], 255}),
		Face: face,
		Dot:  fixed.P(size, size/2+face.Metrics().Ascent.Ceil()/2),
	}
	dr.DrawString(text)

	// Scroll a pixel at a time for smoothness, stepping wider only when the
	// frame count would exceed what the device stores. Longer messages are
	// therefore necessarily jumpier — the device's animation buffer, not a
	// stylistic choice, is what sets the ceiling.
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
