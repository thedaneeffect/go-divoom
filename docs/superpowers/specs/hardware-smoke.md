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

## Findings

1. **Silent-drop defect (fixed).** Writes to `/dev/cu.*` succeed even when the RFCOMM link never establishes; commands vanish. Fix: `Device.Ping()` — a get-view request/response roundtrip used as a link-up barrier, with up to 3 retries because the first write after open can be swallowed during channel establishment. CLI and server both ping after dialing; failure is loud (`no response from device (link not established?)`).

2. **macOS SPP session quirk.** Closing the serial fd tears down the Bluetooth connection, and subsequent opens (even after `blueutil --connect`; even after device power-cycle) get a dead SPP channel. Only a fresh **pairing** reliably restores it. Consequences:
   - **Server mode is the recommended usage on macOS** — one persistent connection, works indefinitely.
   - One-shot CLI works for the first invocation after pairing; repeated invocations fail loudly at ping. Recovery: `blueutil --unpair <mac> && blueutil --pair <mac> && blueutil --connect <mac>`.
   - This is a device/macOS interaction, not a go-divoom defect; Linux/Windows native RFCOMM sockets are expected to behave better (explicit connect per socket) but remain hardware-untested.

3. **Screen-on appears dim.** `ScreenOn` follows the hass-divoom reference (`show_light` with RGB(1,1,1), i.e. near-black). The display is on but dark until another view is selected. Candidate future change: use white or restore the previous channel.

4. **Animation/text upload latency.** Multi-frame uploads (scrolling text ≈ tens of chunks) take seconds over BT; rapid successive commands queue up and play through in order. Expected behavior, worth a note in the UI later.
