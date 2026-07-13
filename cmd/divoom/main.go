package main

import (
	"flag"
	"fmt"
	"image"
	"image/gif"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	_ "image/jpeg"
	_ "image/png"

	divoom "github.com/thedaneeffect/go-divoom"
)

func main() {
	// On macOS the IOBluetooth transport needs the main thread servicing
	// the event loop, so all real work runs on a second goroutine.
	// Elsewhere RunEventLoop is a plain call.
	divoom.RunEventLoop(func() {
		os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
	})
}

// run parses args and dispatches to a command, writing to stdout/stderr and
// returning a process exit code. It contains no os.Exit calls itself so it
// can be exercised directly from tests; main is the only caller that turns
// its return value into a real exit.
func run(args []string, stdout, stderr io.Writer) int {
	// flag.Usage is part of the standard library's help contract: anything
	// that ends up driving flag.CommandLine (e.g. a future package using the
	// top-level flag.* funcs) picks up our manual instead of the default
	// one-line dump.
	flag.Usage = func() { printUsage(stderr) }

	if len(args) == 0 {
		// Bare `divoom` used to silently start the server, which surprised
		// users expecting a command list. Print help instead; `divoom serve`
		// still works when asked for explicitly.
		printUsage(stdout)
		return 0
	}
	if isHelpArg(args[0]) {
		if len(args) > 1 {
			return helpFor(args[1], stdout, stderr)
		}
		printUsage(stdout)
		return 0
	}

	fs := flag.NewFlagSet("divoom", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(stderr) }
	serialFlag := fs.String("serial", "", serialFlagUsage)
	macFlag := fs.String("mac", "", macFlagUsage)
	directFlag := fs.Bool("direct", false, directFlagUsage)
	if err := fs.Parse(args); err != nil {
		// fs.Usage already printed the manual to stderr (flag calls it for
		// both -h/--help and genuine parse errors).
		return 2
	}

	rest := fs.Args()
	if len(rest) == 0 {
		printUsage(stdout)
		return 0
	}
	name, cmdArgs := rest[0], rest[1:]

	if len(cmdArgs) > 0 && isHelpArg(cmdArgs[0]) {
		return helpFor(name, stdout, stderr)
	}
	if name == "help" {
		if len(cmdArgs) == 0 {
			printUsage(stdout)
			return 0
		}
		return helpFor(cmdArgs[0], stdout, stderr)
	}

	cmd, ok := lookupCommand(name)
	if !ok {
		fmt.Fprintf(stderr, "divoom: unknown command %q\n\n", name)
		printUsage(stderr)
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(stderr, "divoom:", err)
		return 1
	}
	if *serialFlag != "" {
		cfg.Transport, cfg.SerialPath = "serial", *serialFlag
	}
	if *macFlag != "" {
		cfg.Transport, cfg.MAC = "rfcomm", *macFlag
	}
	flags := cliFlags{direct: *directFlag}

	if err := cmd.run(cfg, flags, cmdArgs, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "divoom:", err)
		return 1
	}
	return 0
}

// helpFor prints the focused manual page for one command to stdout, or (if
// name isn't a known command) a usage error to stderr, matching the
// unknown-command behavior in run.
func helpFor(name string, stdout, stderr io.Writer) int {
	cmd, ok := lookupCommand(name)
	if !ok {
		fmt.Fprintf(stderr, "divoom: unknown command %q\n\n", name)
		printUsage(stderr)
		return 2
	}
	printCommandHelp(stdout, cmd)
	return 0
}

// cliFlags holds global command-line flags that affect how a command talks
// to the device, as opposed to cfg (the persisted device configuration).
type cliFlags struct {
	direct bool // -direct: skip the daemon probe and always dial directly
}

func cmdSend(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: divoom send <image file>")
	}
	path := args[0]
	return routeCommand(cfg, flags,
		func(baseURL string) error { return daemonSendImage(baseURL, path) },
		func(d *divoom.Device) error { return sendFile(d, path) },
	)
}

