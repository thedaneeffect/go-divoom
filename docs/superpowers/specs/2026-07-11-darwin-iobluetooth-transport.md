# Darwin IOBluetooth Transport

Date: 2026-07-11
Status: Implemented (design amended after hardware investigation)

## Problem

macOS's `/dev/cu.*` Bluetooth SPP tty emulation is unreliable: it works roughly once per pairing; after the fd closes, reopening yields a dead channel that only re-pairing recovers. Empirically proven not to be a device limitation — direct IOBluetooth RFCOMM connections to the Pixoo Max reconnect indefinitely (3/3 ping ACKs across open/close cycles, channel 1, MTU 666; see `docs/superpowers/specs/hardware-smoke.md` and the PyObjC probe used during investigation).

## Design

Implement `DialRFCOMM(mac string, channel uint8)` for darwin in `pkg/divoom/transport_darwin.go` using IOBluetooth via a small cgo Objective-C shim (`transport_darwin_shim.m/.h`). `transport_other.go` build tag narrows to `!linux && !windows && !darwin`. `DialSerial` remains as fallback; `CGO_ENABLED=0` darwin builds get a `DialRFCOMM` stub that directs callers to `DialSerial`.

### Amendment: main-queue callback model (supersedes the original dedicated-thread design)

The originally approved design — a dedicated ObjC thread owning the channel and running a CFRunLoop for delegate callbacks — is **not viable** on current macOS and was replaced during implementation. Verified with minimal ObjC repro binaries plus bluetoothd logs:

- Modern IOBluetooth is a wrapper over CoreBluetooth's private classic transport (`CBRFCOMMChannel`; visible in the class's selector list, e.g. `initWithCBRFCOMMChannel:delegate:`, `stream:handleEvent:`).
- All RFCOMM delegate callbacks (`rfcommChannelOpenComplete`, `rfcommChannelData`, `rfcommChannelClosed`) are delivered on the **process main dispatch queue**, regardless of which thread opened the channel or which runloops are being pumped.
- `openRFCOMMChannelSync` fails instantly with kIOReturnError when called off the main thread (bluetoothd logs show the open request and client-side cancel in the same millisecond). The PyObjC probe only worked because Python ran on the main thread while pumping the main runloop.

Actual shim contract:

- Open: `deviceWithAddressString` → `openRFCOMMChannelAsync` from the dialing (cgo) thread, then a semaphore wait (15s bound) for the open-complete callback, which the main queue delivers.
- Received data: the delegate (on the main queue) writes raw bytes into an OS pipe, nonblocking with a bounded retry so a wedged reader can never stall the main queue. Go wraps the pipe's read end in `*os.File` — `Read` and `SetReadDeadline` come for free, so `Device.Ping()`'s deadline works unchanged.
- Write: Go calls the shim's synchronous write (`writeSync`, chunked to channel MTU), safe from any non-main thread while the loop runs.
- Close: `closeChannel` + `closeConnection` (XPC sends, any thread); delegate detach and pipe-write-fd close are serialized on the main queue; the connection object is released after a grace period for any in-flight callbacks. Closing the baseband link matters: a leaked ACL connection (e.g. from a killed process) makes every subsequent RFCOMM connect fail system-wide (`OI_RFCOMM_Connect status=911`) until `blueutil --disconnect` clears it.
- Cgo pointer rules: handles (small ints) map to ObjC connection objects in a C-side table; no Go pointers cross the boundary.

Public API consequence: on macOS the main queue must be serviced, so the package exposes `divoom.RunEventLoop(f func())` — it runs the main runloop on the main OS thread (guaranteed by `runtime.LockOSThread` in the darwin transport's `init`) while `f` runs on a goroutine, returning when `f` does. Sequential calls are supported. On non-darwin/nocgo builds it is a pass-through that just calls `f`. `cmd/divoom` wraps its work in it; dialing without the loop returns a distinct "event loop not running" error.

Device timing quirk: reconnecting within ~2s of a disconnect can produce an open-but-silent session (occasionally longer); `Device.Ping()`'s 3-attempt retry plus clean ACL teardown covers it.

## Testing

- Unit: parseMAC reuse; shim not unit-testable without hardware — gate is `go build`/`go vet` on darwin plus live hardware validation (brightness/ping over `-mac`, repeated CLI invocations proving reconnect works without re-pair).
- Cross-compile: linux/windows builds unaffected (darwin file is tag-guarded; cgo only active for darwin native builds).
- Hardware validation (2026-07-11, Pixoo Max, MAC AA:BB:CC:DD:EE:FF): 4/4 consecutive CLI runs (brightness/light) with 5s gaps, all exit 0, ACL cleanly down between runs.
