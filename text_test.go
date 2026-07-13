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
