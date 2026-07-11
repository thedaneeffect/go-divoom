//go:build linux

package divoom

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// DialRFCOMM connects to a Bluetooth Classic device via a native RFCOMM
// socket. channel is the RFCOMM channel (1 for Divoom devices).
func DialRFCOMM(mac string, channel uint8) (Transport, error) {
	addr, err := parseMAC(mac)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_STREAM, unix.BTPROTO_RFCOMM)
	if err != nil {
		return nil, fmt.Errorf("divoom: rfcomm socket: %w", err)
	}
	// SockaddrRFCOMM wants the address little-endian (reversed from the
	// big-endian, as-written form parseMAC returns).
	var rev [6]byte
	for i := 0; i < 6; i++ {
		rev[i] = addr[5-i]
	}
	sa := &unix.SockaddrRFCOMM{Addr: rev, Channel: channel}
	if err := unix.Connect(fd, sa); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("divoom: rfcomm connect %s: %w", mac, err)
	}
	return os.NewFile(uintptr(fd), "rfcomm:"+mac), nil
}
