package divoom

import "testing"

func TestParseMAC(t *testing.T) {
	got, err := parseMAC("AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	want := [6]byte{0x11, 0x75, 0x58, 0x7D, 0x38, 0x4C}
	if got != want {
		t.Errorf("got %x, want %x", got, want)
	}
	if _, err := parseMAC("nope"); err == nil {
		t.Error("expected error for invalid MAC")
	}
}
