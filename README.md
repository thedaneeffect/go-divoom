# go-divoom

Go library, headless daemon, and CLI for controlling Divoom pixel displays over Bluetooth Classic. Primary target: **Divoom Pixoo Max** (32×32). Single binary: one-shot CLI + JSON REST API daemon.

Reimplementation of the reverse-engineered Divoom binary protocol, validated byte-for-byte against [hass-divoom](https://github.com/d03n3rfr1tz3/hass-divoom) and real hardware. Inspired by [Pixoo64-Advanced-Tools](https://github.com/tidyhf/Pixoo64-Advanced-Tools) and [divoom-pixoo-max-nodejs](https://github.com/jakobwesthoff/divoom-pixoo-max-nodejs).

## Features

- Static images (PNG/JPEG/GIF) and animated GIFs, auto-resized to 16×16 or 32×32
- Scrolling text, brightness, solid-color light, clock faces, screen on/off, time sync
- Headless daemon (`divoom serve`) exposing a JSON REST API over a single persistent Bluetooth connection; one-shot CLI commands route through it automatically when it's running, so they complete instantly instead of paying a fresh dial/ping cost
- Transports: native RFCOMM everywhere — IOBluetooth on macOS (cgo), RFCOMM sockets on Linux and Windows (pure Go) — plus macOS Bluetooth serial (`/dev/cu.*`) as a fallback

## Build

Requires [mise](https://mise.jdx.dev) (Go 1.24 is pinned in `.mise.toml`):

```bash
mise install
mise run build     # go build -o bin/divoom ./cmd/divoom
mise run test      # go test ./...
```

## Pairing

- **macOS**: pair the device (System Settings → Bluetooth, or `blueutil --pair <mac>`). A serial port appears as `/dev/cu.<device-name>`.
- **Linux**: `bluetoothctl pair <mac>`, then use `-mac` (native RFCOMM, channel 1).
- **Windows**: pair in Settings, then use `-mac`.

Find the device MAC with `blueutil --inquiry` (macOS) or `bluetoothctl scan on` (Linux).

## Usage

Daemon (recommended, especially on macOS — see quirks): holds one persistent connection, and every one-shot command below routes through it automatically once it's running, making them near-instant.

```bash
./bin/divoom serve &           # JSON API on :8377
./bin/divoom brightness 40     # routed through the daemon: instant
```

One-shot CLI (dials directly when no daemon is running; pass `-direct` to force a direct dial even when one is):

```bash
./bin/divoom -mac AA:BB:CC:DD:EE:FF brightness 40
./bin/divoom -mac AA:BB:CC:DD:EE:FF send image.png    # or animated .gif
./bin/divoom -mac AA:BB:CC:DD:EE:FF text 'HELLO'
./bin/divoom -mac AA:BB:CC:DD:EE:FF light '#ff8800' 80
./bin/divoom -mac AA:BB:CC:DD:EE:FF clock 1
./bin/divoom -mac AA:BB:CC:DD:EE:FF on|off
./bin/divoom config           # print config path + contents
# macOS serial fallback: replace -mac ... with -serial /dev/cu.Pixoo-Max
```

Without a daemon running, leave a few seconds between separate one-shot invocations — the device needs time to settle before it accepts another connection.

Settings persist to `~/.config/go-divoom/config.json`, editable via `divoom use <mac>` or the JSON API's `PUT /api/config`.

## Library

```go
import "github.com/thedaneeffect/go-divoom/pkg/divoom"

func main() {
	divoom.RunEventLoop(func() { // see "macOS RFCOMM contract" below
		t, err := divoom.DialRFCOMM(mac, 1) // or divoom.DialSerial("/dev/cu.Pixoo-Max")
		d := divoom.NewDevice(t, divoom.PixooMax)
		defer d.Close()
		if err := d.Ping(); err != nil { /* link not established */ }
		d.SendImage(img)                       // image.Image, auto-resized
		d.ShowText("HELLO", divoom.TextOptions{})
		d.SetBrightness(80)
	})
}
```

### macOS RFCOMM contract

Modern IOBluetooth delivers all RFCOMM events through the process **main
dispatch queue**, so on macOS `DialRFCOMM` only works while
`divoom.RunEventLoop` is servicing the main event loop:

- Wrap your program's work in `divoom.RunEventLoop(func() { ... })` from
  `main`. On every other platform (and in `CGO_ENABLED=0` macOS builds)
  `RunEventLoop` is a plain pass-through that just calls the function, so
  the same code stays portable. Sequential `RunEventLoop` calls are
  supported.
- Importing `pkg/divoom` in a darwin cgo build pins the main goroutine to
  the main OS thread (`runtime.LockOSThread` in an `init`); this is what
  guarantees `RunEventLoop` runs on the thread macOS requires.
- If a process is killed mid-connection, macOS can leave the baseband (ACL)
  link up, which makes *every* subsequent RFCOMM connect fail until it is
  cleared: `blueutil --disconnect <mac>` recovers. `Close` always drops the
  link, so normal use never hits this.

## macOS Bluetooth quirks (Pixoo Max)

- The serial (`/dev/cu.*`) transport survives roughly one open per pairing: closing the port drops the Bluetooth connection and re-opening it often gets a dead channel that only re-pairing recovers (`blueutil --unpair <mac> && blueutil --pair <mac> && blueutil --connect <mac>`). **Prefer `-mac` (IOBluetooth RFCOMM)** — it reconnects indefinitely.
- Every dial performs a ping (request/response) so a dead link fails loudly instead of silently dropping commands.
- Animation and text uploads take a few seconds over Bluetooth; commands queue in order.

See `docs/superpowers/specs/hardware-smoke.md` for the full validation log.

## Credits

Protocol reverse-engineering by the [hass-divoom](https://github.com/d03n3rfr1tz3/hass-divoom) and [divoom-pixoo-max-nodejs](https://github.com/jakobwesthoff/divoom-pixoo-max-nodejs) projects. Golden test fixtures are generated from hass-divoom's reference implementation (`pkg/divoom/testdata/gen_goldens.py`).