func cmdText(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: divoom text <message>")
	}
	text := strings.Join(args, " ")
	return routeCommand(cfg, flags,
		func(baseURL string) error { return daemonText(baseURL, text) },
		func(d *divoom.Device) error { return d.ShowText(text, divoom.TextOptions{}) },
	)
}

func cmdBrightness(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: divoom brightness <0-100>")
	}
	v, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("brightness must be a number: %w", err)
	}
	return routeCommand(cfg, flags,
		func(baseURL string) error { return daemonBrightness(baseURL, v) },
		func(d *divoom.Device) error { return d.SetBrightness(v) },
	)
}

func cmdOn(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	return routeCommand(cfg, flags,
		func(baseURL string) error { return daemonScreen(baseURL, true) },
		(*divoom.Device).ScreenOn,
	)
}

func cmdOff(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	return routeCommand(cfg, flags,
		func(baseURL string) error { return daemonScreen(baseURL, false) },
		(*divoom.Device).ScreenOff,
	)
}

// cmdTime syncs the device's own clock. The Pixoo has no network time source of
// its own, so its clock faces show whatever was last pushed to it — this is the
// only way to correct them.
func cmdTime(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	ts := time.Now()
	if len(args) > 0 && args[0] != "now" {
		// Accept a full timestamp or just a wall-clock time for today.
		parsed, err := parseTimeArg(args[0])
		if err != nil {
			return err
		}
		ts = parsed
	}
	return routeCommand(cfg, flags,
		func(baseURL string) error { return daemonTime(baseURL, ts) },
		func(d *divoom.Device) error { return d.SetDateTime(ts) },
	)
}

// parseTimeArg accepts RFC3339 ("2026-07-12T15:04:05Z"), a local datetime
// ("2026-07-12 15:04"), or a bare time of day ("15:04") meaning today.
func parseTimeArg(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"15:04:05",
		"15:04",
	}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, s, time.Local)
		if err != nil {
			continue
		}
		// The time-only layouts parse to year zero; graft them onto today.
		if t.Year() == 0 {
			now := time.Now()
			t = time.Date(now.Year(), now.Month(), now.Day(),
				t.Hour(), t.Minute(), t.Second(), 0, time.Local)
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q: use 'now', '15:04', '2006-01-02 15:04', or RFC3339", s)
}

func cmdClock(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	style := 0
	if len(args) > 0 {
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("clock style must be a number: %w", err)
		}
		style = v
	}
	return routeCommand(cfg, flags,
		func(baseURL string) error { return daemonClock(baseURL, style, true) },
		func(d *divoom.Device) error {
			return d.ShowClock(divoom.ClockOptions{Style: style, TwentyFour: true})
		},
	)
}

func cmdLight(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: divoom light <#RRGGBB> [brightness]")
	}
	rgb, err := parseHexColor(args[0])
	if err != nil {
		return err
	}
	brightness := 100
	if len(args) > 1 {
		v, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("light brightness must be a number: %w", err)
		}
		brightness = v
	}
	return routeCommand(cfg, flags,
		func(baseURL string) error { return daemonLight(baseURL, rgb, brightness) },
		func(d *divoom.Device) error { return d.ShowLight(rgb, brightness, true) },
	)
}

// routeCommand is the shared dispatch path for every one-shot device
// command that has a daemon-routed equivalent. When `divoom serve` is
// already running, it holds a persistent, already-pinged connection to the
// device — routing through its HTTP API (viaDaemon) skips the dial/ping
// cost of a fresh connection and completes near-instantly. When the daemon
// isn't reachable, or -direct was passed, this falls back to precisely the
// pre-daemon direct-dial path (withDevice/runDevice below), including the
// Ping-on-dial and Flush-before-close hardware barriers.
//
// The two paths are mutually exclusive for a single invocation: if the
// probe says the daemon is up but viaDaemon itself then fails, that error
// is returned as-is rather than retried via a direct dial, since the
// daemon already holds the device's only connection and a concurrent
// direct dial would contend with it.
func routeCommand(cfg Config, flags cliFlags, viaDaemon func(baseURL string) error, direct func(*divoom.Device) error) error {
	if probeDaemon(cfg) {
		// The device accepts exactly one RFCOMM channel at a time, and dialing a
		// second one while the daemon holds the first doesn't just fail — it wedges
		// the device's Bluetooth stack until it is power-cycled. So -direct is
		// refused rather than honored while the daemon is up.
		if flags.direct {
			return fmt.Errorf("the daemon is running and holds the device's only connection; " +
				"stop it (or omit -direct) — dialing directly alongside it wedges the device")
		}
		return viaDaemon(daemonBaseURL(cfg.ListenAddr))
	}
	return withDevice(cfg, direct)
}

