# Darwin IOBluetooth Transport

Date: 2026-07-11
Status: Approved

## Problem

macOS's `/dev/cu.*` Bluetooth SPP tty emulation is unreliable: it works roughly once per pairing; after the fd closes, reopening yields a dead channel that only re-pairing recovers. Empirically proven not to be a device limitation — direct IOBluetooth RFCOMM connections to the Pixoo Max reconnect indefinitely (3/3 ping ACKs across open/close cycles, channel 1, MTU 666; see `docs/superpowers/specs/hardware-smoke.md` and the PyObjC probe used during investigation).

## Design

Implement `DialRFCOMM(mac string, channel uint8)` for darwin in `pkg/divoom/transport_darwin.go` using IOBluetooth via a small cgo Objective-C shim. `transport_other.go` build tag narrows to `!linux && !windows && !darwin`. `DialSerial` remains as fallback.

Shim contract (`transport_darwin.m/.h`):
- A dedicated ObjC-side thread owns the RFCOMM channel and runs a CFRunLoop (IOBluetooth delegate callbacks require one).
- Open: `IOBluetoothDevice deviceWithAddressString` → `openRFCOMMChannelSync` with a delegate, performed on the runloop thread; result reported back synchronously to the caller.
- Received data: delegate writes raw bytes into an OS pipe. Go wraps the pipe's read end in `*os.File` — `Read` and `SetReadDeadline` come for free, so `Device.Ping()`'s deadline works unchanged.
- Write: Go calls the shim's synchronous write (`writeSync` semantics), which is safe to invoke from a non-runloop thread.
- Close: close channel + `closeConnection` on the runloop thread, stop the runloop, close the pipe write end (Go read end then returns EOF).

Device timing quirk: reconnecting within ~2s of a disconnect can produce an open-but-silent session; `Device.Ping()`'s 3-attempt retry already covers this.

## Testing

- Unit: parseMAC reuse; shim not unit-testable without hardware — gate is `go build`/`go vet` on darwin plus live hardware validation (brightness/ping over `-mac`, repeated CLI invocations proving reconnect works without re-pair).
- Cross-compile: linux/windows builds unaffected (darwin file is tag-guarded; cgo only active for darwin native builds).
