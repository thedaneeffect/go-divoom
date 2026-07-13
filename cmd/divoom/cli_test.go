package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCapture runs the CLI dispatcher against fresh stdout/stderr buffers and
// returns their contents alongside the exit code.
func runCapture(t *testing.T, args []string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	code = run(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

// TestHelpListsAllCommands guards against a command being added to the
// dispatch table without a corresponding help entry: it iterates the same
// commands slice run() dispatches from and asserts every name shows up in
// the printed manual, so help and dispatch can never drift apart silently.
func TestHelpListsAllCommands(t *testing.T) {
	if len(commands) == 0 {
		t.Fatal("commands table is empty")
	}
	stdout, _, code := runCapture(t, nil)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, c := range commands {
		if !strings.Contains(stdout, c.name) {
			t.Errorf("usage output missing command %q:\n%s", c.name, stdout)
		}
		if c.short == "" {
			t.Errorf("command %q has no short description", c.name)
		}
		if c.run == nil {
			t.Errorf("command %q has no handler", c.name)
		}
	}
}

// TestNoArgsPrintsHelp asserts bare `divoom` prints the manual instead of
// silently starting the server, while still exiting 0 (it's not an error).
func TestNoArgsPrintsHelp(t *testing.T) {
	stdout, stderr, code := runCapture(t, nil)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "USAGE") {
		t.Errorf("stdout missing USAGE section:\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

// TestTopLevelHelpForms asserts -h, --help, and help all print the same
// manual to stdout and exit 0.
func TestTopLevelHelpForms(t *testing.T) {
	base, _, code := runCapture(t, nil)
	if code != 0 {
		t.Fatalf("baseline exit code = %d, want 0", code)
	}
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		stdout, stderr, code := runCapture(t, args)
		if code != 0 {
			t.Errorf("run(%v) exit code = %d, want 0", args, code)
		}
		if stdout != base {
			t.Errorf("run(%v) stdout differs from bare invocation", args)
		}
		if stderr != "" {
			t.Errorf("run(%v) stderr = %q, want empty", args, stderr)
		}
	}
}

// TestPerCommandHelp asserts `divoom help <cmd>` and `divoom <cmd> -h` print
// a focused page mentioning the command's usage line, and agree with each
// other.
func TestPerCommandHelp(t *testing.T) {
	for _, c := range commands {
		viaHelp, _, code := runCapture(t, []string{"help", c.name})
		if code != 0 {
			t.Errorf("help %s: exit code = %d, want 0", c.name, code)
		}
		if !strings.Contains(viaHelp, "usage: divoom "+c.name) {
			t.Errorf("help %s: missing usage line:\n%s", c.name, viaHelp)
		}
		viaFlag, _, code := runCapture(t, []string{c.name, "-h"})
		if code != 0 {
			t.Errorf("%s -h: exit code = %d, want 0", c.name, code)
		}
		if viaFlag != viaHelp {
			t.Errorf("%s -h and help %s disagree:\n%s\n---\n%s", c.name, c.name, viaFlag, viaHelp)
		}
	}
}

// TestHelpUnknownCommand asserts asking for help on a nonexistent command is
// treated the same as an unknown top-level command.
func TestHelpUnknownCommand(t *testing.T) {
	stdout, stderr, code := runCapture(t, []string{"help", "bogus"})
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "bogus") || !strings.Contains(stderr, "USAGE") {
		t.Errorf("stderr missing unknown-command message and/or usage:\n%s", stderr)
	}
}

// TestUnknownCommand asserts an unrecognized command exits non-zero (2) and
// writes its usage error to stderr, leaving stdout untouched.
func TestUnknownCommand(t *testing.T) {
	stdout, stderr, code := runCapture(t, []string{"frobnicate"})
	if code == 0 {
		t.Errorf("exit code = %d, want non-zero", code)
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, `unknown command "frobnicate"`) {
		t.Errorf("stderr missing unknown-command message:\n%s", stderr)
	}
	if !strings.Contains(stderr, "USAGE") {
		t.Errorf("stderr missing usage:\n%s", stderr)
	}
}

func readConfigFile(t *testing.T, xdgHome string) Config {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(xdgHome, "go-divoom", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

// TestUseValidMAC asserts `divoom use <mac>` persists an rfcomm config with
// the given MAC.
func TestUseValidMAC(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	stdout, stderr, code := runCapture(t, []string{"use", "AA:BB:CC:DD:EE:FF"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "AA:BB:CC:DD:EE:FF") {
		t.Errorf("stdout missing saved MAC:\n%s", stdout)
	}

	cfg := readConfigFile(t, xdg)
	want := Config{Transport: "rfcomm", MAC: "AA:BB:CC:DD:EE:FF", Channel: 1, ListenAddr: ":8377"}
	if cfg != want {
		t.Errorf("config = %+v, want %+v", cfg, want)
	}
}

// TestUseSerialPath asserts `divoom use <path>` persists a serial config
// when the target looks like a filesystem path.
func TestUseSerialPath(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	stdout, stderr, code := runCapture(t, []string{"use", "/dev/cu.Pixoo-Max"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "/dev/cu.Pixoo-Max") {
		t.Errorf("stdout missing saved path:\n%s", stdout)
	}

	cfg := readConfigFile(t, xdg)
	want := Config{Transport: "serial", SerialPath: "/dev/cu.Pixoo-Max", Channel: 1, ListenAddr: ":8377"}
	if cfg != want {
		t.Errorf("config = %+v, want %+v", cfg, want)
	}
}

// TestUseGarbageMAC asserts a MAC-shaped-but-invalid target errors instead
// of silently writing garbage to the config, and that no config file is
// created at all.
func TestUseGarbageMAC(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	_, stderr, code := runCapture(t, []string{"use", "not-a-mac"})
	if code == 0 {
		t.Fatalf("exit code = %d, want non-zero", code)
	}
	if !strings.Contains(stderr, "invalid Bluetooth MAC") {
		t.Errorf("stderr missing validation message:\n%s", stderr)
	}

	if _, err := os.Stat(filepath.Join(xdg, "go-divoom", "config.json")); !os.IsNotExist(err) {
		t.Errorf("config file was created for a garbage MAC (err = %v)", err)
	}
}

// TestUseMissingArg asserts `divoom use` with no target is a usage error.
func TestUseMissingArg(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	_, stderr, code := runCapture(t, []string{"use"})
	if code == 0 {
		t.Fatalf("exit code = %d, want non-zero", code)
	}
	if !strings.Contains(stderr, "usage: divoom use") {
		t.Errorf("stderr missing usage message:\n%s", stderr)
	}
}

// TestConfigCommandNoConfigYet asserts `divoom config` points the user at
// `divoom use` instead of printing nothing when no config has been saved.
func TestConfigCommandNoConfigYet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	stdout, _, code := runCapture(t, []string{"config"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "divoom use") {
		t.Errorf("stdout missing hint to run divoom use:\n%s", stdout)
	}
}

func TestValidateMAC(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"AA:BB:CC:DD:EE:FF", false},
		{"AA-BB-CC-DD-EE-FF", false},
		{"not-a-mac", true},
		{"", true},
		{"AA:BB:CC:DD:EE", true}, // too short
	}
	for _, tc := range cases {
		err := validateMAC(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateMAC(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
	}
}
