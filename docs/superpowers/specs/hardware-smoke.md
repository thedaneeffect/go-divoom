# Hardware Smoke Test — Pixoo Max (2026-07-11)

Device: Divoom Pixoo Max "Pixoo-Max", MAC AA:BB:CC:DD:EE:FF, macOS Bluetooth serial (`/dev/cu.Pixoo-Max`).

## Results (server mode, single persistent connection)

| Step | Command | Result |
|---|---|---|
| Brightness dim/restore | `POST /api/brightness` 20 → 90 | ✅ visible dim → bright |
| Solid light | `POST /api/light` `#ff0000` | ✅ solid red |
| Clock face | `POST /api/clock` style 1, 24h | ✅ (appeared in sequence) |
| Scrolling text | `POST /api/text` "HELLO" | ✅ (appeared in sequence) |
| Static image | `POST /api/image` 16×16 gradient PNG | ✅ (appeared in sequence) |
| Animated GIF | `POST /api/image` 2-frame red/blue GIF | ✅ red/blue flashing |
| Screen off/on | `POST /api/screen` | ✅ off; "on" shows near-black light screen (see notes) |
| Ping/ACK | `Draw get view` on connect | ✅ device responds `0x01…0x02` frame |

CLI one-shot commands verified working immediately after a fresh pairing (`divoom -serial /dev/cu.Pixoo-Max brightness 20`).

## Live-frame throughput (2026-07-12, RFCOMM/IOBluetooth)

Frames pushed as repeated single-image commands (`0x44`), paced at a target rate; a rate counts as sustained only if every write succeeds and a closing `Flush()` roundtrip returns within one round-trip of its idle baseline (~33 ms) — i.e. the device drained the burst rather than falling behind.

| Rate | 2-color frames (139 B) | 256-color frames (962 B) |
|---|---|---|
| ≤ 40 fps | sustained | sustained |
| 50 fps | sustained | sustained |
| 52 fps | — | backlog (254 ms after 312 frames) |
| 60 fps | backlog (297 ms) | backlog (2.0 s) |

**~50 fps is the ceiling; 30 fps runs with comfortable headroom.** The limit is command-rate bound, not bandwidth bound: a 962 B frame hits the same ceiling as a 139 B one (50 fps × 962 B ≈ 48 KB/s), so per-command device processing dominates. Backlog grows much faster for large frames once past the edge.

**Pushing frames unthrottled wedges the device's Bluetooth stack.** Writes queue locally, the channel dies (`IOReturn 0xe00002e7`), and afterwards the device refuses all RFCOMM opens (`0xe00002d6`) — a host-side `blueutil --disconnect` does NOT clear it; only a device power-cycle does. Any live-streaming feature must pace sends and treat the device as a slow consumer (bench: `Flush()`-based backlog detection).

## Findings

1. **Silent-drop defect (fixed).** Writes to `/dev/cu.*` succeed even when the RFCOMM link never establishes; commands vanish. Fix: `Device.Ping()` — a get-view request/response roundtrip used as a link-up barrier, with up to 3 retries because the first write after open can be swallowed during channel establishment. CLI and server both ping after dialing; failure is loud (`no response from device (link not established?)`).

2. **macOS SPP session quirk.** Closing the serial fd tears down the Bluetooth connection, and subsequent opens (even after `blueutil --connect`; even after device power-cycle) get a dead SPP channel. Only a fresh **pairing** reliably restores it. Consequences:
   - **Server mode is the recommended usage on macOS** — one persistent connection, works indefinitely.
   - One-shot CLI works for the first invocation after pairing; repeated invocations fail loudly at ping. Recovery: `blueutil --unpair <mac> && blueutil --pair <mac> && blueutil --connect <mac>`.
   - This is a device/macOS interaction, not a go-divoom defect; Linux/Windows native RFCOMM sockets are expected to behave better (explicit connect per socket) but remain hardware-untested.

3. **Screen-on appears dim.** `ScreenOn` follows the hass-divoom reference (`show_light` with RGB(1,1,1), i.e. near-black). The display is on but dark until another view is selected. Candidate future change: use white or restore the previous channel.

4. **Premature-close defect (fixed 2026-07-12).** One-shot CLI commands had no visible effect while the identical command via `serve` worked: `withDevice` closed the RFCOMM channel immediately after the write, tearing down the link before the device consumed it. Fix: `Device.Flush()` — the same get-view roundtrip barrier as `Ping` (shared `roundtrip()` helper); the CLI flushes after every command, and `Close` now always runs before exit (previously `fatal()`'s `os.Exit` skipped the deferred close, which can leave a wedging ACL). Proof: consecutive CLI processes read back the brightness the previous one set (`0x64` → `0x14` → `0x5a`).

5. **Animation/text upload latency.** Multi-frame uploads (scrolling text ≈ tens of chunks) take seconds over BT; rapid successive commands queue up and play through in order. Expected behavior, worth a note in the UI later.
