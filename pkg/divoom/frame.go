// Package divoom implements the reverse-engineered Divoom Bluetooth Classic
// binary protocol used by Pixoo Max and related devices.
package divoom

import "encoding/binary"

// checksum returns the rolling sum of payload as u16LE, widening to u32LE
// when the sum no longer fits (the device expects this widening).
func checksum(payload []byte) []byte {
	var sum uint32
	for _, b := range payload {
		sum += uint32(b)
	}
	if sum >= 0xFFFF {
		return binary.LittleEndian.AppendUint32(nil, sum)
	}
	return binary.LittleEndian.AppendUint16(nil, uint16(sum))
}

// makeMessage wraps a payload into a wire message:
// 0x01 + payload + checksum(payload) + 0x02.
func makeMessage(payload []byte) []byte {
	msg := make([]byte, 0, len(payload)+7)
	msg = append(msg, 0x01)
	msg = append(msg, payload...)
	msg = append(msg, checksum(payload)...)
	return append(msg, 0x02)
}

// makeCommand builds the wire message for a command byte with arguments.
// The leading length field counts itself (2) plus the command byte.
func makeCommand(cmd byte, args []byte) []byte {
	payload := make([]byte, 0, len(args)+3)
	payload = binary.LittleEndian.AppendUint16(payload, uint16(len(args)+3))
	payload = append(payload, cmd)
	payload = append(payload, args...)
	return makeMessage(payload)
}
