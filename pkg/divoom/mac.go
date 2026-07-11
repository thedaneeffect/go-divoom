package divoom

import (
	"fmt"
	"net"
)

// parseMAC parses a Bluetooth address into big-endian bytes (as written).
func parseMAC(s string) ([6]byte, error) {
	var out [6]byte
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return out, fmt.Errorf("divoom: invalid Bluetooth address %q", s)
	}
	copy(out[:], hw)
	return out, nil
}
