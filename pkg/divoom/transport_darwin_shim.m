//go:build darwin && cgo

// See transport_darwin_shim.h for the API and threading contract. Compiled
// without ARC (cgo default), so retain/release is manual.
//
// Design notes:
//   - All delegate callbacks arrive on the main dispatch queue (verified by
//     probing: callbacks land on the main thread even for channels opened
//     from background threads; the sync open variants fail instantly off
//     the main thread). The Go side keeps the main runloop serviced via
//     divoom_run_event_loop.
//   - The pipe write fd is touched only from main-queue blocks (delegate
//     callbacks and the close block), so fd writes and the final close are
//     naturally serialized without extra locking.
//   - Connection objects are released via dispatch_after on the main queue
//     rather than immediately at close: IOBluetooth may still deliver a
//     late rfcommChannelClosed/data callback that was enqueued before
//     closeChannel took effect, and the delay keeps the delegate alive for
//     any such stragglers.

#import <Foundation/Foundation.h>
#import <IOBluetooth/IOBluetooth.h>

#include <errno.h>
#include <fcntl.h>
#include <pthread.h>
#include <stdatomic.h>
#include <unistd.h>

#include "transport_darwin_shim.h"

// ---- Event loop -----------------------------------------------------------

static atomic_int divoom_loop_stop;    // set by divoom_stop_event_loop
static atomic_int divoom_loop_running; // set while divoom_run_event_loop runs

void divoom_run_event_loop(void) {
    atomic_store(&divoom_loop_running, 1);
    // A runloop with no sources returns from CFRunLoopRunInMode immediately;
    // park a far-future timer so the loop actually blocks between events.
    CFRunLoopTimerRef keepalive = CFRunLoopTimerCreate(
        kCFAllocatorDefault, CFAbsoluteTimeGetCurrent() + 1e9, 1e9, 0, 0,
        NULL, NULL);
    CFRunLoopAddTimer(CFRunLoopGetCurrent(), keepalive, kCFRunLoopDefaultMode);
    while (!atomic_load(&divoom_loop_stop)) {
        @autoreleasepool {
            CFRunLoopRunInMode(kCFRunLoopDefaultMode, 3600.0, true);
        }
    }
    CFRunLoopRemoveTimer(CFRunLoopGetCurrent(), keepalive, kCFRunLoopDefaultMode);
    CFRelease(keepalive);
    atomic_store(&divoom_loop_running, 0);
}

void divoom_stop_event_loop(void) {
    atomic_store(&divoom_loop_stop, 1);
    CFRunLoopStop(CFRunLoopGetMain());
}

// ---- Connection object -----------------------------------------------------

@interface DivoomRFCOMMConn : NSObject <IOBluetoothRFCOMMChannelDelegate> {
@public
    int writeFD; // owned; touched only on the main queue; -1 once closed
    IOBluetoothDevice *device;
    IOBluetoothRFCOMMChannel *channel;
    BluetoothRFCOMMMTU mtu;
    IOReturn openStatus;
    dispatch_semaphore_t openSem; // signaled by rfcommChannelOpenComplete
}
- (void)closeWriteFD;
@end

@implementation DivoomRFCOMMConn

// closeWriteFD must only run on the main queue (see fd serialization note).
- (void)closeWriteFD {
    if (writeFD >= 0) {
        close(writeFD);
        writeFD = -1;
    }
}

- (void)rfcommChannelOpenComplete:(IOBluetoothRFCOMMChannel *)rfcommChannel
                           status:(IOReturn)error {
    openStatus = error;
    dispatch_semaphore_signal(openSem);
}

- (void)rfcommChannelData:(IOBluetoothRFCOMMChannel *)rfcommChannel
                     data:(void *)dataPointer
                   length:(size_t)dataLength {
    // Main queue. The fd is nonblocking: a full pipe (Go reader wedged)
    // must not freeze the main queue, or every IOBluetooth callback in the
    // process would stall. Retry briefly, then treat the connection's data
    // path as dead.
    const uint8_t *p = dataPointer;
    size_t left = dataLength;
    int spins = 0;
    while (left > 0 && writeFD >= 0) {
        ssize_t n = write(writeFD, p, left);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            if (errno == EAGAIN && spins < 200) { // ~2s total
                spins++;
                usleep(10000);
                continue;
            }
            // EPIPE (Go closed the read end), persistent EAGAIN, or a real
            // error: no functioning reader remains.
            [self closeWriteFD];
            return;
        }
        p += n;
        left -= (size_t)n;
    }
}

- (void)rfcommChannelClosed:(IOBluetoothRFCOMMChannel *)rfcommChannel {
    // Remote close or connection loss: EOF the Go read end.
    [self closeWriteFD];
}

- (void)dealloc {
    [device release];
    [channel release];
    if (openSem) {
        dispatch_release(openSem);
    }
    [super dealloc];
}

@end

// ---- Handle table -----------------------------------------------------------

#define DIVOOM_MAX_CONNS 32

static struct {
    int handle;
    DivoomRFCOMMConn *conn;
} divoom_table[DIVOOM_MAX_CONNS];
static int divoom_next_handle = 1;
static pthread_mutex_t divoom_table_lock = PTHREAD_MUTEX_INITIALIZER;

// divoom_lookup returns the connection for handle, retained (caller
// releases), or nil.
static DivoomRFCOMMConn *divoom_lookup(int handle) {
    DivoomRFCOMMConn *conn = nil;
    pthread_mutex_lock(&divoom_table_lock);
    for (int i = 0; i < DIVOOM_MAX_CONNS; i++) {
        if (divoom_table[i].conn != nil && divoom_table[i].handle == handle) {
            conn = [divoom_table[i].conn retain];
            break;
        }
    }
    pthread_mutex_unlock(&divoom_table_lock);
    return conn;
}

