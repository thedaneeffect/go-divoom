package divoom

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
)

// testFontPath writes the Go Regular TTF (compiled into the x/image module
// we already depend on, so this works on any OS/CI without relying on
// system font paths) to a temp file and returns its path.
func testFontPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test-font.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatalf("write test font: %v", err)
	}
	return path
}

func TestRenderTextFrames(t *testing.T) {
	frames := renderTextFrames("HI", 32, basicfont.Face7x13, [3]uint8{255, 255, 255}, [3]uint8{0, 0, 0})
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
	frames := renderTextFrames("THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG", 32, basicfont.Face7x13, [3]uint8{255, 0, 0}, [3]uint8{0, 0, 0})
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
		frames := renderTextFrames(msg, 32, basicfont.Face7x13, [3]uint8{255, 255, 255}, [3]uint8{0, 0, 0})
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

// TestLoadFaceMissingFile asserts a nonexistent font path fails clearly
// instead of panicking.
func TestLoadFaceMissingFile(t *testing.T) {
	_, err := loadFace(filepath.Join(t.TempDir(), "does-not-exist.ttf"), 0)
	if err == nil {
		t.Fatal("expected an error for a missing font file")
	}
	if !strings.Contains(err.Error(), "divoom: load font") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "divoom: load font")
	}
}

// TestLoadFaceCorruptFile asserts garbage bytes fail to parse cleanly rather
// than panicking — a bad/missing/corrupt font must always return an error.
func TestLoadFaceCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.ttf")
	if err := os.WriteFile(path, []byte("this is not a font file, just garbage bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadFace(path, 16)
	if err == nil {
		t.Fatal("expected an error for a corrupt font file")
	}
	if !strings.Contains(err.Error(), "divoom: load font") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "divoom: load font")
	}
}

// TestShowTextInvalidFontPathErrors asserts ShowText surfaces a font load
// failure as an error instead of panicking or silently falling back.
func TestShowTextInvalidFontPathErrors(t *testing.T) {
	d, _ := newFakeDevice()
	err := d.ShowText("HI", TextOptions{FontPath: filepath.Join(t.TempDir(), "nope.ttf")})
	if err == nil {
		t.Fatal("expected an error for a missing font file")
	}
}

// TestValidateFontEmptyPathOK asserts an empty path (no font requested) is
// always valid — ValidateFont must not require a font to be present.
func TestValidateFontEmptyPathOK(t *testing.T) {
	if err := ValidateFont("", 0); err != nil {
		t.Errorf("ValidateFont(\"\", 0) = %v, want nil", err)
	}
}

// TestValidateFontValidPath asserts a real font loads cleanly.
func TestValidateFontValidPath(t *testing.T) {
	if err := ValidateFont(testFontPath(t), 16); err != nil {
		t.Errorf("ValidateFont on a valid TTF = %v, want nil", err)
	}
}

// TestValidateFontMissingOrCorrupt guards the reason ValidateFont exists:
// callers like the HTTP daemon need to distinguish "bad font input" from "a
// device fault" before ever touching the device, so this must return a
// clear error (never panic) for both a missing file and a corrupt one.
func TestValidateFontMissingOrCorrupt(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.ttf")
	if err := ValidateFont(missing, 16); err == nil {
		t.Error("expected an error for a missing font file")
	}

	garbage := filepath.Join(t.TempDir(), "garbage.ttf")
	if err := os.WriteFile(garbage, []byte("not a font"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFont(garbage, 16); err == nil {
		t.Error("expected an error for a corrupt font file")
	}
}

// color32 is a hashable RGBA color used to collect distinct colors seen
// across a set of frames.
type color32 struct{ r, g, b, a uint32 }

// distinctColors collects every distinct RGBA color across a set of frames.
func distinctColors(frames []image.Image) map[color32]bool {
	seen := map[color32]bool{}
	for _, f := range frames {
		b := f.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, a := f.At(x, y).RGBA()
				seen[color32{r, g, bl, a}] = true
			}
		}
	}
	return seen
}

// TestRenderTextFramesTTFThresholdsToTwoColors is the critical guard for the
// whole feature: a vector font's glyphs are anti-aliased at the rasterizer
// level, but drawTextThresholded must collapse that coverage to exactly the
// foreground or the background color. Letting antialiased grays through
// would triple frame size on the wire (see the comment on
// drawTextThresholded) and blur what should be crisp pixel text.
func TestRenderTextFramesTTFThresholdsToTwoColors(t *testing.T) {
	face, err := loadFace(testFontPath(t), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()

	fg := [3]uint8{255, 255, 255}
	bg := [3]uint8{0, 0, 0}
	frames := renderTextFrames("Hello, world", 32, face, fg, bg)
	if len(frames) == 0 {
		t.Fatal("no frames rendered")
	}

	colors := distinctColors(frames)
	if len(colors) != 2 {
		t.Errorf("got %d distinct colors across frames, want exactly 2 (fg+bg) — antialiased grays leaked through", len(colors))
	}
}

// TestRenderTextFramesLargeFontStillFitsBudget asserts the scroll-step
// widening logic (renderTextFrames) still caps frame count at
// maxAnimationFrames even when a much wider glyph advance (a large TTF
// point size, vs. the built-in font's fixed 7px advance) makes the text
// span far more pixels for the same message.
func TestRenderTextFramesLargeFontStillFitsBudget(t *testing.T) {
	face, err := loadFace(testFontPath(t), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()

	frames := renderTextFrames("THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG", 32, face, [3]uint8{255, 255, 255}, [3]uint8{0, 0, 0})
	if len(frames) > maxAnimationFrames {
		t.Errorf("got %d frames, want <= %d (device frame limit)", len(frames), maxAnimationFrames)
	}
	if len(frames) == 0 {
		t.Error("got 0 frames")
	}
}

// TestShowTextWithFontPathSendsAnimation is an integration check that a
// valid FontPath flows all the way through ShowText to a wire-level
// animation upload, the same shape TestShowTextSendsAnimation checks for
// the default font.
func TestShowTextWithFontPathSendsAnimation(t *testing.T) {
	d, ft := newFakeDevice()
	if err := d.ShowText("HI", TextOptions{FontPath: testFontPath(t), FontSize: 16}); err != nil {
		t.Fatal(err)
	}
	if ft.Len() == 0 {
		t.Fatal("nothing written to transport")
	}
	if got := ft.Bytes()[3]; got != 0x49 {
		t.Errorf("first command byte = %#x, want 0x49", got)
	}
}

// TestLoadFaceDefaultSize asserts a zero size falls back to defaultFontSize
// rather than producing a degenerate (zero-scale) face.
func TestLoadFaceDefaultSize(t *testing.T) {
	face, err := loadFace(testFontPath(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	if adv := font.MeasureString(face, "M"); adv <= 0 {
		t.Errorf("advance for size-0 (default) face = %v, want > 0", adv)
	}
}
