package main

import "testing"

// blueutilSample is a real `blueutil --inquiry` capture (macOS, blueutil
// 2.13.0) covering a paired Pixoo among unrelated nearby devices.
const blueutilSample = `address: e0-03-6b-bf-ea-77, not connected, not favourite, not paired, name: "Samsung CU7000 75", recent access date: 2026-07-12 17:02:48 +0000
address: aa-bb-cc-dd-ee-ff, not connected, not favourite, paired, name: "Pixoo-Max", recent access date: 2026-07-12 17:02:48 +0000
address: 54-44-a3-4e-c5-4b, not connected, not favourite, not paired, name: "Samsung Q60BD 55 TV", recent access date: 2026-07-12 17:02:48 +0000
`

func TestParseBlueutilOutput(t *testing.T) {
	devs := parseBlueutilOutput(blueutilSample)
	if len(devs) != 3 {
		t.Fatalf("got %d devices, want 3: %+v", len(devs), devs)
	}
	want := foundDevice{mac: "AA:BB:CC:DD:EE:FF", name: "Pixoo-Max"}
	if devs[1] != want {
		t.Errorf("devs[1] = %+v, want %+v", devs[1], want)
	}
}

func TestParseBlueutilOutputEmpty(t *testing.T) {
	if devs := parseBlueutilOutput("no devices found\n"); len(devs) != 0 {
		t.Errorf("got %d devices, want 0: %+v", len(devs), devs)
	}
}

const bluetoothctlSample = `Device AA:BB:CC:DD:EE:FF Pixoo-Max
Device AA:BB:CC:DD:EE:FF Some Other Device
`

func TestParseBluetoothctlOutput(t *testing.T) {
	devs := parseBluetoothctlOutput(bluetoothctlSample)
	if len(devs) != 2 {
		t.Fatalf("got %d devices, want 2: %+v", len(devs), devs)
	}
	want := foundDevice{mac: "AA:BB:CC:DD:EE:FF", name: "Pixoo-Max"}
	if devs[0] != want {
		t.Errorf("devs[0] = %+v, want %+v", devs[0], want)
	}
}
