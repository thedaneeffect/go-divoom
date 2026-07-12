//go:build darwin && cgo

// IOBluetooth RFCOMM shim for the darwin transport. Handle-based C API so
// the Go side never stores pointers across the cgo boundary (cgo pointer
// rules); each handle maps to an Objective-C connection object.
//
// Threading contract (empirically verified on macOS 15/26): modern
// IOBluetooth is a wrapper over CoreBluetooth's classic-transport support
// and delivers ALL RFCOMM delegate callbacks (open-complete, data, close)
// on the process MAIN dispatch queue, regardless of which thread opened the
// channel. openRFCOMMChannelSync fails instantly off the main thread. The
// process therefore must service the main runloop: divoom_run_event_loop
// must be running on the main OS thread for any of this to work.
#ifndef DIVOOM_TRANSPORT_DARWIN_SHIM_H
#define DIVOOM_TRANSPORT_DARWIN_SHIM_H

#include <stddef.h>
#include <stdint.h>

// divoom_event_loop_prepare marks the event loop as running and clears any
// previous stop request. Call it BEFORE starting workers that may dial and
// before divoom_run_event_loop, so a dial issued immediately never races
// the loop startup. The prepare/run/stop cycle may be repeated sequentially
// (one loop at a time).
void divoom_event_loop_prepare(void);

// divoom_run_event_loop services the main-thread runloop (and with it the
// main dispatch queue that IOBluetooth delivers callbacks on). Must be
// called on the process's main OS thread after divoom_event_loop_prepare.
// Blocks until divoom_stop_event_loop is called.
void divoom_run_event_loop(void);

// divoom_stop_event_loop makes divoom_run_event_loop return. Callable from
// any thread, before or after the loop starts.
void divoom_stop_event_loop(void);

// Error code returned by divoom_rfcomm_open when the main event loop is not
// being serviced (distinct from the negative IOReturn errors).
#define DIVOOM_ERR_NO_EVENT_LOOP 0

// divoom_rfcomm_open connects to addr (dash-separated, e.g.
// "aa-bb-cc-dd-ee-ff") on the given RFCOMM channel. Received bytes are
// written raw to write_fd; the shim takes ownership of write_fd and closes
// it when the channel closes, the connection drops, or the handle is closed
// (the Go pipe read end then returns EOF). Blocks up to timeout_ms waiting
// for the open to complete. Returns a handle >= 1 on success, a negative
// IOReturn error code on failure, or DIVOOM_ERR_NO_EVENT_LOOP if the main
// event loop is not running (in every failure case write_fd is closed,
// possibly asynchronously).
int divoom_rfcomm_open(const char *addr, uint8_t channel, int write_fd,
                       int timeout_ms);

// divoom_rfcomm_write synchronously writes len bytes, chunked to the
// channel MTU. Safe to call from any thread except the main one (it blocks
// on completion callbacks the main queue must deliver). Returns 0 on
// success or a nonzero IOReturn error code.
int divoom_rfcomm_write(int handle, const void *buf, size_t len);

// divoom_rfcomm_close closes the RFCOMM channel and baseband connection and
// closes the pipe write fd (ordered on the main queue after any in-flight
// data callbacks). Idempotent per handle. Returns 0.
int divoom_rfcomm_close(int handle);

#endif
