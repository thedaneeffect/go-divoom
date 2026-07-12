//go:build darwin && cgo

package divoom

/*
#cgo LDFLAGS: -framework IOBluetooth -framework Foundation
#include <stdlib.h>
#include "transport_darwin_shim.h"
*/
import "C"

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openTimeout bounds the RFCOMM open (baseband connect completes in well
// under a second on a healthy link; generous for sleepy devices).
const openTimeout = 15 * time.Second

func init() {
	// RunEventLoop must execute on the process's main OS thread (macOS
	// delivers IOBluetooth callbacks through the main queue, which only the
	// main thread can service). Locking during init is the documented way
	// to guarantee main.main — and thus RunEventLoop — runs there.
	runtime.LockOSThread()
}

// RunEventLoop services the macOS main event loop while f runs on a
// separate goroutine, returning once f returns. IOBluetooth delivers all
// RFCOMM events (connection completion, received data) through the process
// main queue, so DialRFCOMM only functions while this loop is running.
// Must be called from the main goroutine:
//
//	func main() {
//		divoom.RunEventLoop(func() {
//			// dial, talk to the device, ...
//		})
//	}
func RunEventLoop(f func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer C.divoom_stop_event_loop()
		f()
	}()
	C.divoom_run_event_loop()
	<-done
}

// DialRFCOMM connects to a Bluetooth Classic device via IOBluetooth RFCOMM.
// channel is the RFCOMM channel (1 for Divoom devices).
//
// This bypasses the /dev/cu.* SPP tty emulation, which only survives about
// one open per pairing; direct IOBluetooth channels reconnect indefinitely.
// Received bytes arrive through an OS pipe whose read end is an *os.File,
// so Read and SetReadDeadline behave exactly like the other platforms'
// fd-backed transports.
//
// The caller must be running the event loop via RunEventLoop.
func DialRFCOMM(mac string, channel uint8) (Transport, error) {
	addr, err := parseMAC(mac)
	if err != nil {
		return nil, err
	}
	// IOBluetooth's deviceWithAddressString wants dash separators.
	dashed := fmt.Sprintf("%02x-%02x-%02x-%02x-%02x-%02x",
		addr[0], addr[1], addr[2], addr[3], addr[4], addr[5])

	// Raw pipe: the write end is handed to (and owned by) the shim, so it
	// must not be wrapped in an *os.File whose lifecycle Go controls. The
	// read end must be nonblocking before os.NewFile so the runtime
	// registers it with the poller and SetReadDeadline works (see
	// transport_linux.go for the same constraint).
	var p [2]int
	if err := unix.Pipe(p[:]); err != nil {
		return nil, fmt.Errorf("divoom: rfcomm pipe: %w", err)
	}
	unix.CloseOnExec(p[0])
	unix.CloseOnExec(p[1])
	if err := unix.SetNonblock(p[0], true); err != nil {
		unix.Close(p[0])
		unix.Close(p[1])
		return nil, fmt.Errorf("divoom: rfcomm pipe nonblock: %w", err)
	}

	caddr := C.CString(dashed)
	defer C.free(unsafe.Pointer(caddr))
	h := C.divoom_rfcomm_open(caddr, C.uint8_t(channel), C.int(p[1]),
		C.int(openTimeout/time.Millisecond))
	if h < 1 {
		// The shim owns the write end on every failure path.
		unix.Close(p[0])
		if h == C.DIVOOM_ERR_NO_EVENT_LOOP {
			return nil, fmt.Errorf("divoom: rfcomm open %s: event loop not running; call divoom.RunEventLoop from main", mac)
		}
		return nil, fmt.Errorf("divoom: rfcomm open %s: %s", mac, ioReturnError(h))
	}
	return &rfcommConn{
		h:   h,
		r:   os.NewFile(uintptr(p[0]), "rfcomm:"+mac),
		mac: mac,
	}, nil
}

// rfcommConn adapts an IOBluetooth RFCOMM channel (held by the ObjC shim
// behind an integer handle) to io.ReadWriteCloser plus SetReadDeadline.
type rfcommConn struct {
	mu     sync.Mutex // guards h/closed against Write/Close races
	h      C.int
	closed bool
	r      *os.File // pipe read end fed by the shim's delegate callback
	mac    string
}

func (c *rfcommConn) Read(p []byte) (int, error) {
	// Delegates to the pipe: returns EOF once the shim closes the write end
	// (remote close, connection loss, or local Close).
	return c.r.Read(p)
}

func (c *rfcommConn) SetReadDeadline(t time.Time) error {
	return c.r.SetReadDeadline(t)
}

func (c *rfcommConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, fmt.Errorf("divoom: rfcomm write %s: connection closed", c.mac)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if st := C.divoom_rfcomm_write(c.h, unsafe.Pointer(&p[0]), C.size_t(len(p))); st != 0 {
		return 0, fmt.Errorf("divoom: rfcomm write %s: %s", c.mac, ioReturnError(st))
	}
	return len(p), nil
}

func (c *rfcommConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	// Shim close first: it closes the pipe write end, so the read end is
	// EOF'd before we drop it.
	C.divoom_rfcomm_close(c.h)
	if err := c.r.Close(); err != nil {
		return fmt.Errorf("divoom: rfcomm close %s: %w", c.mac, err)
	}
	return nil
}

// ioReturnError renders a shim error code. Codes are IOKit IOReturn values
// (negative as C int, e.g. kIOReturnNoDevice = 0xe00002c0); a few common
// RFCOMM-open failures get readable names.
func ioReturnError(code C.int) error {
	c := uint32(int32(code))
	switch c {
	case 0xe00002c0: // kIOReturnNoDevice
		return fmt.Errorf("no such device (is it paired and powered on?)")
	case 0xe00002d5: // kIOReturnTimeout
		return fmt.Errorf("connection timed out (is the device in range and powered on?)")
	case 0xe00002cd: // kIOReturnNotOpen
		return fmt.Errorf("channel not open")
	case 0xe00002be: // kIOReturnNoResources
		return fmt.Errorf("too many open RFCOMM connections")
	default:
		return fmt.Errorf("IOReturn 0x%08x", c)
	}
}
