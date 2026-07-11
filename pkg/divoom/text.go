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
// background, 50ms per frame.
type TextOptions struct {
	Color      [3]uint8
	Background [3]uint8
	FrameTime  time.Duration
}

// ShowText scrolls text across the display as an animation.
func (d *Device) ShowText(text string, o TextOptions) error {
	if o.Color == [3]uint8{} {
		o.Color = [3]uint8{255, 255, 255}
	}
	if o.FrameTime == 0 {
		o.FrameTime = 50 * time.Millisecond
	}
	frames := renderTextFrames(text, d.p.ScreenSize, o.Color, o.Background)
	imgs := make([]image.Image, len(frames))
	copy(imgs, frames)
	return d.SendAnimation(imgs, o.FrameTime)
}

// renderTextFrames renders text into scroll animation frames of size x size.
// Scroll speed doubles until the frame count fits the device's 60-frame limit,
// mirroring the reference implementation.
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

	speed := size / 16 // 2px per frame on a 32px screen
	if speed < 1 {
		speed = 1
	}
	for (width-size)/speed > 60 {
		speed *= 2
	}

	count := (width - size) / speed
	if count < 1 {
		count = 1
	}
	frames := make([]image.Image, 0, count)
	for i := 0; i < count; i++ {
		f := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.Draw(f, f.Bounds(), canvas, image.Pt(i*speed, 0), draw.Src)
		frames = append(frames, f)
	}
	return frames
}
