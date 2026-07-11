# go-divoom

Go library, server, and web panel for controlling Divoom pixel displays over Bluetooth Classic. Primary target: **Divoom Pixoo Max** (32×32). Single binary: REST API + embedded Svelte control panel + one-shot CLI.

Reimplementation of the reverse-engineered Divoom binary protocol, validated byte-for-byte against [hass-divoom](https://github.com/d03n3rfr1tz3/hass-divoom) and real hardware. Inspired by [Pixoo64-Advanced-Tools](https://github.com/tidyhf/Pixoo64-Advanced-Tools) and [divoom-pixoo-max-nodejs](https://github.com/jakobwesthoff/divoom-pixoo-max-nodejs).

## Features

- Static images (PNG/JPEG/GIF) and animated GIFs, auto-resized to 16×16 or 32×32
- Scrolling text, brightness, solid-color light, clock faces, screen on/off, time sync
- Web panel (embedded, no separate deploy) and JSON REST API
- Transports: macOS Bluetooth serial (`/dev/cu.*`), native RFCOMM sockets on Linux and Windows — all pure Go, no cgo

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
./bin/divoom -serial /dev/cu.Pixoo-Max brightness 40
./bin/divoom -serial /dev/cu.Pixoo-Max send image.png    # or animated .gif
./bin/divoom -serial /dev/cu.Pixoo-Max text 'HELLO'
./bin/divoom -serial /dev/cu.Pixoo-Max light '#ff8800' 80
./bin/divoom -serial /dev/cu.Pixoo-Max clock 1
./bin/divoom -serial /dev/cu.Pixoo-Max on|off
./bin/divoom config           # print config path + contents
# Linux/Windows: replace -serial with -mac AA:BB:CC:DD:EE:FF
```

Settings persist to `~/.config/go-divoom/config.json` (server) and are editable in the web panel.

## Library

```go
import "github.com/thedaneeffect/go-divoom/pkg/divoom"

t, err := divoom.DialSerial("/dev/cu.Pixoo-Max") // or divoom.DialRFCOMM(mac, 1)
d := divoom.NewDevice(t, divoom.PixooMax)
defer d.Close()
if err := d.Ping(); err != nil { /* link not established */ }
d.SendImage(img)                       // image.Image, auto-resized
d.ShowText("HELLO", divoom.TextOptions{})
d.SetBrightness(80)
```

## macOS Bluetooth quirks (Pixoo Max)

- Closing the serial port drops the Bluetooth connection, and re-opening it often gets a dead channel; only re-pairing reliably recovers (`blueutil --unpair <mac> && blueutil --pair <mac> && blueutil --connect <mac>`). **Prefer server mode** — it holds one connection open.
- Every dial performs a ping (request/response) so a dead link fails loudly instead of silently dropping commands.
- Animation and text uploads take a few seconds over Bluetooth; commands queue in order.

See `docs/superpowers/specs/hardware-smoke.md` for the full validation log.

## Credits

Protocol reverse-engineering by the [hass-divoom](https://github.com/d03n3rfr1tz3/hass-divoom) and [divoom-pixoo-max-nodejs](https://github.com/jakobwesthoff/divoom-pixoo-max-nodejs) projects. Golden test fixtures are generated from hass-divoom's reference implementation (`pkg/divoom/testdata/gen_goldens.py`).
