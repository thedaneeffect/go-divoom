//go:build darwin && !cgo

package divoom

import "fmt"

// DialRFCOMM on darwin requires cgo (the IOBluetooth shim). Without it —
// e.g. a CGO_ENABLED=0 cross-compile — fall back to DialSerial.
func DialRFCOMM(mac string, channel uint8) (Transport, error) {
	return nil, fmt.Errorf("divoom: RFCOMM on macOS requires a cgo-enabled build; use DialSerial")
}