// withDevice dials the device, runs fn, and flushes before closing.
//
// One-shot CLI invocations only get a single write/read cycle before the
// transport goes away, unlike `divoom serve`'s long-lived connection. On
// hardware, closing the RFCOMM channel immediately after a write can tear
// down the link before the device has consumed it, silently dropping the
// command even though Write and Ping both reported success. runDevice's
// trailing d.Flush() is a barrier that blocks until the device proves it
// has drained fn's writes, so that failure surfaces instead of vanishing.
//
// The transport must be closed on every exit path — including after a
// fatal error — because skipping it can leave the Bluetooth ACL link up,
// wedging the adapter for other connections. So the work runs in
// runDevice, which returns an error instead of exiting, and Close always
// runs before this function returns anything to its caller.
// dialFunc is the seam withDevice uses to obtain a transport. It defaults
// to dial (the real Bluetooth/serial dialer); tests substitute a fake so
// the direct-dial fallback path can be exercised without touching
// hardware.
var dialFunc = dial

func withDevice(cfg Config, fn func(*divoom.Device) error) error {
	t, err := dialFunc(cfg)
	if err != nil {
		return err
	}
	d := divoom.NewDevice(t, divoom.PixooMax)
	runErr := runDevice(d, fn)
	closeErr := d.Close()
	if runErr != nil {
		if closeErr != nil {
			fmt.Fprintln(os.Stderr, "divoom: close:", closeErr)
		}
		return runErr
	}
	return closeErr
}

// runDevice pings to confirm the link is live, runs fn, then flushes to
// confirm the device drained fn's writes before the caller closes the
// transport out from under it.
func runDevice(d *divoom.Device, fn func(*divoom.Device) error) error {
	if err := d.Ping(); err != nil {
		return err
	}
	if err := fn(d); err != nil {
		return err
	}
	return d.Flush()
}

func dial(cfg Config) (divoom.Transport, error) {
	switch cfg.Transport {
	case "serial":
		if cfg.SerialPath == "" {
			return nil, fmt.Errorf("no serial port configured; pass -serial /dev/cu.YourPixoo or run 'divoom config'")
		}
		return divoom.DialSerial(cfg.SerialPath)
	case "rfcomm":
		if cfg.MAC == "" {
			return nil, fmt.Errorf("no MAC configured; pass -mac AA:BB:CC:DD:EE:FF")
		}
		return divoom.DialRFCOMM(cfg.MAC, cfg.Channel)
	default:
		return nil, fmt.Errorf("unknown transport %q", cfg.Transport)
	}
}

// sendFile displays an image file; animated GIFs become animations using
// their own frame delays (first frame's delay applies to all).
func sendFile(d *divoom.Device, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if strings.HasSuffix(strings.ToLower(path), ".gif") {
		g, err := gif.DecodeAll(f)
		if err != nil {
			return fmt.Errorf("decode gif: %w", err)
		}
		if len(g.Image) > 1 {
			frames := gifFrames(g)
			delay := 100 * time.Millisecond
			if len(g.Delay) > 0 && g.Delay[0] > 0 {
				delay = time.Duration(g.Delay[0]) * 10 * time.Millisecond
			}
			return d.SendAnimation(frames, delay)
		}
		return d.SendImage(g.Image[0])
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	return d.SendImage(img)
}

func parseHexColor(s string) ([3]uint8, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return [3]uint8{}, fmt.Errorf("color must be #RRGGBB, got %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return [3]uint8{}, fmt.Errorf("invalid color %q: %w", s, err)
	}
	return [3]uint8{uint8(v >> 16), uint8(v >> 8), uint8(v)}, nil
}
