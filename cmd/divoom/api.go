package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	divoom "github.com/thedaneeffect/go-divoom"
)

// server owns a lazily-dialed device connection behind an HTTP API.
type server struct {
	mux    *http.ServeMux
	dialer func(Config) (divoom.Transport, error)
	scan   func() (devs []foundDevice, note string, err error)

	mu  sync.Mutex
	cfg Config
	dev *divoom.Device
}

func newServer(cfg Config, dialer func(Config) (divoom.Transport, error)) *server {
	s := &server{cfg: cfg, dialer: dialer, scan: scanDevices, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	s.mux.HandleFunc("GET /api/devices", s.handleDevices)
	s.mux.HandleFunc("POST /api/brightness", s.handleBrightness)
	s.mux.HandleFunc("POST /api/screen", s.handleScreen)
	s.mux.HandleFunc("POST /api/light", s.handleLight)
	s.mux.HandleFunc("POST /api/clock", s.handleClock)
	s.mux.HandleFunc("POST /api/time", s.handleTime)
	s.mux.HandleFunc("POST /api/text", s.handleText)
	s.mux.HandleFunc("POST /api/image", s.handleImage)
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// cmdServe is the "serve" command: run the headless daemon exposing the
// JSON API. flags is unused here — -direct only affects one-shot commands'
// routing (see routeCommand), and has no meaning for the daemon itself.
func cmdServe(cfg Config, flags cliFlags, args []string, stdout, stderr io.Writer) error {
	return serve(cfg, stdout)
}

// serve starts the headless HTTP daemon: a persistent Bluetooth connection
// behind a JSON API, with no browser UI. One-shot CLI invocations detect
// this daemon (daemonAvailable in client.go) and route through it instead
// of dialing fresh each time, because dialing costs seconds and rapid
// reconnects can wedge the device (see
// docs/superpowers/specs/hardware-smoke.md).
func serve(cfg Config, stdout io.Writer) error {
	fmt.Fprintf(stdout, "go-divoom daemon: JSON API at %s (no browser UI)\n", daemonBaseURL(cfg.ListenAddr))
	fmt.Fprintf(stdout, "device: %s\n", describeDevice(cfg))
	srv := newServer(cfg, dial)

	// Dial in the background right away rather than waiting for the first
	// request: server.device() otherwise dials lazily, which would mean
	// the very first command routed here after startup pays the same
	// dial cost a direct CLI invocation does — defeating the reason to
	// route through the daemon at all. A failure here isn't fatal: it's
	// exactly the error a direct dial would have hit too, and device()
	// retries it on the first real request.
	go func() {
		if _, err := srv.device(); err != nil {
			log.Printf("go-divoom: initial device connection failed (will retry on first command): %v", err)
		}
	}()

	log.Printf("go-divoom daemon listening on %s", cfg.ListenAddr)
	return http.ListenAndServe(cfg.ListenAddr, srv)
}

// describeDevice summarizes the configured transport and device address for
// display in `divoom serve`'s startup banner.
func describeDevice(cfg Config) string {
	switch cfg.Transport {
	case "serial":
		if cfg.SerialPath == "" {
			return "serial (not configured; run `divoom use /dev/cu.YourPixoo`)"
		}
		return "serial " + cfg.SerialPath
	case "rfcomm":
		if cfg.MAC == "" {
			return "rfcomm (not configured; run `divoom use <mac>`)"
		}
		return "rfcomm " + cfg.MAC
	default:
		return cfg.Transport
	}
}

// device returns the connected device, dialing if needed. Caller must not
// retain it past the request.
func (s *server) device() (*divoom.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dev != nil {
		return s.dev, nil
	}
	t, err := s.dialer(s.cfg)
	if err != nil {
		return nil, err
	}
	d := divoom.NewDevice(t, divoom.PixooMax)
	if err := d.Ping(); err != nil {
		t.Close()
		return nil, err
	}
	s.dev = d
	return s.dev, nil
}

// dropDevice closes and forgets the connection so the next call redials.
// It only clears s.dev if it still holds d, so a stale request's error path
// cannot tear down a newer connection dialed by a concurrent request.
func (s *server) dropDevice(d *divoom.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d != nil && s.dev == d {
		s.dev.Close()
		s.dev = nil
	}
}

// withDevice runs fn against the device, translating failures to HTTP errors.
func (s *server) withDevice(w http.ResponseWriter, fn func(*divoom.Device) error) {
	d, err := s.device()
	if err != nil {
		jsonError(w, http.StatusBadGateway, err)
		return
	}
	if err := fn(d); err != nil {
		s.dropDevice(d)
		jsonError(w, http.StatusBadGateway, err)
		return
	}
	jsonOK(w)
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	connected := s.dev != nil
	transport := s.cfg.Transport
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"connected": connected,
		"profile":   divoom.PixooMax.Name,
		"transport": transport,
	})
}

