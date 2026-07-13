package divoom

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex fixture: %v", err)
	}
	return b
}

// Golden values from hass-divoom reference implementation
// (testdata/gen_goldens.py). Brightness frames additionally validated
// against real Pixoo Max hardware on 2026-07-11.
func TestMakeCommandBrightness(t *testing.T) {
	got := makeCommand(0x74, []byte{0x0A})
	want := mustHex(t, "010400740a820002")
	if !bytes.Equal(got, want) {
		t.Errorf("brightness 10:\ngot  %x\nwant %x", got, want)
	}

	got = makeCommand(0x74, []byte{0x64})
	want = mustHex(t, "0104007464dc0002")
	if !bytes.Equal(got, want) {
		t.Errorf("brightness 100:\ngot  %x\nwant %x", got, want)
	}
}

func TestChecksumWidens(t *testing.T) {
	// 260 bytes of 0xFF sum to 66300 >= 65535 -> u32LE checksum.
	payload := bytes.Repeat([]byte{0xFF}, 260)
	msg := makeMessage(payload)
	if len(msg) != 266 { // 1 + 260 + 4 + 1
		t.Fatalf("message length = %d, want 266", len(msg))
	}
	wantTail := mustHex(t, "fffc02010002") // last payload byte, u32LE checksum 66300, terminator
	if !bytes.Equal(msg[len(msg)-6:], wantTail) {
		t.Errorf("tail:\ngot  %x\nwant %x", msg[len(msg)-6:], wantTail)
	}
}

func TestChecksumNarrow(t *testing.T) {
	if got := checksum([]byte{0x04, 0x00, 0x74, 0x0A}); !bytes.Equal(got, []byte{0x82, 0x00}) {
		t.Errorf("checksum = %x, want 8200", got)
	}
}
