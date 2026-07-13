package divoom

import (
	"image"
	"testing"
)

func TestRenderTextFrames(t *testing.T) {
	frames := renderTextFrames("HI", 32, [3]uint8{255, 255, 255}, [3]uint8{0, 0, 0})
	if len(frames) < 2 {
		t.Fatalf("got %d frames, want at least 2 (scroll)", len(frames))
	}
	if len(frames) > 60 {
		t.Fatalf("got %d frames, want <= 60 (device frame limit)", len(frames))
	}
	for i, f := range frames {
		if b := f.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
			t.Fatalf("frame %d is %dx%d, want 32x32", i, b.Dx(), b.Dy())
		}
	}
	// First frame starts with text off-screen right: leftmost column is background.
	first := frames[0].(*image.RGBA)
	r, g, b, _ := first.At(0, 16).RGBA()
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("first frame left edge not background: %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestRenderTextFramesLongTextCapped(t *testing.T) {
	frames := renderTextFrames("THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG", 32, [3]uint8{255, 0, 0}, [3]uint8{0, 0, 0})
	if len(frames) > 60 {
		t.Fatalf("got %d frames, want <= 60", len(frames))
	}
}

func TestShowTextSendsAnimation(t *testing.T) {
	d, ft := newFakeDevice()
	if err := d.ShowText("HI", TextOptions{}); err != nil {
		t.Fatal(err)
	}
	if ft.Len() == 0 {
		t.Fatal("nothing written to transport")
	}
	// Wire bytes must be animation chunks (command 0x49 after 0x01 + u16 length).
	if got := ft.Bytes()[3]; got != 0x49 {
		t.Errorf("first command byte = %#x, want 0x49", got)
	}
}

// TestRenderTextFramesScrollsFully guards the bug where floor division ended the
// scroll early, cutting the tail of the message mid-character: the last frame
// must contain no text pixels at all, i.e. the message has fully left the screen.
func TestRenderTextFramesScrollsFully(t *testing.T) {
	for _, msg := range []string{"HI", "Hello, world", "a much longer message that must still finish"} {
		frames := renderTextFrames(msg, 32, [3]uint8{255, 255, 255}, [3]uint8{0, 0, 0})
		if len(frames) > maxAnimationFrames {
			t.Errorf("%q: %d frames exceeds the device's %d-frame buffer", msg, len(frames), maxAnimationFrames)
		}
		last := frames[len(frames)-1]
		for y := range 32 {
			for x := range 32 {
				r, g, b, _ := last.At(x, y).RGBA()
				if r|g|b != 0 {
					t.Errorf("%q: last frame still shows text at (%d,%d) — scroll ended early", msg, x, y)
					break
				}
			}
		}
	}
}
