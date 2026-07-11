package divoom

import (
	"bytes"
	"image"
	"image/color"
	"testing"
	"time"
)

func newFakeDevice() (*Device, *fakeTransport) {
	ft := &fakeTransport{}
	return NewDevice(ft, PixooMax), ft
}

// Goldens from testdata/gen_goldens.py; brightness also hardware-validated.
func TestDeviceSetBrightness(t *testing.T) {
	d, ft := newFakeDevice()
	if err := d.SetBrightness(10); err != nil {
		t.Fatal(err)
	}
	if got, want := ft.Bytes(), mustHex(t, "010400740a820002"); !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestDeviceSetBrightnessValidates(t *testing.T) {
	d, _ := newFakeDevice()
	if err := d.SetBrightness(101); err == nil {
		t.Error("expected error for brightness > 100")
	}
	if err := d.SetBrightness(-1); err == nil {
		t.Error("expected error for negative brightness")
	}
}

func TestDeviceSendImage(t *testing.T) {
	d, ft := newFakeDevice()
	if err := d.SendImage(fill16(color.RGBA{255, 0, 0, 255})); err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "01310044000a0a04aa2a0000000001ff00000000000000000000000000000000000000000000000000000000000000000000610202")
	if !bytes.Equal(ft.Bytes(), want) {
		t.Errorf("got  %x\nwant %x", ft.Bytes(), want)
	}
}

func TestDeviceShowLight(t *testing.T) {
	d, ft := newFakeDevice()
	if err := d.ShowLight([3]uint8{0xFF, 0xFF, 0xFF}, 100, true); err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "010d004501ffffff640101000000b60302")
	if !bytes.Equal(ft.Bytes(), want) {
		t.Errorf("got  %x\nwant %x", ft.Bytes(), want)
	}
}

func TestDeviceShowClock(t *testing.T) {
	d, ft := newFakeDevice()
	err := d.ShowClock(ClockOptions{Style: 2, TwentyFour: true})
	if err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "010a004500010201000000530002")
	if !bytes.Equal(ft.Bytes(), want) {
		t.Errorf("got  %x\nwant %x", ft.Bytes(), want)
	}
}

func TestDeviceSetDateTime(t *testing.T) {
	d, ft := newFakeDevice()
	ts := time.Date(2026, 7, 11, 15, 30, 45, 0, time.UTC)
	if err := d.SetDateTime(ts); err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "010a00181a14070b0f1e2dbc0002")
	if !bytes.Equal(ft.Bytes(), want) {
		t.Errorf("got  %x\nwant %x", ft.Bytes(), want)
	}
}

func TestDeviceSendAnimationWritesAllChunks(t *testing.T) {
	d, ft := newFakeDevice()
	frames := []image.Image{
		fill16(color.RGBA{255, 0, 0, 255}),
		fill16(color.RGBA{0, 0, 0, 255}),
	}
	if err := d.SendAnimation(frames, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "015d0049540000000000aa2a00f4010001ff00000000000000000000000000000000000000000000000000000000000000000000aa2a00f401000100000000000000000000000000000000000000000000000000000000000000000000008d0502")
	if !bytes.Equal(ft.Bytes(), want) {
		t.Errorf("got  %x\nwant %x", ft.Bytes(), want)
	}
}
