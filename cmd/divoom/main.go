package main

import (
	"flag"
	"fmt"
	"image"
	"image/gif"
	"os"
	"strconv"
	"strings"
	"time"

	_ "image/jpeg"
	_ "image/png"

	"github.com/thedaneeffect/go-divoom/pkg/divoom"
)

func main() {
	serialFlag := flag.String("serial", "", "serial port path (overrides config)")
	macFlag := flag.String("mac", "", "Bluetooth MAC for RFCOMM (overrides config)")
	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}
	if *serialFlag != "" {
		cfg.Transport, cfg.SerialPath = "serial", *serialFlag
	}
	if *macFlag != "" {
		cfg.Transport, cfg.MAC = "rfcomm", *macFlag
	}

	args := flag.Args()
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "serve":
		fatal(serve(cfg))
	case "config":
		path, _ := configPath()
		fmt.Println(path)
		data, err := os.ReadFile(path)
		if err == nil {
			fmt.Print(string(data))
		}
	case "send":
		requireArgs(args, 1, "send <image file>")
		withDevice(cfg, func(d *divoom.Device) error { return sendFile(d, args[0]) })
	case "text":
		requireArgs(args, 1, "text <message>")
		withDevice(cfg, func(d *divoom.Device) error {
			return d.ShowText(strings.Join(args, " "), divoom.TextOptions{})
		})
	case "brightness":
		requireArgs(args, 1, "brightness <0-100>")
		v, err := strconv.Atoi(args[0])
		if err != nil {
			fatal(fmt.Errorf("brightness must be a number: %w", err))
		}
		withDevice(cfg, func(d *divoom.Device) error { return d.SetBrightness(v) })
	case "on":
		withDevice(cfg, (*divoom.Device).ScreenOn)
	case "off":
		withDevice(cfg, (*divoom.Device).ScreenOff)
	case "clock":
		style := 0
		if len(args) > 0 {
			v, err := strconv.Atoi(args[0])
			if err != nil {
				fatal(fmt.Errorf("clock style must be a number: %w", err))
			}
			style = v
		}
		withDevice(cfg, func(d *divoom.Device) error {
			return d.ShowClock(divoom.ClockOptions{Style: style, TwentyFour: true})
		})
	case "light":
		requireArgs(args, 1, "light <#RRGGBB> [brightness]")
		rgb, err := parseHexColor(args[0])
		if err != nil {
			fatal(err)
		}
		brightness := 100
		if len(args) > 1 {
			v, err := strconv.Atoi(args[1])
			if err != nil {
				fatal(fmt.Errorf("light brightness must be a number: %w", err))
			}
			brightness = v
		}
		withDevice(cfg, func(d *divoom.Device) error { return d.ShowLight(rgb, brightness, true) })
	default:
		fatal(fmt.Errorf("unknown command %q (serve|send|text|brightness|on|off|clock|light|config)", cmd))
	}
}

func withDevice(cfg Config, fn func(*divoom.Device) error) {
	t, err := dial(cfg)
	if err != nil {
		fatal(err)
	}
	d := divoom.NewDevice(t, divoom.PixooMax)
	defer d.Close()
	if err := d.Ping(); err != nil {
		fatal(err)
	}
	fatal(fn(d))
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

func requireArgs(args []string, n int, usage string) {
	if len(args) < n {
		fatal(fmt.Errorf("usage: divoom %s", usage))
	}
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "divoom:", err)
		os.Exit(1)
	}
}
