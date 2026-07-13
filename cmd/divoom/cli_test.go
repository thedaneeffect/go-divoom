package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
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

func TestParseTimeArg(t *testing.T) {
	now := time.Now()

	got, err := parseTimeArg("15:04")
	if err != nil {
		t.Fatal(err)
	}
	// A bare time of day means today, not year zero.
	if got.Year() != now.Year() || got.Month() != now.Month() || got.Day() != now.Day() {
		t.Errorf("bare time landed on %s, want today's date", got.Format(time.RFC3339))
	}
	if got.Hour() != 15 || got.Minute() != 4 {
		t.Errorf("got %02d:%02d, want 15:04", got.Hour(), got.Minute())
	}

	got, err = parseTimeArg("2026-07-12 15:04:05")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() != 2026 || got.Day() != 12 || got.Second() != 5 {
		t.Errorf("got %s, want 2026-07-12 15:04:05", got.Format(time.RFC3339))
	}

	if _, err := parseTimeArg("half past four"); err == nil {
		t.Error("want an error for an unparseable time")
	}
}

// writeTestFont writes the Go Regular TTF (compiled into golang.org/x/image,
// already a dependency, so this needs no system font path and works on any
// OS/CI) to a temp file and returns its path.
func writeTestFont(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test-font.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatalf("write test font: %v", err)
	}
	return path
}

// TestCmdTextRequiresMessage asserts `divoom text` with no message (with or
// without flags) fails with a usage error before ever touching the daemon
// probe or a device — cmdText's own flag.FlagSet validates this locally.
func TestCmdTextRequiresMessage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stdout, stderr, code := runCapture(t, []string{"text"})
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero; stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stderr, "usage: divoom text") {
		t.Errorf("stderr = %q, want it to contain %q", stderr, "usage: divoom text")
	}
}

// TestCmdTextUnknownFlagFails asserts an unrecognized flag is rejected by
// cmdText's own FlagSet rather than silently swallowed into the message text.
func TestCmdTextUnknownFlagFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, stderr, code := runCapture(t, []string{"text", "-bogus", "hello"})
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero; stderr=%q", stderr)
	}
}

// TestCmdTextFontAndSizeFlagsRouteThroughDirect is an end-to-end check that
// -font/-size parse correctly and flow all the way through
// cmdText -> routeCommand -> ShowText -> a wire-level animation upload, the
// same shape TestBrightnessCommandFallsBackWithoutDaemon checks for
// brightness. The daemon probe is faked down so this exercises the direct
// dial fallback without a real listener.
func TestCmdTextFontAndSizeFlagsRouteThroughDirect(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fakeProbe(t, false)
	fc := &alwaysRespondingConn{}
	fakeDial(t, fc)

	fontPath := writeTestFont(t)
	stdout, stderr, code := runCapture(t, []string{"text", "-font", fontPath, "-size", "20", "hi"})
	if code != 0 {
		t.Fatalf("exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	if fc.Len() == 0 {
		t.Error("direct transport received no bytes; -font/-size never reached ShowText")
	}
}

// TestCmdTextInvalidFontPathSurfacesError asserts a bad -font path fails the
// command with a clear error (via loadFace) instead of panicking or
// silently falling back to the built-in font.
func TestCmdTextInvalidFontPathSurfacesError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fakeProbe(t, false)
	fakeDial(t, &alwaysRespondingConn{})

	missing := filepath.Join(t.TempDir(), "does-not-exist.ttf")
	_, stderr, code := runCapture(t, []string{"text", "-font", missing, "hi"})
	if code == 0 {
		t.Fatalf("exit code = 0, want nonzero for a missing font path; stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "load font") {
		t.Errorf("stderr = %q, want it to mention %q", stderr, "load font")
	}
}

// TestCmdTextDefaultFontStillWorks guards the no-regression requirement:
// `divoom text` without -font must keep using the built-in bitmap font
// exactly as before.
func TestCmdTextDefaultFontStillWorks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fakeProbe(t, false)
	fc := &alwaysRespondingConn{}
	fakeDial(t, fc)

	stdout, stderr, code := runCapture(t, []string{"text", "hello world"})
	if code != 0 {
		t.Fatalf("exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	if fc.Len() == 0 {
		t.Error("direct transport received no bytes")
	}
}
