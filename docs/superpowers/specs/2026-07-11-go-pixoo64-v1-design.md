# go-pixoo64 v1 Design

Date: 2026-07-11
Status: Approved

## Purpose

Rewrite of [Pixoo64-Advanced-Tools](https://github.com/tidyhf/Pixoo64-Advanced-Tools) (Python/customtkinter, single 3,972-line file) as a maintainable Go project. v1 delivers core device control only: a reusable Go client library for the Divoom Pixoo 64 local HTTP API, a single-binary server exposing a REST API, and a minimal embedded Svelte control panel.

## Architecture Decision

Hybrid Go + Svelte, chosen over pure Go CLI and pure Svelte SPA:

- The device protocol (JSON POST to `http://<device-ip>/post`) and long-running tasks belong server-side in Go.
- Future media features (screen capture, webcam, audio visualizer, pixel designer) map to native browser APIs (`getDisplayMedia`, `getUserMedia`, WebAudio, canvas) and would be costly cgo work in Go (opencv, audio loopback capture).
- A browser UI cannot talk to the device or Divoom cloud directly (CORS), so the Go server doubles as proxy.
- Ships as one binary: Svelte build embedded via `go:embed`.

## Repository Layout

```
go-pixoo64/
├── pkg/pixoo/          # device client library (zero server deps, importable standalone)
│   ├── client.go       # Client{addr, http.Client}, request plumbing, error_code handling
│   ├── draw.go         # SendImage, SendAnimation (multi-frame), SendText, ClearText, pic-ID counter
│   ├── channel.go      # Brightness, Screen on/off, Clock, EqPosition, CustomPage, GetAllConf
│   └── device.go       # SetUTC, 24h mode, rotation, temperature mode, timer
├── cmd/pixoo64/        # single binary
│   ├── main.go         # serves HTTP API + embedded UI; also flag-driven one-shot CLI mode
│   └── api.go          # REST: /api/device/* proxying to pkg/pixoo; device IP persisted to config
├── web/                # Svelte (Vite, bun) — dist/ embedded via go:embed
└── docs/superpowers/specs/
```

## Device Protocol (from survey)

All commands are JSON POST to `http://<ip>/post`; responses carry `error_code` (0 = success). Commands in v1 scope:

- `Draw/SendHttpGif` — 64×64 frames as base64 raw RGB; `PicNum`, `PicOffset`, `PicID`, `PicSpeed`
- `Draw/GetHttpGifId`, `Draw/ResetHttpGifId` — pic-ID counter management
- `Draw/SendHttpText`, `Draw/ClearHttpText` — scrolling text (position, font, color, speed, align); text IDs cycle 1–19
- `Channel/SetBrightness`, `Channel/OnOffScreen`, `Channel/SetClockSelectId`, `Channel/SetEqPosition`, `Channel/SetCustomPageIndex`, `Channel/GetAllConf`
- `Device/SetUTC`, `Device/SetTime24Flag`, `Device/SetScreenRotationAngle`, `Device/SetDisTempMode`, `Device/GetDeviceTime`
- `Tools/SetTimer`

Device quirk preserved from the reference `pixoo1664` implementation: the animation pic-ID counter must be loaded on connect (`Draw/GetHttpGifId`) and reset remotely once it exceeds 32, otherwise frame uploads fail.

## Components

### pkg/pixoo

- `Client` created with device address; configurable timeout via `http.Client`.
- Image input is `image.Image`; the library resizes/validates to 64×64 and encodes to base64 raw RGB.
- `SendAnimation([]image.Image, speed)` batches frames into one `Draw/CommandList` request.
- Errors: nonzero `error_code` returns typed `PixooError{Code int}`; transport errors wrapped with context. No panics or asserts.
- Pic-ID counter semantics: increment per send, remote reset at limit 32.

### cmd/pixoo64

- Default mode: HTTP server (configurable port) serving `/api/*` and the embedded UI.
- REST endpoints map 1:1 onto pkg/pixoo methods (e.g. `POST /api/device/image`, `POST /api/device/text`, `POST /api/device/brightness`, `GET /api/device/config`).
- Device IP set via UI or flag, persisted to a config file (`~/.config/go-pixoo64/config.json`).
- Flag-driven one-shot CLI mode for scripting (e.g. `pixoo64 --ip x.x.x.x send image.png`).

### web (Svelte)

v1 panel:
- Connect: device IP entry, saved server-side, connection status.
- Image/GIF upload with 64×64 preview (canvas downscale client-side; GIF frames decoded client-side and sent as animation).
- Text sender: message, color, speed, position.
- Controls: brightness slider, screen on/off, clock face selector, channel/EQ position.

## Data Flow

UI → REST (`/api/device/*`) → pkg/pixoo → device HTTP. CLI mode calls pkg/pixoo directly. No WebSocket in v1 — live streaming is a later phase; the REST hop is sufficient for one-shot sends.

## Error Handling

- pkg/pixoo: typed errors, wrapped transport failures, input validation (image size, angle ∈ {0,90,180,270}, xy bounds).
- API layer: maps `PixooError` and validation errors to 4xx/5xx JSON responses with message; device unreachable → 502.
- UI: surfaces API error messages inline; no silent failures.

## Testing

- `pkg/pixoo`: unit tests against `httptest.Server` faking the device — assert exact command JSON, error_code handling, pic-ID rollover at 32, image encoding (known 64×64 input → known base64).
- `cmd/pixoo64` API handlers: same fake-device approach through the REST layer.
- Web: v1 keeps logic thin; component tests deferred unless client-side GIF decoding grows complex.

## Tooling

- mise tasks: `mise run dev` (Vite dev server + Go server), `mise run build` (web build → go:embed → go build), `mise run test`, `mise run check`.
- bun for web package management.

## Out of Scope (later phases, architecture already accommodates)

1. Playlists and schedulers (RSS, calendar, clock rotations) — Go task engine.
2. Live capture: screen region streaming, webcam, audio visualizer, video playback — browser capture + WebSocket frame push.
3. Divoom Cloud: login, gallery browse, PixelBean decode (AES + LZO), likes/comments.
4. Spotify album art, AI image generation, lyrics.
