# go-divoom v1 Design

Date: 2026-07-11
Status: Approved (rev 2 — retargeted from Pixoo 64 to Pixoo Max, renamed go-pixoo64 → go-divoom)

## Purpose

Go rewrite of Divoom device tooling, inspired by [Pixoo64-Advanced-Tools](https://github.com/tidyhf/Pixoo64-Advanced-Tools) (Python/customtkinter). Primary target is the **Divoom Pixoo Max (32×32)** — the only device on hand — which uses the Bluetooth Classic binary protocol, not the Pixoo 64's WiFi HTTP API. v1 delivers: a reusable Go protocol/device library, a single-binary server exposing a REST API, and a minimal embedded Svelte control panel.

## Device Landscape (survey findings)

| | Pixoo 64 | Pixoo Max (32×32) |
|---|---|---|
| Transport | WiFi, local HTTP `http://<ip>/post` | Bluetooth Classic SPP (RFCOMM, port 1); no HTTP API |
| Encoding | JSON commands, base64 raw RGB | Binary framing: `0x01` start, u16LE length, command byte, rolling u16LE checksum, `0x02` end |
| Pixels | Full RGB, 64×64 | Palette-indexed (≤1024 colors), bit-packed indices, 200-byte write chunks |

The binary protocol is shared across the Divoom Bluetooth family (Pixoo, Pixoo Max, Timebox, Ditoo, Tivoo, …) with per-device capability differences — hence the project name **go-divoom** with per-device backends.

Protocol references (cloned to /tmp during survey):
- [hass-divoom](https://github.com/d03n3rfr1tz3/hass-divoom) — complete Python implementation (`devices/divoom.py`), Pixoo Max subclass, ESP32 TCP proxy support
- [divoom-pixoo-max-nodejs](https://github.com/jakobwesthoff/divoom-pixoo-max-nodejs) — clean reference for framing, checksum, and palette/bit-packed frame encoding

## Architecture Decision

Hybrid Go + Svelte, single binary:

- Browsers cannot speak Bluetooth Classic (WebBluetooth is BLE-only), so the Go server must own the device transport regardless of UI choice.
- Future media features (screen capture, webcam, audio visualizer, pixel designer) map to native browser APIs and would be costly cgo work in Go.
- Ships as one binary: Svelte build embedded via `go:embed`. Cross-compiles per platform.

## Transport Strategy

Bluetooth Classic RFCOMM behind a small `Transport` interface (`io.ReadWriteCloser` + connect semantics):

1. **macOS (dev machine)** — serial port: pairing the Pixoo Max creates `/dev/cu.<device-name>`; the transport is plain file I/O. Pure Go, no cgo. **Validated on hardware 2026-07-11**: paired `Pixoo-Max` via blueutil, wrote brightness frames (`0x01 0x04 0x00 0x74 <val> <cksum u16LE> 0x02`) to `/dev/cu.Pixoo-Max`, device responded (brightness changed, BT icon shown). Framing + checksum confirmed correct.
2. **Linux** — native RFCOMM sockets via `golang.org/x/sys/unix` (`SockaddrRFCOMM`). Pure Go, no cgo.
3. **Windows** — native RFCOMM via Winsock `AF_BTH`, `golang.org/x/sys/windows` (`SockaddrBth`, verified present in x/sys v0.47.0). Pure Go. This is the answer for Windows machines; WSL2 is explicitly NOT supported (no Bluetooth stack in the WSL2 kernel) — run the native Windows binary instead.
4. **TCP proxy** — hass-divoom-compatible ESP32 Bluetooth proxy (TCP port 7777; on connect, sends device MAC bytes + port). Pure network code, works from any OS. Deferred unless needed.

Serial transport is primary for v1 development (macOS); Linux/Windows RFCOMM sockets for deployment.

## Repository Layout

```
go-divoom/
├── pkg/divoom/            # protocol core (zero server deps, importable standalone)
│   ├── frame.go           # message framing: start/len/payload/checksum/end, payload escaping
│   ├── encode.go          # palette extraction, bit-packed pixel encoding, 200-byte chunking
│   ├── transport.go       # Transport interface
│   ├── transport_serial.go  # serial port (/dev/cu.* on macOS) — primary dev transport
│   ├── transport_linux.go # RFCOMM via x/sys/unix
│   ├── transport_windows.go # RFCOMM via x/sys/windows
│   └── device.go          # Device interface: capabilities, screen size
├── pkg/divoom/pixoomax/   # Pixoo Max backend (screensize 32, chunksize 200, capability set)
├── cmd/divoom/            # single binary
│   ├── main.go            # HTTP server + embedded UI; flag-driven one-shot CLI mode
│   └── api.go             # REST: /api/device/* ; device MAC/proxy config persisted
├── web/                   # Svelte (Vite, bun) — dist/ embedded via go:embed
└── docs/superpowers/specs/
```

## Components

### pkg/divoom (protocol core)

- Message framing per reverse-engineered protocol: `0x01` start, u16LE length (payload+2), payload, rolling u16LE checksum over length+payload, `0x02` end. Optional payload escaping (device-dependent; Pixoo Max: off).
- Frame encoding: build color palette from `image.Image` (error if >1024 colors after quantization), bit-packed palette indices (minimal bits, little-endian bit order), per divoom-pixoo-max-nodejs reference.
- Writes chunked to device chunk size (200 bytes for Max).
- `Device` interface: `SendImage(image.Image)`, `SendAnimation([]image.Image, frameTime)`, `SetBrightness(int)`, `ShowClock(opts)`, `ShowLight(color, brightness)`, `ShowText(...)`, `SetScreen(on bool)` — capability-gated per device.
- Errors: typed, wrapped transport failures, input validation (image dimensions must match device screen size after resize; brightness 0–100). No panics.

### pkg/divoom/pixoomax

- Screen 32×32, chunk 200, no payload escaping.
- Unsupported ops (equalizer, radio, lyrics, volume, keyboard, playstate) return typed `ErrUnsupported`.

### cmd/divoom

- Default mode: HTTP server (configurable port) serving `/api/*` and embedded UI.
- REST endpoints map onto Device interface (e.g. `POST /api/device/image`, `POST /api/device/text`, `POST /api/device/brightness`, `POST /api/device/clock`, `GET /api/device/status`).
- Config: device MAC / serial path, transport choice (serial | rfcomm | proxy host), persisted to `~/.config/go-divoom/config.json`.
- One-shot CLI mode for scripting (e.g. `divoom --mac AA:BB:.. send image.png`).
- Connection management: single serialized connection to device; reconnect on failure with backoff.

### web (Svelte)

v1 panel:
- Connect: MAC/transport entry, saved server-side, connection status.
- Image/GIF upload with 32×32 preview (canvas downscale client-side; server re-validates and encodes).
- Text sender, brightness slider, screen on/off, clock face selector, solid-color light mode.

## Data Flow

UI → REST (`/api/device/*`) → pkg/divoom Device → Transport (RFCOMM/TCP) → device. CLI mode calls pkg/divoom directly. No WebSocket in v1; Bluetooth throughput limits streaming anyway — revisit with capture features.

## Error Handling

- pkg/divoom: typed errors (`ErrUnsupported`, `ErrChecksum`, transport errors wrapped), input validation at API boundary.
- API layer: validation → 400, unsupported op → 422, device unreachable/disconnected → 502 with reconnect attempt.
- UI: surfaces API error messages inline; connection state visible.

## Testing

- Framing/encoding: golden tests against byte sequences captured from reference implementations (hass-divoom `.txt` protocol dumps, divoom-pixoo-max-nodejs output) — exact bytes for known images, checksum, bit-packing edge cases (palette sizes 1, 2, 3, 256, 1024).
- Transport: fake in-memory Transport for Device-level tests; TCP proxy backend tested against local TCP fixture.
- API handlers: httptest against fake Transport.
- Hardware smoke test: manual checklist against the real Pixoo Max early in implementation (framing assumptions must be validated on-device before building higher layers).

## Tooling

- mise tasks: `mise run dev`, `mise run build` (web build → go:embed → go build), `mise run test`, `mise run check`.
- bun for web package management.
- Cross-compile targets: `linux/arm64`, `linux/amd64`, `windows/amd64`.

## Out of Scope (later phases)

1. Playlists and schedulers (RSS, calendar, rotations) — Go task engine.
2. Live capture: screen region streaming, webcam, audio visualizer — browser capture + WebSocket push (throughput-limited over BT; needs on-device testing).
3. Divoom Cloud gallery: login, browse, PixelBean decode (AES + LZO).
4. Pixoo 64 HTTP backend (`pkg/divoom/pixoo64`) — slots into Device interface if hardware materializes.
5. Other BT family devices (Ditoo, Tivoo, Timebox) — thin subclass-style backends.
6. macOS IOBluetooth/cgo transport — unnecessary; serial port path covers macOS.
7. ESP32 TCP proxy backend — deferred unless a deployment needs it.
