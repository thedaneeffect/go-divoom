//go:build windows

package divoom

import (
	"fmt"
	"io"
	"sync"

	"golang.org/x/sys/windows"
)

// wsaStartup lazily initializes Winsock for the process. Today this is
// technically redundant: mac.go imports net (for net.ParseMAC), and on
// Windows the net package's init already runs WSAStartup before DialRFCOMM
// can be called. That is an incidental transitive import, though — if
// parseMAC ever stopped using net, Winsock init would silently vanish. The
// explicit call makes this file self-sufficient; WSAStartup is refcounted
// and idempotent, so the extra call is harmless.
var wsaStartup = sync.OnceValue(func() error {
	var data windows.WSAData
	// 0x0202 requests Winsock 2.2, matching the version the Go runtime
	// itself negotiates.
	return windows.WSAStartup(0x0202, &data)
})

// DialRFCOMM connects to a Bluetooth Classic device via Winsock AF_BTH.
// channel is the RFCOMM channel (1 for Divoom devices).
func DialRFCOMM(mac string, channel uint8) (Transport, error) {
	addr, err := parseMAC(mac)
	if err != nil {
		return nil, err
	}
	if err := wsaStartup(); err != nil {
		return nil, fmt.Errorf("divoom: wsa startup: %w", err)
	}
	fd, err := windows.Socket(windows.AF_BTH, windows.SOCK_STREAM, windows.BTHPROTO_RFCOMM)
	if err != nil {
		return nil, fmt.Errorf("divoom: bth socket: %w", err)
	}
	var btAddr uint64
	for _, b := range addr {
		btAddr = btAddr<<8 | uint64(b)
	}
	sa := &windows.SockaddrBth{BtAddr: btAddr, Port: uint32(channel)}
	if err := windows.Connect(fd, sa); err != nil {
		windows.Closesocket(fd)
		return nil, fmt.Errorf("divoom: bth connect %s: %w", mac, err)
	}
	return &bthConn{fd: fd, mac: mac}, nil
}

// bthConn adapts a Winsock Bluetooth socket to io.ReadWriteCloser.
type bthConn struct {
	fd  windows.Handle
	mac string
}

func (c *bthConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	var n uint32
	buf := windows.WSABuf{Len: uint32(len(p)), Buf: &p[0]}
	var flags uint32
	err := windows.WSARecv(c.fd, &buf, 1, &n, &flags, nil, nil)
	if err != nil {
		// Win32 leaves the transfer count unspecified on failure.
		return 0, fmt.Errorf("divoom: bth read %s: %w", c.mac, err)
	}
	if n == 0 {
		// A zero-byte, nil-error result signals a graceful peer close, same
		// as a raw recv() returning 0. Translate to io.EOF so callers get
		// the same contract os.File.Read gives the Linux backend.
		return 0, io.EOF
	}
	return int(n), nil
}

func (c *bthConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	var n uint32
	buf := windows.WSABuf{Len: uint32(len(p)), Buf: &p[0]}
	err := windows.WSASend(c.fd, &buf, 1, &n, 0, nil, nil)
	if err != nil {
		// Win32 leaves the transfer count unspecified on failure.
		return 0, fmt.Errorf("divoom: bth write %s: %w", c.mac, err)
	}
	return int(n), nil
}

func (c *bthConn) Close() error {
	if err := windows.Closesocket(c.fd); err != nil {
		return fmt.Errorf("divoom: bth close %s: %w", c.mac, err)
	}
	return nil
}
