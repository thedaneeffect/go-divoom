package divoom

import "testing"

func TestParseMAC(t *testing.T) {
	got, err := parseMAC("AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	want := [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	if got != want {
		t.Errorf("got %x, want %x", got, want)
	}
	if _, err := parseMAC("nope"); err == nil {
		t.Error("expected error for invalid MAC")
	}
}
