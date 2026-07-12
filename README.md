# go-divoom

Go library, server, and web panel for controlling Divoom pixel displays over Bluetooth Classic. Primary target: **Divoom Pixoo Max** (32×32). Single binary: REST API + embedded Svelte control panel + one-shot CLI.

Reimplementation of the reverse-engineered Divoom binary protocol, validated byte-for-byte against [hass-divoom](https://github.com/d03n3rfr1tz3/hass-divoom) and real hardware. Inspired by [Pixoo64-Advanced-Tools](https://github.com/tidyhf/Pixoo64-Advanced-Tools) and [divoom-pixoo-max-nodejs](https://github.com/jakobwesthoff/divoom-pixoo-max-nodejs).

## Features

- Static images (PNG/JPEG/GIF) and animated GIFs, auto-resized to 16×16 or 32×32
- Scrolling text, brightness, solid-color light, clock faces, screen on/off, time sync
- Web panel (embedded, no separate deploy) and JSON REST API
- Transports: native RFCOMM everywhere — IOBluetooth on macOS (cgo), RFCOMM sockets on Linux and Windows (pure Go) — plus macOS Bluetooth serial (`/dev/cu.*`) as a fallback

## Build

Requires [mise](https://mise.jdx.dev) (Go 1.24 + bun are pinned in `.mise.toml`):

```bash
mise install
mise run build     # web build → embed → bin/divoom
mise run test      # go test ./...
```

## Pairing

- **macOS**: pair the device (System Settings → Bluetooth, or `blueutil --pair <mac>`). A serial port appears as `/dev/cu.<device-name>`.
- **Linux**: `bluetoothctl pair <mac>`, then use `-mac` (native RFCOMM, channel 1).
- **Windows**: pair in Settings, then use `-mac`.

Find the device MAC with `blueutil --inquiry` (macOS) or `bluetoothctl scan on` (Linux).

## Usage

Server + web panel (recommended, especially on macOS — see quirks):

```bash
./bin/divoom serve            # http://localhost:8377
```

One-shot CLI:

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

Settings persist to `~/.config/go-divoom/config.json` (server) and are editable in the web panel.

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
