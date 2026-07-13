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

Routing through the daemon is not a small win: on real hardware a one-shot command takes **~4.8 s** dialing directly (open RFCOMM, ping, settle) versus **~13 ms** through a running daemon.

Two consequences of the device accepting only one RFCOMM channel at a time:

- Without a daemon running, leave several seconds between separate one-shot invocations — the device needs time to settle before it accepts another connection.
- While the daemon is running it holds that only connection, so `-direct` is refused rather than honored. Dialing a second channel alongside it wedges the device's Bluetooth stack until it is power-cycled.

Settings persist to `~/.config/go-divoom/config.json`, editable via `divoom use <mac>` or the JSON API's `PUT /api/config`.

## Sprite sheets

`scripts/sheet2gif.py` splits a horizontal strip of 32×32 cells into an animated GIF, so a sprite sheet can go straight to the display. Frame count comes from the sheet's width, so adding poses needs no flags. Requires Python with Pillow.

```bash
./scripts/sheet2gif.py mario.png mario.gif --fps 5
./bin/divoom send mario.gif
```

## Library

```go
import divoom "github.com/thedaneeffect/go-divoom"

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
- Importing this package in a darwin cgo build pins the main goroutine to
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

Divoom publishes no documentation for the Pixoo Max's Bluetooth protocol. Everything here rests on prior reverse-engineering work by others:

- **[hass-divoom](https://github.com/d03n3rfr1tz3/hass-divoom)** (d03n3rfr1tz3) — the most complete implementation of the Divoom Bluetooth binary protocol. It is the reference this project is validated against: `testdata/gen_goldens.py` runs hass-divoom's own encoder to generate the golden byte fixtures that every encoding test asserts, so the wire format is verified byte-for-byte rather than by eye.
- **[divoom-pixoo-max-nodejs](https://github.com/jakobwesthoff/divoom-pixoo-max-nodejs)** (Jakob Westhoff) — a clear reference for message framing, the rolling checksum, and the palette / bit-packed frame encoding.
- **[node-divoom-timebox-evo](https://github.com/RomRider/node-divoom-timebox-evo)** (RomRider) — documented the pixel-string encoding that the frame format builds on.
- **[Pixoo64-Advanced-Tools](https://github.com/tidyhf/Pixoo64-Advanced-Tools)** (tidyhf) — the Python toolset that prompted this rewrite. It targets the Pixoo 64, which speaks a *different* protocol (WiFi HTTP/JSON); discovering that the Pixoo Max is Bluetooth-only redirected the whole project.

The macOS transport is original work: `SOCKADDR_BTH` byte order was cross-checked against Microsoft's documentation and the [32feet.NET](https://github.com/inthehand/32feet) implementation, and the IOBluetooth main-queue callback behavior was established empirically against real hardware (see `docs/superpowers/specs/2026-07-11-darwin-iobluetooth-transport.md`).

## How this was built

This project was written by **Claude** (Anthropic) in [Claude Code](https://claude.com/claude-code), directed by a human who owned the hardware, made every product decision, and verified each change on the physical device.

- **Claude Fable 5** led the session: surveying the reference implementations, designing the architecture, coordinating the work, reviewing diffs, and doing the hardware debugging.
- **Claude Sonnet** and **Claude Haiku** ran as subagents for implementation and code review, one task at a time, each change gated by an independent review pass before it landed.

Work followed a spec → plan → test-driven-implementation → review loop; specs and plans are preserved under `docs/superpowers/`. The interesting parts of this codebase came from hardware disagreeing with the design, not from the plan:

- The device's protocol was validated against golden bytes generated from a reference implementation *before* any encoder code was written.
- The approved macOS design (a dedicated Bluetooth runloop thread) turned out to be impossible — modern IOBluetooth delivers all callbacks on the main dispatch queue — and was replaced only after minimal reproductions and `bluetoothd` logs proved it.
- Several bugs were only findable on real hardware: writes to `/dev/cu.*` that "succeed" into a dead link, RFCOMM channels torn down before the device consumed the command, and unthrottled frame pushes that wedge the device's Bluetooth stack. Each is now a barrier, a guard, or a documented limit.

Nothing in this repository was claimed to work without being run against a real Pixoo Max.
