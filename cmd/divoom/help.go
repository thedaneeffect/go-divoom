package main

import (
	"fmt"
	"io"
	"strings"
)

// command describes one divoom subcommand: its dispatch handler and the
// help text shown both in the top-level manual and in `divoom help <name>`.
// This table is the single source of truth for dispatch and help — add a
// command here and both update automatically.
type command struct {
	name     string   // e.g. "send"
	args     string   // e.g. "<image file>"; "" if the command takes none
	short    string   // one-line description, shown in the COMMANDS table
	long     string   // paragraph shown by per-command help
	examples []string // full command lines, shown as-is under EXAMPLES
	run      func(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error
}

// commands is every divoom subcommand except "help" itself, which is
// handled specially in run() alongside -h/--help (see main.go).
var commands = []command{
	{
		name:  "serve",
		short: "run as a headless daemon exposing the JSON API",
		long:  "Starts a headless daemon exposing a JSON API and keeping a single persistent Bluetooth connection open. One-shot commands (send, text, brightness, on, off, clock, light) automatically detect and route through this daemon when it's running, which makes them near-instant instead of paying a fresh dial/ping/settle cost. There is no browser UI. Runs until interrupted.",
		examples: []string{
			"divoom serve",
		},
		run: cmdServe,
	},
	{
		name:  "devices",
		short: "list nearby Bluetooth devices (pair first)",
		long:  "Lists paired/nearby Bluetooth devices with their name and MAC address, so you can find your Pixoo without a separate tool. The Pixoo must already be paired with your OS's Bluetooth settings — this does not pair it for you.",
		examples: []string{
			"divoom devices",
		},
		run: cmdDevices,
	},
	{
		name:  "use",
		args:  "<mac|serial-path>",
		short: "save a device address/path to the config",
		long:  "Persists a Bluetooth MAC (rfcomm transport) or a serial device path (serial transport) to the config file, so -mac/-serial aren't needed on every invocation. Run `divoom devices` first if you don't know the MAC.",
		examples: []string{
			"divoom use AA:BB:CC:DD:EE:FF",
			"divoom use /dev/cu.Pixoo-Max",
		},
		run: cmdUse,
	},
	{
		name:  "config",
		short: "print the config file path and contents",
		long:  "Prints the config file's path, then its contents if it exists.",
		examples: []string{
			"divoom config",
		},
		run: cmdConfig,
	},
	{
		name:  "send",
		args:  "<image file>",
		short: "display an image or animated GIF",
		long:  "Displays an image file. Animated GIFs play as an animation using their own frame delays (the first frame's delay applies to all).",
		examples: []string{
			"divoom send animation.gif",
			"divoom send photo.png",
		},
		run: cmdSend,
	},
	{
		name:  "text",
		args:  "<message>",
		short: "scroll a text message across the display",
		long:  "Scrolls a text message across the display. Arguments after the command are joined with spaces, so quoting is only needed to control word spacing.",
		examples: []string{
			`divoom text "hello world"`,
		},
		run: cmdText,
	},
	{
		name:  "brightness",
		args:  "<0-100>",
		short: "set display brightness (0-100)",
		long:  "Sets display brightness as a percentage from 0 to 100.",
		examples: []string{
			"divoom brightness 60",
		},
		run: cmdBrightness,
	},
	{
		name:  "on",
		short: "turn the screen on",
		long:  "Turns the screen on.",
		examples: []string{
			"divoom on",
		},
		run: cmdOn,
	},
	{
		name:  "off",
		short: "turn the screen off",
		long:  "Turns the screen off.",
		examples: []string{
			"divoom off",
		},
		run: cmdOff,
	},
	{
		name:  "clock",
		args:  "[style]",
		short: "show a clock face (style 0-15, default 0)",
		long:  "Shows a clock face. style selects one of the device's built-in clock faces (0-15); it defaults to 0 if omitted. This only chooses the face — to correct the time it shows, use `divoom time`.",
		examples: []string{
			"divoom clock",
			"divoom clock 3",
		},
		run: cmdClock,
	},
	{
		name:  "time",
		args:  "[when]",
		short: "set the device's clock (defaults to now)",
		long: "Sets the device's internal clock. The Pixoo has no time source of its own, so its clock faces keep showing whatever was last pushed to it — run this whenever the clock is wrong or has drifted.\n\n" +
			"when defaults to the current local time. It also accepts a bare time of day (15:04), a local datetime (2006-01-02 15:04), or an RFC3339 timestamp.",
		examples: []string{
			"divoom time",
			"divoom time 15:04",
			"divoom time '2026-07-12 15:04:05'",
		},
		run: cmdTime,
	},
	{
		name:  "light",
		args:  "<#RRGGBB> [brightness]",
		short: "show a solid color",
		long:  "Shows a solid color at the given brightness (0-100, default 100).",
		examples: []string{
			"divoom light '#ff8800' 50",
		},
		run: cmdLight,
	},
}

// commandLabel renders a command's name and args as shown in usage text,
// e.g. "light <#RRGGBB> [brightness]".
func commandLabel(c command) string {
	if c.args == "" {
		return c.name
	}
	return c.name + " " + c.args
}

// lookupCommand finds a command by name.
func lookupCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// isHelpArg reports whether s is a way of asking for help, used to detect
// `divoom <command> -h` in addition to the top-level -h/--help/help forms.
func isHelpArg(s string) bool {
	switch s {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

// helpRow is one aligned row of the COMMANDS section: a label (name + args)
// and its one-line description.
type helpRow struct {
	label string
	short string
}

// commandRows builds the COMMANDS table rows from the commands slice, plus
// a trailing synthetic row documenting the "help" pseudo-command (which
// isn't in commands since it's handled specially in run()).
func commandRows() []helpRow {
	rows := make([]helpRow, 0, len(commands)+1)
	for _, c := range commands {
		rows = append(rows, helpRow{label: commandLabel(c), short: c.short})
	}
	rows = append(rows, helpRow{label: "help [command]", short: "show this help, or help for one command"})
	return rows
}

const (
	serialFlagUsage = "serial port path (overrides config), e.g. /dev/cu.Pixoo-Max"
	macFlagUsage    = "Bluetooth MAC for RFCOMM (overrides config)"
	directFlagUsage = "skip the daemon probe and always dial the device directly"
)

// printUsage prints the full manual: synopsis, usage, commands, global
// flags, a getting-started walkthrough, examples, and operational notes.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "divoom - control a Divoom Pixoo Max over Bluetooth (or serial)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE")
	fmt.Fprintln(w, "  divoom <command> [arguments] [global flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS")
	rows := commandRows()
	width := 0
	for _, r := range rows {
		if len(r.label) > width {
			width = len(r.label)
		}
	}
	for _, r := range rows {
		fmt.Fprintf(w, "  %-*s  %s\n", width, r.label, r.short)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "GLOBAL FLAGS")
	fmt.Fprintln(w, "  -serial <path>")
	fmt.Fprintln(w, "        "+serialFlagUsage)
	fmt.Fprintln(w, "  -mac <address>")
	fmt.Fprintln(w, "        "+macFlagUsage)
	fmt.Fprintln(w, "  -direct")
	fmt.Fprintln(w, "        "+directFlagUsage)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "GETTING STARTED")
	fmt.Fprintln(w, "  1. Pair the Pixoo with your OS's Bluetooth settings.")
	fmt.Fprintln(w, "  2. Run `divoom devices` to find its MAC (or read it from your")
	fmt.Fprintln(w, "     Bluetooth settings).")
	fmt.Fprintln(w, "  3. Run `divoom use <mac>` to save it to the config.")
	fmt.Fprintln(w, "  4. Run `divoom serve` to start the daemon (recommended - every")
	fmt.Fprintln(w, "     one-shot command below then routes through it and is near-")
	fmt.Fprintln(w, "     instant), or just run a command directly, e.g.")
	fmt.Fprintln(w, "     `divoom text \"hello\"`.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES")
	fmt.Fprintln(w, "  divoom use AA:BB:CC:DD:EE:FF")
	fmt.Fprintln(w, "  divoom serve &")
	fmt.Fprintln(w, "  divoom send animation.gif")
	fmt.Fprintln(w, "  divoom text \"hello world\"")
	fmt.Fprintln(w, "  divoom brightness 60")
	fmt.Fprintln(w, "  divoom light '#ff8800' 50")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "NOTES")
	fmt.Fprintln(w, "  Commands automatically route through `divoom serve` when it's")
	fmt.Fprintln(w, "  running (fast: no dial, no settle wait) and dial the device")
	fmt.Fprintln(w, "  directly otherwise (slower - leave a few seconds between separate")
	fmt.Fprintln(w, "  invocations so the device has time to settle before the next")
	fmt.Fprintln(w, "  connection). Pass -direct to always dial directly, skipping the")
	fmt.Fprintln(w, "  daemon even if one is running - but a Bluetooth device generally")
	fmt.Fprintln(w, "  only accepts one connection at a time, so doing this while")
	fmt.Fprintln(w, "  `divoom serve` holds one can drop the daemon's connection too")
	fmt.Fprintln(w, "  (observed on macOS/IOBluetooth); stop the daemon first if so.")
	fmt.Fprintln(w, "  If Bluetooth wedges, macOS: blueutil --disconnect <mac>.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'divoom help <command>' for details on one command.")
}

// printCommandHelp prints the focused manual page for a single command.
func printCommandHelp(w io.Writer, c command) {
	fmt.Fprintf(w, "usage: divoom %s\n\n", commandLabel(c))
	if c.long != "" {
		fmt.Fprintln(w, wrap(c.long, 78))
	} else {
		fmt.Fprintln(w, c.short)
	}
	if len(c.examples) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "EXAMPLES")
		for _, ex := range c.examples {
			fmt.Fprintf(w, "  %s\n", ex)
		}
	}
}

// wrap greedily wraps s to lines of at most width columns, breaking only on
// spaces. It never splits a word, so a single word longer than width is left
// on its own line.
func wrap(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	lineLen := 0
	for i, word := range words {
		if i > 0 {
			if lineLen+1+len(word) > width {
				b.WriteByte('\n')
				lineLen = 0
			} else {
				b.WriteByte(' ')
				lineLen++
			}
		}
		b.WriteString(word)
		lineLen += len(word)
	}
	return b.String()
}
