package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/thedaneeffect/go-divoom/pkg/divoom"
)

// server owns a lazily-dialed device connection behind an HTTP API.
type server struct {
	mux    *http.ServeMux
	dialer func(Config) (divoom.Transport, error)

	mu  sync.Mutex
	cfg Config
	dev *divoom.Device
}

func newServer(cfg Config, dialer func(Config) (divoom.Transport, error)) *server {
	s := &server{cfg: cfg, dialer: dialer, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	s.mux.HandleFunc("POST /api/brightness", s.handleBrightness)
	s.mux.HandleFunc("POST /api/screen", s.handleScreen)
	s.mux.HandleFunc("POST /api/light", s.handleLight)
	s.mux.HandleFunc("POST /api/clock", s.handleClock)
	s.mux.HandleFunc("POST /api/text", s.handleText)
	s.mux.HandleFunc("POST /api/image", s.handleImage)
	s.mux.Handle("/", uiHandler())
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func serve(cfg Config) error {
	srv := newServer(cfg, dial)
	log.Printf("go-divoom listening on %s", cfg.ListenAddr)
	return http.ListenAndServe(cfg.ListenAddr, srv)
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
			frames := make([]image.Image, len(g.Image))
			for i, im := range g.Image {
				frames[i] = im
			}
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
