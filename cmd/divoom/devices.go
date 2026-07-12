package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// inquirySeconds bounds how long a Bluetooth inquiry scan runs. Long enough
// to catch a device that takes a moment to answer, short enough that
// `divoom devices` still feels responsive.
const inquirySeconds = 6

// foundDevice is one Bluetooth device discovered by cmdDevices.
type foundDevice struct {
	mac  string
	name string
}

// cmdDevices lists nearby/paired Bluetooth devices so a user can find their
// Pixoo's MAC without a separate tool. It shells out to the OS's Bluetooth
// CLI if one is available, falling back to a message pointing at the OS
// Bluetooth settings otherwise. No new Go module dependencies are used.
func cmdDevices(cfg Config, args []string, stdout, stderr io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return scanDarwin(stdout)
	case "linux":
		return scanLinux(stdout)
	default:
		fmt.Fprintln(stdout, fallbackMessage)
		return nil
	}
}

const fallbackMessage = "no supported Bluetooth scanner found for this OS.\n" +
	"Pair the Pixoo in your OS's Bluetooth settings, find its MAC there,\n" +
	"then run: divoom use <mac>"

// blueutilLineRE matches blueutil's default output format, e.g.:
//
//	address: aa-bb-cc-dd-ee-ff, not connected, not favourite, paired, name: "Pixoo-Max", recent access date: ...
var blueutilLineRE = regexp.MustCompile(`address:\s*([0-9a-fA-F-]{17}).*?name:\s*"([^"]*)"`)

// scanDarwin lists devices via blueutil (https://github.com/toy/blueutil),
// a small, widely-installed (brew install blueutil) CLI wrapper around
// IOBluetooth; it is not a Go dependency.
func scanDarwin(stdout io.Writer) error {
	if _, err := exec.LookPath("blueutil"); err != nil {
		fmt.Fprintln(stdout, "blueutil not found (install: brew install blueutil).")
		fmt.Fprintln(stdout, fallbackMessage)
		return nil
	}
	fmt.Fprintf(stdout, "scanning for %ds (device must already be paired to appear reliably)...\n", inquirySeconds)
	out, err := exec.Command("blueutil", "--inquiry", strconv.Itoa(inquirySeconds)).Output()
	if err != nil {
		return fmt.Errorf("blueutil --inquiry: %w", err)
	}
	printDevices(stdout, parseBlueutilOutput(string(out)))
	return nil
}

func parseBlueutilOutput(out string) []foundDevice {
	var devs []foundDevice
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		m := blueutilLineRE.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		devs = append(devs, foundDevice{
			mac:  strings.ToUpper(strings.ReplaceAll(m[1], "-", ":")),
			name: m[2],
		})
	}
	return devs
}

// bluetoothctlLineRE matches `bluetoothctl devices` output, e.g.:
//
//	Device AA:BB:CC:DD:EE:FF Pixoo-Max
var bluetoothctlLineRE = regexp.MustCompile(`^Device\s+([0-9A-Fa-f:]{17})\s+(.*)$`)

// scanLinux lists devices via bluetoothctl (part of BlueZ), which ships
// with most Linux Bluetooth stacks; it is not a Go dependency.
func scanLinux(stdout io.Writer) error {
	if _, err := exec.LookPath("bluetoothctl"); err != nil {
		fmt.Fprintln(stdout, "bluetoothctl not found.")
		fmt.Fprintln(stdout, fallbackMessage)
		return nil
	}
	out, err := exec.Command("bluetoothctl", "devices").Output()
	if err != nil {
		return fmt.Errorf("bluetoothctl devices: %w", err)
	}
	printDevices(stdout, parseBluetoothctlOutput(string(out)))
	return nil
}

func parseBluetoothctlOutput(out string) []foundDevice {
	var devs []foundDevice
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		m := bluetoothctlLineRE.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		devs = append(devs, foundDevice{mac: strings.ToUpper(m[1]), name: m[2]})
	}
	return devs
}

func printDevices(w io.Writer, devs []foundDevice) {
	if len(devs) == 0 {
		fmt.Fprintln(w, "no devices found. Is the Pixoo paired and powered on?")
		return
	}
	for _, d := range devs {
		fmt.Fprintf(w, "%-17s  %s\n", d.mac, d.name)
	}
}
