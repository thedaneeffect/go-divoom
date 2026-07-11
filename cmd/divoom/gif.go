package main

import (
	"image"
	"image/draw"
	"image/gif"
)

// gifFrames composites an animated GIF's frames onto a full-size canvas and
// returns one full-bounds image.Image per frame.
//
// Animated GIFs are frequently encoded so that each frame after the first
// is only a sub-rectangle delta covering the pixels that changed, not the
// whole picture — g.Image[i].Bounds() may be far smaller than the logical
// screen. Handing those raw frames to SendAnimation would stretch each
// delta rectangle to fill the entire display, producing garbage for any
// optimized GIF. This instead paints each frame over a persistent canvas
// at its own offset (draw.Over, so transparent delta pixels leave the prior
// frame showing through) and snapshots a copy after each frame, giving
// disposal method "none" semantics (leave the previous frame in place).
//
// The rarer "restore to background" and "restore to previous" disposal
// methods (gif.DisposalBackground / DisposalPrevious) are not implemented;
// frames relying on them will retain stale pixels instead of being cleared
// or reverted. That is an accepted v1 limitation.
func gifFrames(g *gif.GIF) []image.Image {
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		bounds = g.Image[0].Bounds()
	}

	canvas := image.NewRGBA(bounds)
	frames := make([]image.Image, len(g.Image))
	for i, frame := range g.Image {
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		snap := image.NewRGBA(bounds)
		draw.Draw(snap, bounds, canvas, bounds.Min, draw.Src)
		frames[i] = snap
	}
	return frames
}
