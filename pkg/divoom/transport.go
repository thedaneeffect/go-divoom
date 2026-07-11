package divoom

import (
	"fmt"
	"io"
	"os"
)

// Transport is a bidirectional byte stream to a Divoom device.
type Transport = io.ReadWriteCloser

// DialSerial opens a Bluetooth SPP serial port (e.g. /dev/cu.Pixoo-Max
// on macOS, created automatically when the device is paired). Opening blocks
// until the Bluetooth link is established.
func DialSerial(path string) (Transport, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("divoom: open serial port: %w", err)
	}
	return f, nil
}
