package main

import (
	"fmt"
	"io"
	"net"
	"strings"
)

// cmdUse is the "use" command: the missing setup step between pairing a
// device and running commands against it. It detects whether target looks
// like a serial path or a Bluetooth MAC and persists the corresponding
// transport to the config file.
func cmdUse(cfg Config, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: divoom use <mac|serial-path>")
	}
	target := args[0]

	if strings.Contains(target, "/") || strings.HasPrefix(target, "/dev") {
		cfg.Transport = "serial"
		cfg.SerialPath = target
	} else {
		if err := validateMAC(target); err != nil {
			return err
		}
		cfg.Transport = "rfcomm"
		cfg.MAC = target
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}
	path, err := configPath()
	if err != nil {
		return err
	}

	switch cfg.Transport {
	case "serial":
		fmt.Fprintf(stdout, "saved serial device: %s\n", cfg.SerialPath)
	case "rfcomm":
		fmt.Fprintf(stdout, "saved Bluetooth device: %s\n", cfg.MAC)
	}
	fmt.Fprintf(stdout, "config: %s\n", path)
	return nil
}

// validateMAC checks that s is a 6-byte Bluetooth address, the same shape
// divoom.DialRFCOMM requires (its parseMAC is unexported, so this mirrors
// it with the standard library rather than duplicating parsing logic).
func validateMAC(s string) error {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return fmt.Errorf("invalid Bluetooth MAC address %q (want e.g. AA:BB:CC:DD:EE:FF)", s)
	}
	return nil
}
