//go:build !linux && !windows

package divoom

import "fmt"

// DialRFCOMM is unavailable on this platform. On macOS, pair the device and
// use DialSerial with the /dev/cu.* port instead.
func DialRFCOMM(mac string, channel uint8) (Transport, error) {
	return nil, fmt.Errorf("divoom: native RFCOMM not supported on this OS; use DialSerial")
}
