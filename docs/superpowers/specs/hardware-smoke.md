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

## The "60-frame animation limit" does not exist (2026-07-12)

The reference implementation warns that animations over 60 frames are "very likely cut off", and this project inherited that number on faith. It is wrong.

Measured with a counter animation — each frame displays its own index, so truncation is directly observable by reading the highest number the device reaches before looping:

| Upload | Writes | Highest frame played |
|---|---|---|
| 60 frames, 8-color (~416 B/frame) | unpaced | **4** |
| 40 frames, 2-color (~142 B/frame) | unpaced | **22** |
| 60 frames, 8-color | paced 20ms/chunk | **60** (all) |
| 120 frames, 2-color (~17 KB) | paced | **120** (all) |
| 240 frames, 2-color (~34 KB) | paced | **240** (all) |

Neither a frame cap nor a byte cap explains the unpaced numbers (4 × 416 B = 1.7 KB, but 22 × 142 B = 3.1 KB — no constant). **The truncation was receive-buffer overrun, not storage.** The device has no flow control on the animation path: it acknowledges nothing, and writes arriving faster than it consumes them are dropped silently. The animation "uploads successfully" and plays back short.

Consequences:

- `chunkPacing` (20 ms between messages of a multi-message upload) is load-bearing. Removing it silently truncates animations again.
- No storage ceiling was found up to 240 frames / 34 KB. `maxAnimationFrames` is a *budget* (bounding upload time), not a device limit.
- This is the same failure mode as the unthrottled frame-push benchmark below: flood the device and it drops, then wedges. Pace everything.

## One connection at a time (2026-07-13)

The device serves exactly one RFCOMM channel. While it is connected, it stops answering inquiry and page requests, so a second host cannot reach it — and the errors it produces do not say so:

| Host | Symptom while the device is held elsewhere |
|---|---|
| Linux / BlueZ | `EHOSTDOWN` on dial, and **no device object is created at all** — pairing can't even be attempted. Looks like the device is off. |
| macOS / IOBluetooth | `IOReturn 0xe00002d6` on RFCOMM open. |

Consequences, all verified on hardware:

- A running `divoom serve` daemon holds the device's only channel. `divoom disconnect` (POST /api/disconnect) releases it while the daemon keeps running; the next command redials (measured: 10ms cached → 4.2s after release, i.e. a genuine redial).
- The daemon releases the device on SIGINT/SIGTERM. Without that, killing it stranded the link, which is the same stale-ACL state that wedges every subsequent connect.
- Dialing a second channel while one is held does not merely fail — it can wedge the device's Bluetooth stack until it is power-cycled. This is why `-direct` is refused while the daemon is up.

## Findings

1. **Silent-drop defect (fixed).** Writes to `/dev/cu.*` succeed even when the RFCOMM link never establishes; commands vanish. Fix: `Device.Ping()` — a get-view request/response roundtrip used as a link-up barrier, with up to 3 retries because the first write after open can be swallowed during channel establishment. CLI and server both ping after dialing; failure is loud (`no response from device (link not established?)`).

2. **macOS SPP session quirk.** Closing the serial fd tears down the Bluetooth connection, and subsequent opens (even after `blueutil --connect`; even after device power-cycle) get a dead SPP channel. Only a fresh **pairing** reliably restores it. Consequences:
   - **Server mode is the recommended usage on macOS** — one persistent connection, works indefinitely.
   - One-shot CLI works for the first invocation after pairing; repeated invocations fail loudly at ping. Recovery: `blueutil --unpair <mac> && blueutil --pair <mac> && blueutil --connect <mac>`.
   - This is a device/macOS interaction, not a go-divoom defect; Linux/Windows native RFCOMM sockets are expected to behave better (explicit connect per socket) but remain hardware-untested.

3. **Screen-on appears dim.** `ScreenOn` follows the hass-divoom reference (`show_light` with RGB(1,1,1), i.e. near-black). The display is on but dark until another view is selected. Candidate future change: use white or restore the previous channel.

4. **Premature-close defect (fixed 2026-07-12).** One-shot CLI commands had no visible effect while the identical command via `serve` worked: `withDevice` closed the RFCOMM channel immediately after the write, tearing down the link before the device consumed it. Fix: `Device.Flush()` — the same get-view roundtrip barrier as `Ping` (shared `roundtrip()` helper); the CLI flushes after every command, and `Close` now always runs before exit (previously `fatal()`'s `os.Exit` skipped the deferred close, which can leave a wedging ACL). Proof: consecutive CLI processes read back the brightness the previous one set (`0x64` → `0x14` → `0x5a`).

5. **Animation/text upload latency.** Multi-frame uploads (scrolling text ≈ tens of chunks) take seconds over BT; rapid successive commands queue up and play through in order. Expected behavior, worth a note in the UI later.