// divoom_take removes the handle's entry and returns the table's reference
// (ownership transfers to the caller), or nil.
static DivoomRFCOMMConn *divoom_take(int handle) {
    DivoomRFCOMMConn *conn = nil;
    pthread_mutex_lock(&divoom_table_lock);
    for (int i = 0; i < DIVOOM_MAX_CONNS; i++) {
        if (divoom_table[i].conn != nil && divoom_table[i].handle == handle) {
            conn = divoom_table[i].conn;
            divoom_table[i].conn = nil;
            break;
        }
    }
    pthread_mutex_unlock(&divoom_table_lock);
    return conn;
}

// divoom_release_later releases conn on the main queue after a grace period
// so any IOBluetooth callback already in flight still finds a live delegate.
static void divoom_release_later(DivoomRFCOMMConn *conn) {
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC),
                   dispatch_get_main_queue(), ^{
                     [conn release];
                   });
}

// divoom_teardown closes the channel/connection (any thread; these are XPC
// sends) and schedules the fd close + delegate release on the main queue.
// Consumes the caller's reference to conn.
static void divoom_teardown(DivoomRFCOMMConn *conn) {
    [conn->channel closeChannel];
    [conn->device closeConnection];
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    dispatch_async(dispatch_get_main_queue(), ^{
      [conn closeWriteFD];
      dispatch_semaphore_signal(done);
    });
    // Bounded wait so a dead event loop cannot hang Close; on timeout the
    // fd close (and the EOF it delivers) happens whenever the loop resumes.
    dispatch_semaphore_wait(done,
                            dispatch_time(DISPATCH_TIME_NOW, 3 * NSEC_PER_SEC));
    dispatch_release(done);
    divoom_release_later(conn);
}

// ---- Public API -------------------------------------------------------------

int divoom_rfcomm_open(const char *addr, uint8_t channel, int write_fd,
                       int timeout_ms) {
    @autoreleasepool {
        // Writes from the delegate must never raise SIGPIPE (it would kill
        // the whole process if Go's read end closed first), and must never
        // block the main queue (see rfcommChannelData).
        fcntl(write_fd, F_SETNOSIGPIPE, 1);
        int fl = fcntl(write_fd, F_GETFL, 0);
        fcntl(write_fd, F_SETFL, fl | O_NONBLOCK);

        if (!atomic_load(&divoom_loop_running)) {
            close(write_fd);
            return DIVOOM_ERR_NO_EVENT_LOOP;
        }

        DivoomRFCOMMConn *conn = [[DivoomRFCOMMConn alloc] init];
        conn->writeFD = write_fd;
        conn->openSem = dispatch_semaphore_create(0);
        conn->openStatus = kIOReturnError;

        IOBluetoothDevice *dev = [IOBluetoothDevice
            deviceWithAddressString:[NSString stringWithUTF8String:addr]];
        if (dev == nil) {
            close(write_fd);
            conn->writeFD = -1;
            [conn release];
            return (int)kIOReturnNoDevice;
        }
        conn->device = [dev retain];

        IOBluetoothRFCOMMChannel *ch = nil;
        IOReturn st = [dev openRFCOMMChannelAsync:&ch
                                    withChannelID:channel
                                         delegate:conn];
        if (st != kIOReturnSuccess || ch == nil) {
            [dev closeConnection];
            close(write_fd);
            conn->writeFD = -1;
            divoom_release_later(conn); // late callbacks may still arrive
            return st != kIOReturnSuccess ? (int)st : (int)kIOReturnError;
        }
        conn->channel = [ch retain];

        long timedOut = dispatch_semaphore_wait(
            conn->openSem,
            dispatch_time(DISPATCH_TIME_NOW,
                          (int64_t)timeout_ms * NSEC_PER_MSEC));
        if (timedOut != 0 || conn->openStatus != kIOReturnSuccess) {
            int result =
                timedOut != 0 ? (int)kIOReturnTimeout : (int)conn->openStatus;
            divoom_teardown(conn); // consumes the alloc reference
            return result;
        }

        conn->mtu = [conn->channel getMTU];
        if (conn->mtu == 0) {
            conn->mtu = 127; // RFCOMM spec minimum; defensive only
        }

        pthread_mutex_lock(&divoom_table_lock);
        int h = -1;
        for (int i = 0; i < DIVOOM_MAX_CONNS; i++) {
            if (divoom_table[i].conn == nil) {
                h = divoom_next_handle++;
                divoom_table[i].handle = h;
                divoom_table[i].conn = conn; // table takes the alloc reference
                break;
            }
        }
        pthread_mutex_unlock(&divoom_table_lock);

        if (h < 0) { // table full: tear the connection back down
            divoom_teardown(conn);
            return (int)kIOReturnNoResources;
        }
        return h;
    }
}

int divoom_rfcomm_write(int handle, const void *buf, size_t len) {
    @autoreleasepool {
        DivoomRFCOMMConn *conn = divoom_lookup(handle);
        if (conn == nil) {
            return (int)kIOReturnNotOpen;
        }
        const uint8_t *p = buf;
        IOReturn st = kIOReturnSuccess;
        while (len > 0) {
            UInt16 chunk = len > conn->mtu ? conn->mtu : (UInt16)len;
            st = [conn->channel writeSync:(void *)p length:chunk];
            if (st != kIOReturnSuccess) {
                break;
            }
            p += chunk;
            len -= chunk;
        }
        [conn release];
        return (int)st;
    }
}

int divoom_rfcomm_close(int handle) {
    @autoreleasepool {
        DivoomRFCOMMConn *conn = divoom_take(handle);
        if (conn == nil) {
            return 0;
        }
        divoom_teardown(conn);
        return 0;
    }
}
