package divoom

import (
	"fmt"
	"image"
	"sync"
	"time"
)

// Device is a connected Divoom device. Methods are safe for concurrent use;
// writes are serialized because the protocol is stateful per connection.
type Device struct {
	mu sync.Mutex
	t  Transport
	p  Profile
}

// NewDevice wraps an open transport with a device profile.
func NewDevice(t Transport, p Profile) *Device {
	return &Device{t: t, p: p}
}

// Close closes the underlying transport.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.t.Close()
}

// roundtripTimeout bounds how long each roundtrip attempt waits for a reply.
const roundtripTimeout = 2 * time.Second

// roundtripAttempts is how many write+read roundtrips a barrier (Ping or
// Flush) makes before giving up. Hardware-observed: the first write after
// opening /dev/cu.* can be swallowed while the RFCOMM channel is still
// establishing, so a single roundtrip reports a dead link on a device that
// is merely slow to come up.
const roundtripAttempts = 3

// deadlineSetter is implemented by transports that support read deadlines
// (e.g. *os.File). roundtrip uses it when available to bound the wait for a
// reply instead of risking an indefinite block.
type deadlineSetter interface {
	SetReadDeadline(time.Time) error
}

// Ping sends a lightweight status query ("get view") and blocks until the
// device replies, retrying up to roundtripAttempts times with a fresh write
// and a roundtripTimeout read deadline per attempt.
//
// This exists because opening a macOS Bluetooth serial port
// (/dev/cu.Pixoo-Max-*) succeeds, and writes to it report success, even when
// the underlying RFCOMM link never actually came up: commands vanish
// silently with no error anywhere in the stack. The device does reply to
// commands, so a full write/read roundtrip is the only reliable way to
// confirm the link is live and flush past whatever buffering hides the
// failure. Callers should invoke Ping immediately after dialing, before
// trusting the connection for anything else.
func (d *Device) Ping() error {
	return d.roundtrip()
}

// Flush blocks until a write/read roundtrip proves the device has drained
// every command written so far on this connection.
//
// This exists because one-shot CLI invocations write a command and then
// immediately close the transport: on hardware the RFCOMM channel tears
// down before the device finishes consuming the command out of its input
// buffer, so the command silently vanishes even though Write reported
// success — the exact same defect Ping guards against at dial time, just at
// the other end of the connection's life. RFCOMM delivers bytes reliably
// and in order, so a "get view" request written after a command cannot be
// answered until the device has processed everything ahead of it in the
// stream; receiving that reply is proof the command already landed.
//
// Callers that keep the connection open across multiple commands (e.g. the
// HTTP server) don't need this — only code about to close the transport
// right after writing does.
func (d *Device) Flush() error {
	return d.roundtrip()
}

// roundtrip performs the shared write("get view")+read barrier used by both
// Ping and Flush, retrying up to roundtripAttempts times.
func (d *Device) roundtrip() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var lastErr error
	for i := 0; i < roundtripAttempts; i++ {
		lastErr = d.roundtripOnce()
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

// roundtripOnce performs a single write+read roundtrip. Caller must hold d.mu.
func (d *Device) roundtripOnce() error {
	if _, err := d.t.Write(makeCommand(0x46, nil)); err != nil {
		return fmt.Errorf("divoom: write: %w", err)
	}

	if dl, ok := d.t.(deadlineSetter); ok {
		if err := dl.SetReadDeadline(time.Now().Add(roundtripTimeout)); err != nil {
			return fmt.Errorf("divoom: set read deadline: %w", err)
		}
		defer dl.SetReadDeadline(time.Time{})
	}

	var acc []byte
	buf := make([]byte, 64)
	for {
		n, err := d.t.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			if isCompleteFrame(acc) {
				return nil
			}
		}
		if err != nil {
			return fmt.Errorf("divoom: no response from device (link not established?): %w", err)
		}
	}
}

// isCompleteFrame reports whether b holds at least one full wire frame:
// terminated by 0x02 with a nonempty payload.
func isCompleteFrame(b []byte) bool {
	return len(b) >= 3 && b[len(b)-1] == 0x02
}

func (d *Device) send(msgs ...[]byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, m := range msgs {
		if _, err := d.t.Write(m); err != nil {
			return fmt.Errorf("divoom: write: %w", err)
		}
	}
	return nil
}

// SetBrightness sets display brightness in percent (0-100).
func (d *Device) SetBrightness(pct int) error {
	if pct < 0 || pct > 100 {
		return fmt.Errorf("divoom: brightness %d out of range 0-100", pct)
	}
	return d.send(makeCommand(0x74, []byte{byte(pct)}))
}

// SendImage displays a static image, resized to the device resolution.
func (d *Device) SendImage(img image.Image) error {
	msgs, err := d.p.imageMessages(img)
	if err != nil {
		return err
	}
	return d.send(msgs...)
}

// SendAnimation displays an animation with the given per-frame duration.
func (d *Device) SendAnimation(frames []image.Image, frameTime time.Duration) error {
	msgs, err := d.p.animationMessages(frames, frameTime)
	if err != nil {
		return err
	}
	return d.send(msgs...)
}

// ClockOptions configures the clock channel.
type ClockOptions struct {
	Style      int // 0-15
	TwentyFour bool
	Weather    bool
	Temp       bool
	Calendar   bool
	Color      *[3]uint8 // optional
}

// ShowClock switches the device to the clock channel.
func (d *Device) ShowClock(o ClockOptions) error {
	if o.Style < 0 || o.Style > 15 {
		return fmt.Errorf("divoom: clock style %d out of range 0-15", o.Style)
	}
	args := []byte{0x00, b2b(o.TwentyFour), byte(o.Style), 0x01, b2b(o.Weather), b2b(o.Temp), b2b(o.Calendar)}
	if o.Color != nil {
		args = append(args, o.Color[0], o.Color[1], o.Color[2])
	}
	return d.send(makeCommand(0x45, args))
}

// ShowLight switches the device to solid-color light mode. on=false turns
// the display off.
func (d *Device) ShowLight(rgb [3]uint8, brightness int, on bool) error {
	if brightness < 0 || brightness > 100 {
		return fmt.Errorf("divoom: brightness %d out of range 0-100", brightness)
	}
	white := byte(0x00)
	if rgb == [3]uint8{0xFF, 0xFF, 0xFF} {
		white = 0x01
	}
	args := []byte{0x01, rgb[0], rgb[1], rgb[2], byte(brightness), white, b2b(on), 0x00, 0x00, 0x00}
	return d.send(makeCommand(0x45, args))
}

// ScreenOn wakes the display (reference implementation semantics).
func (d *Device) ScreenOn() error { return d.ShowLight([3]uint8{1, 1, 1}, 100, true) }

// ScreenOff turns the display off.
func (d *Device) ScreenOff() error { return d.ShowLight([3]uint8{1, 1, 1}, 100, false) }

// SetDateTime syncs the device clock.
func (d *Device) SetDateTime(ts time.Time) error {
	args := []byte{
		byte(ts.Year() % 100), byte(ts.Year() / 100),
		byte(ts.Month()), byte(ts.Day()),
		byte(ts.Hour()), byte(ts.Minute()), byte(ts.Second()),
	}
	return d.send(makeCommand(0x18, args))
}

func b2b(v bool) byte {
	if v {
		return 1
	}
	return 0
}
