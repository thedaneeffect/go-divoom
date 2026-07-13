package main

import (
	"image"
	"image/color"
	"image/gif"
	"testing"
)

// TestGifFramesCompositesDeltaOverFullCanvas exercises the common
// optimized-GIF case: later frames only encode the sub-rectangle that
// changed, not the whole frame. gifFrames must composite each delta onto a
// persistent full-size canvas rather than stretching it, so pixels outside
// the delta retain whatever the previous frame drew there.
func TestGifFramesCompositesDeltaOverFullCanvas(t *testing.T) {
	const w, h = 8, 8
	pal := color.Palette{color.RGBA{0, 0, 0, 0}, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}}

	// Frame 1 fully covers the canvas with red.
	frame1 := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	for y := range h {
		for x := range w {
			frame1.SetColorIndex(x, y, 1)
		}
	}

	// Frame 2 is a small green sub-rectangle delta, offset from the origin,
	// as an optimized encoder would emit for a mostly-static animation.
	deltaRect := image.Rect(2, 2, 5, 5)
	frame2 := image.NewPaletted(deltaRect, pal)
	for y := deltaRect.Min.Y; y < deltaRect.Max.Y; y++ {
		for x := deltaRect.Min.X; x < deltaRect.Max.X; x++ {
			frame2.SetColorIndex(x, y, 2)
		}
	}

	g := &gif.GIF{
		Image:  []*image.Paletted{frame1, frame2},
		Delay:  []int{10, 10},
		Config: image.Config{Width: w, Height: h},
	}

	frames := gifFrames(g)
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}

	full := image.Rect(0, 0, w, h)
	if got := frames[0].Bounds(); got != full {
		t.Errorf("frame 1 bounds = %v, want %v", got, full)
	}
	f2 := frames[1]
	if got := f2.Bounds(); got != full {
		t.Errorf("frame 2 bounds = %v, want full canvas %v (not stretched delta rect)", got, full)
	}

	// Inside the delta rect: frame 2's own green pixels.
	if r, gr, b, _ := f2.At(3, 3).RGBA(); r>>8 != 0 || gr>>8 != 255 || b>>8 != 0 {
		t.Errorf("pixel inside delta (3,3) = (%d,%d,%d), want green (0,255,0)", r>>8, gr>>8, b>>8)
	}

	// Outside the delta rect: frame 1's red must still be there, proving the
	// delta was composited rather than stretched over the whole canvas.
	if r, gr, b, _ := f2.At(0, 0).RGBA(); r>>8 != 255 || gr>>8 != 0 || b>>8 != 0 {
		t.Errorf("pixel outside delta (0,0) = (%d,%d,%d), want red (100,0,0) retained from frame 1", r>>8, gr>>8, b>>8)
	}
	if r, gr, b, _ := f2.At(7, 7).RGBA(); r>>8 != 255 || gr>>8 != 0 || b>>8 != 0 {
		t.Errorf("pixel outside delta (7,7) = (%d,%d,%d), want red retained from frame 1", r>>8, gr>>8, b>>8)
	}
}

// TestGifFramesFallsBackWhenConfigIsZero covers GIFs with no logical screen
// descriptor size (Config.Width/Height left zero), which should fall back
// to the first frame's own bounds instead of producing an empty canvas.
func TestGifFramesFallsBackWhenConfigIsZero(t *testing.T) {
	pal := color.Palette{color.RGBA{0, 0, 0, 255}}
	frame := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	g := &gif.GIF{Image: []*image.Paletted{frame}}

	frames := gifFrames(g)
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	if got, want := frames[0].Bounds(), image.Rect(0, 0, 4, 4); got != want {
		t.Errorf("bounds = %v, want %v", got, want)
	}
}

// TestGifFramesReturnsIndependentCopies ensures each returned frame is a
// snapshot: mutating the shared compositing canvas for a later frame must
// not retroactively change pixels already returned for an earlier frame.
func TestGifFramesReturnsIndependentCopies(t *testing.T) {
	pal := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}}
	frame1 := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	for y := range 4 {
		for x := range 4 {
			frame1.SetColorIndex(x, y, 0)
		}
	}
	frame2 := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	for y := range 4 {
		for x := range 4 {
			frame2.SetColorIndex(x, y, 1)
		}
	}

	g := &gif.GIF{Image: []*image.Paletted{frame1, frame2}, Config: image.Config{Width: 4, Height: 4}}
	frames := gifFrames(g)

	if r, gr, b, _ := frames[0].At(0, 0).RGBA(); r>>8 != 255 || gr>>8 != 0 || b>>8 != 0 {
		t.Errorf("frame 1 pixel = (%d,%d,%d), want red; later frame mutated earlier snapshot", r>>8, gr>>8, b>>8)
	}
	if r, gr, b, _ := frames[1].At(0, 0).RGBA(); r>>8 != 0 || gr>>8 != 255 || b>>8 != 0 {
		t.Errorf("frame 2 pixel = (%d,%d,%d), want green", r>>8, gr>>8, b>>8)
	}
}