func (s *server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func (s *server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	// Reject configs that cannot possibly work before they ever touch disk or
	// an existing connection. Without this, a config naming no device (or one
	// its transport can't use) is persisted happily and only fails later, at
	// dial time, far from the write that caused it.
	if err := validateConfig(cfg); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := saveConfig(cfg); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	// Drop the connection and swap the config in one critical section so a
	// concurrent request cannot redial with the stale config in between.
	s.mu.Lock()
	if s.dev != nil {
		s.dev.Close()
		s.dev = nil
	}
	s.cfg = cfg
	s.mu.Unlock()
	jsonOK(w)
}

// handleDevices runs a Bluetooth scan (the same one `divoom devices` uses)
// and returns discovered devices as JSON. Scanning takes several seconds
// (see inquirySeconds in devices.go) — that's inherent to a Bluetooth
// inquiry scan, so callers should show a spinner/disabled state while this
// is in flight rather than treating the latency as a bug.
//
// A device that is currently connected (e.g. still configured and in use)
// will not answer an inquiry scan, so it may be absent from the results
// even though it's nearby and paired.
//
// If scanning isn't supported on this platform, or the OS's scanner CLI
// isn't installed, that's not a server error: devices comes back empty and
// note explains how to find the MAC by hand. Only a genuine scanner
// failure (the tool ran and errored) produces a non-2xx response.
func (s *server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devs, note, err := s.scan()
	if err != nil {
		jsonError(w, http.StatusBadGateway, err)
		return
	}
	type deviceJSON struct {
		Name string `json:"name"`
		MAC  string `json:"mac"`
	}
	out := make([]deviceJSON, 0, len(devs))
	for _, d := range devs {
		out = append(out, deviceJSON{Name: d.name, MAC: d.mac})
	}
	resp := map[string]any{"devices": out}
	if note != "" {
		resp["note"] = note
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *server) handleBrightness(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value int `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if req.Value < 0 || req.Value > 100 {
		jsonError(w, http.StatusBadRequest, fmt.Errorf("value %d out of range 0-100", req.Value))
		return
	}
	s.withDevice(w, func(d *divoom.Device) error { return d.SetBrightness(req.Value) })
}

func (s *server) handleScreen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.withDevice(w, func(d *divoom.Device) error {
		if req.On {
			return d.ScreenOn()
		}
		return d.ScreenOff()
	})
}

func (s *server) handleLight(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Color      string `json:"color"`
		Brightness *int   `json:"brightness"` // pointer: explicit 0 is legal, omitted defaults to 100
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	rgb, err := parseHexColor(req.Color)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	brightness := 100
	if req.Brightness != nil {
		brightness = *req.Brightness
	}
	if brightness < 0 || brightness > 100 {
		jsonError(w, http.StatusBadRequest, fmt.Errorf("brightness %d out of range 0-100", brightness))
		return
	}
	s.withDevice(w, func(d *divoom.Device) error { return d.ShowLight(rgb, brightness, true) })
}

// handleTime sets the device's internal clock. An omitted or empty "time" means
// now; a supplied RFC3339 timestamp is applied verbatim, so a CLI on another
// machine can push its own instant rather than the daemon's.
func (s *server) handleTime(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Time string `json:"time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	ts := time.Now()
	if req.Time != "" {
		parsed, err := time.Parse(time.RFC3339, req.Time)
		if err != nil {
			jsonError(w, http.StatusBadRequest, fmt.Errorf("time must be RFC3339: %w", err))
			return
		}
		ts = parsed
	}
	s.withDevice(w, func(d *divoom.Device) error { return d.SetDateTime(ts) })
}

func (s *server) handleClock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Style      int  `json:"style"`
		TwentyFour bool `json:"twentyFour"`
		Weather    bool `json:"weather"`
		Temp       bool `json:"temp"`
		Calendar   bool `json:"calendar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if req.Style < 0 || req.Style > 15 {
		jsonError(w, http.StatusBadRequest, fmt.Errorf("style %d out of range 0-15", req.Style))
		return
	}
	s.withDevice(w, func(d *divoom.Device) error {
		return d.ShowClock(divoom.ClockOptions{
			Style: req.Style, TwentyFour: req.TwentyFour,
			Weather: req.Weather, Temp: req.Temp, Calendar: req.Calendar,
		})
	})
}

func (s *server) handleText(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if req.Text == "" {
		jsonError(w, http.StatusBadRequest, fmt.Errorf("text is required"))
		return
	}
	s.withDevice(w, func(d *divoom.Device) error {
		return d.ShowText(req.Text, divoom.TextOptions{})
	})
}

func (s *server) handleImage(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Errorf("multipart field 'file' required: %w", err))
		return
	}
	defer file.Close()

	if header.Header.Get("Content-Type") == "image/gif" {
		g, err := gif.DecodeAll(file)
		if err != nil {
			jsonError(w, http.StatusBadRequest, fmt.Errorf("decode gif: %w", err))
			return
		}
		if len(g.Image) > 1 {
			frames := gifFrames(g)
			delay := 100 * time.Millisecond
			if len(g.Delay) > 0 && g.Delay[0] > 0 {
				delay = time.Duration(g.Delay[0]) * 10 * time.Millisecond
			}
			s.withDevice(w, func(d *divoom.Device) error { return d.SendAnimation(frames, delay) })
			return
		}
		s.withDevice(w, func(d *divoom.Device) error { return d.SendImage(g.Image[0]) })
		return
	}

	img, _, err := image.Decode(file)
	if err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Errorf("decode image: %w", err))
		return
	}
	s.withDevice(w, func(d *divoom.Device) error { return d.SendImage(img) })
}

func jsonOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func jsonError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
