package divoom

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/color"
	"io"
	"strings"
	"testing"
	"time"
)

func newFakeDevice() (*Device, *fakeTransport) {
	ft := &fakeTransport{}
	return NewDevice(ft, PixooMax), ft
}

// pingResponseHex is a real "get view" reply captured from a Pixoo Max over
// Bluetooth serial: 01 03 00 46 49 00 02 in, this out. Used to simulate a
// live link answering Device.Ping.
const pingResponseHex = "011b00044655000001ff5000640001026400ffffff000100000024150c0602"

// pingTransport embeds fakeTransport (so writes are still recorded exactly
// as fakeTransport users expect) but answers Read with the canned hardware
// response once respondOnWrite writes have landed. respondOnWrite <= 1
// models a link that is up immediately; > 1 models the cold-link behavior
// seen on hardware, where writes issued while the RFCOMM channel is still
// establishing are swallowed and only a later retry gets a reply.
type pingTransport struct {
	fakeTransport
	respondOnWrite int // respond once this many writes have occurred; 0 means 1
	writes         int
	responded      bool
}

func (p *pingTransport) Write(b []byte) (int, error) {
	p.writes++
	return p.fakeTransport.Write(b)
}

func (p *pingTransport) Read(b []byte) (int, error) {
	threshold := max(p.respondOnWrite, 1)
	if p.responded || p.writes < threshold {
		return 0, io.EOF
	}
	p.responded = true
	resp := mustHexBytes(pingResponseHex)
	return copy(b, resp), nil
}

// silentTransport accepts writes like fakeTransport but never yields a
// response, modeling the hardware defect: /dev/cu.* opens and Write reports
// success even though the Bluetooth RFCOMM link never came up, so commands
// vanish and no reply ever arrives.
type silentTransport struct{ fakeTransport }

func (s *silentTransport) Read([]byte) (int, error) { return 0, io.EOF }

func mustHexBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err) // fixture is a constant; a decode failure is a test bug
	}
	return b
}

func TestDevicePingSuccess(t *testing.T) {
	pt := &pingTransport{}
	d := NewDevice(pt, PixooMax)
	if err := d.Ping(); err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "01030046490002")
	if got := pt.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

// Hardware-observed: the first write after opening /dev/cu.* is swallowed
// while the RFCOMM channel is still establishing; a later write gets a
// reply. Ping must retry rather than give up after one roundtrip.
func TestDevicePingRetriesColdLink(t *testing.T) {
	pt := &pingTransport{respondOnWrite: 2}
	d := NewDevice(pt, PixooMax)
	if err := d.Ping(); err != nil {
		t.Fatal(err)
	}
	ping := mustHex(t, "01030046490002")
	want := append(append([]byte{}, ping...), ping...) // two attempts written
	if got := pt.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestDevicePingNoResponse(t *testing.T) {
	st := &silentTransport{}
	d := NewDevice(st, PixooMax)
	err := d.Ping()
	if err == nil {
		t.Fatal("expected error when device never responds")
	}
	if !strings.Contains(err.Error(), "no response") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "no response")
	}
	ping := mustHex(t, "01030046490002")
	want := bytes.Repeat(ping, 3) // all three attempts written before giving up
	if got := st.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x (3 ping attempts)", got, want)
	}
}

// Flush shares Ping's roundtrip helper but is called after a command write,
// right before a one-shot CLI command closes the transport, to prove the
// device drained the write instead of it vanishing when the link tears down.
func TestDeviceFlushSuccess(t *testing.T) {
	pt := &pingTransport{}
	d := NewDevice(pt, PixooMax)
	if err := d.SetBrightness(50); err != nil {
		t.Fatal(err)
	}
	prior := append([]byte{}, pt.Bytes()...) // command bytes written before Flush

	if err := d.Flush(); err != nil {
		t.Fatal(err)
	}

	got := pt.Bytes()
	if !bytes.HasPrefix(got, prior) {
		t.Fatalf("Flush must not alter bytes already written: got %x, want prefix %x", got, prior)
	}
	getView := mustHex(t, "01030046490002")
	if after := got[len(prior):]; !bytes.Equal(after, getView) {
		t.Errorf("Flush should write a get-view request after the prior command: got %x, want %x", after, getView)
	}
}

func TestDeviceFlushNoResponse(t *testing.T) {
	st := &silentTransport{}
	d := NewDevice(st, PixooMax)
	if err := d.SetBrightness(50); err != nil {
		t.Fatal(err)
	}

	err := d.Flush()
	if err == nil {
		t.Fatal("expected error when device never responds")
	}
	if !strings.Contains(err.Error(), "no response") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "no response")
	}
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
