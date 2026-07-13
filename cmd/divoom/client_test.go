package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	divoom "github.com/thedaneeffect/go-divoom"
)

// --- daemonBaseURL -----------------------------------------------------

func TestDaemonBaseURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{":8377", "http://127.0.0.1:8377"},
		{"0.0.0.0:9000", "http://127.0.0.1:9000"},
		{"127.0.0.1:8377", "http://127.0.0.1:8377"},
	}
	for _, tc := range cases {
		if got := daemonBaseURL(tc.in); got != tc.want {
			t.Errorf("daemonBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- daemonAvailable -----------------------------------------------------

func TestDaemonAvailableTrue(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"connected":false}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := defaultConfig()
	cfg.ListenAddr = srv.Listener.Addr().String()
	if !daemonAvailable(cfg) {
		t.Error("daemonAvailable() = false, want true against a live /api/status")
	}
}

// TestDaemonAvailableFalseClosedPort asserts a plain "nothing is listening"
// address (the common case: no daemon running) comes back false quickly —
// this is the case every one-shot command hits when there's no `divoom
// serve` around, so it must never add noticeable latency.
func TestDaemonAvailableFalseClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // now guaranteed nothing is listening on addr

	cfg := defaultConfig()
	cfg.ListenAddr = addr

	start := time.Now()
	got := daemonAvailable(cfg)
	elapsed := time.Since(start)

	if got {
		t.Error("daemonAvailable() = true, want false for a closed port")
	}
	if elapsed > time.Second {
		t.Errorf("daemonAvailable() took %s against a closed port, want near-instant", elapsed)
	}
}

// TestDaemonAvailableFalseUnresponsive asserts daemonAvailable does not hang
// when a listener accepts the connection but never answers: the probe's own
// timeout, not a network-level reset, must be what ends the call.
func TestDaemonAvailableFalseUnresponsive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Accept but never write a response and never close — any
			// caller can only return via its own timeout, not a
			// network-level reset.
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	defer func() {
		mu.Lock()
		for _, c := range conns {
			c.Close()
		}
		mu.Unlock()
	}()

	cfg := defaultConfig()
	cfg.ListenAddr = ln.Addr().String()

	start := time.Now()
	got := daemonAvailable(cfg)
	elapsed := time.Since(start)

	if got {
		t.Error("daemonAvailable() = true, want false against a silent listener")
	}
	if elapsed > 2*time.Second {
		t.Errorf("daemonAvailable() took %s, want well under 2s (probe timeout is %s)", elapsed, daemonProbeTimeout)
	}
}

// --- daemon-routed command bodies -----------------------------------------------------

// captureRequest builds an httptest server that records the method, path,
// and raw body of the single request it expects, then answers 200 OK.
func captureRequest(t *testing.T, gotMethod, gotPath *string, gotBody *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotMethod, *gotPath = r.Method, r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		*gotBody = body
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
}

func TestDaemonBrightnessRequest(t *testing.T) {
	var method, path string
	var body []byte
	srv := captureRequest(t, &method, &path, &body)
	defer srv.Close()

	if err := daemonBrightness(srv.URL, 42); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/api/brightness" {
		t.Errorf("got %s %s, want POST /api/brightness", method, path)
	}
	var got struct {
		Value int `json:"value"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Value != 42 {
		t.Errorf("value = %d, want 42", got.Value)
	}
}

func TestDaemonScreenRequest(t *testing.T) {
	var method, path string
	var body []byte
	srv := captureRequest(t, &method, &path, &body)
	defer srv.Close()

	if err := daemonScreen(srv.URL, true); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/api/screen" {
		t.Errorf("got %s %s, want POST /api/screen", method, path)
	}
	var got struct {
		On bool `json:"on"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if !got.On {
		t.Errorf("on = %v, want true", got.On)
	}
}

func TestDaemonLightRequest(t *testing.T) {
	var method, path string
	var body []byte
	srv := captureRequest(t, &method, &path, &body)
	defer srv.Close()

	if err := daemonLight(srv.URL, [3]uint8{0xff, 0x88, 0x00}, 50); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/api/light" {
		t.Errorf("got %s %s, want POST /api/light", method, path)
	}
	var got struct {
		Color      string `json:"color"`
		Brightness int    `json:"brightness"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Color != "#ff8800" || got.Brightness != 50 {
		t.Errorf("got %+v, want color=#ff8800 brightness=50", got)
	}
}

func TestDaemonClockRequest(t *testing.T) {
	var method, path string
	var body []byte
	srv := captureRequest(t, &method, &path, &body)
	defer srv.Close()

	if err := daemonClock(srv.URL, 3, true); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/api/clock" {
		t.Errorf("got %s %s, want POST /api/clock", method, path)
	}
	var got struct {
		Style      int  `json:"style"`
		TwentyFour bool `json:"twentyFour"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Style != 3 || !got.TwentyFour {
		t.Errorf("got %+v, want style=3 twentyFour=true", got)
	}
}

func TestDaemonTextRequest(t *testing.T) {
	var method, path string
	var body []byte
	srv := captureRequest(t, &method, &path, &body)
	defer srv.Close()

	if err := daemonText(srv.URL, "hello world"); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/api/text" {
		t.Errorf("got %s %s, want POST /api/text", method, path)
	}
	var got struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Text != "hello world" {
		t.Errorf("text = %q, want %q", got.Text, "hello world")
	}
}

func TestDaemonPostJSONErrorTranslation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "no response from device (link not established?)"})
	}))
	defer srv.Close()

	err := daemonBrightness(srv.URL, 10)
	if err == nil || !strings.Contains(err.Error(), "no response from device") {
		t.Errorf("err = %v, want it to carry the daemon's error message", err)
	}
}

// --- daemonSendImage -----------------------------------------------------

func TestDaemonSendImageStaticContentType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(path, []byte("fake-png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	var method, urlPath, contentType string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, urlPath = r.Method, r.URL.Path
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			return
		}
		defer file.Close()
		contentType = header.Header.Get("Content-Type")
		body, _ = io.ReadAll(file)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	if err := daemonSendImage(srv.URL, path); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || urlPath != "/api/image" {
		t.Errorf("got %s %s, want POST /api/image", method, urlPath)
	}
	if contentType != "application/octet-stream" {
		t.Errorf("content-type = %q, want application/octet-stream", contentType)
	}
	if string(body) != "fake-png-bytes" {
		t.Errorf("body = %q, want %q", body, "fake-png-bytes")
	}
}

// TestDaemonSendImageGifContentType asserts a .gif upload is tagged
// image/gif, matching handleImage's animated-vs-static branch in api.go —
// this is what preserves animated GIF behavior when routed through the
// daemon instead of a direct dial.
func TestDaemonSendImageGifContentType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anim.gif")
	if err := os.WriteFile(path, []byte("fake-gif-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			return
		}
		contentType = header.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	if err := daemonSendImage(srv.URL, path); err != nil {
		t.Fatal(err)
	}
	if contentType != "image/gif" {
		t.Errorf("content-type = %q, want image/gif", contentType)
	}
}

// --- routing logic -----------------------------------------------------

// fakeProbe swaps probeDaemon for a fixed answer, restoring the original
// on cleanup.
func fakeProbe(t *testing.T, up bool) {
	t.Helper()
	orig := probeDaemon
	probeDaemon = func(Config) bool { return up }
	t.Cleanup(func() { probeDaemon = orig })
}

// fakeDial swaps dialFunc for one returning t, restoring the original on
// cleanup, so the direct-dial fallback can be exercised without hardware.
func fakeDial(t *testing.T, transport divoom.Transport) {
	t.Helper()
	orig := dialFunc
	dialFunc = func(Config) (divoom.Transport, error) { return transport, nil }
	t.Cleanup(func() { dialFunc = orig })
}

// alwaysRespondingConn is a fake divoom.Transport that answers every Read
// following any Write with a canned complete "get view" frame, regardless
// of how many roundtrips are attempted. main.go's withDevice performs two
// separate roundtrip barriers around a command (Ping, then Flush), unlike
// the HTTP server path (api_test.go's fakeConn), which only pings once —
// so the fallback-path tests need a fake that can answer both.
type alwaysRespondingConn struct{ bytes.Buffer }

func (c *alwaysRespondingConn) Close() error { return nil }

func (c *alwaysRespondingConn) Read(b []byte) (int, error) {
	resp, err := hex.DecodeString(pingResponseHex)
	if err != nil {
		panic(err) // fixture is a constant; a decode failure is a test bug
	}
	return copy(b, resp), nil
}

func TestRouteCommandUsesDaemonWhenAvailable(t *testing.T) {
	fakeProbe(t, true)
	fakeDial(t, nil) // dialFunc must not be called on this path

	var daemonCalled bool
	err := routeCommand(defaultConfig(), cliFlags{},
		func(baseURL string) error { daemonCalled = true; return nil },
		func(d *divoom.Device) error { t.Fatal("direct fn called, want daemon path only"); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !daemonCalled {
		t.Error("daemon-routed function was not called")
	}
}

func TestRouteCommandFallsBackWhenDaemonDown(t *testing.T) {
	fakeProbe(t, false)
	fakeDial(t, &alwaysRespondingConn{})

	var directCalled bool
	err := routeCommand(defaultConfig(), cliFlags{},
		func(baseURL string) error { t.Fatal("daemon fn called, want direct path only"); return nil },
		func(d *divoom.Device) error { directCalled = true; return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !directCalled {
		t.Error("direct function was not called")
	}
}

// TestRouteCommandDirectFlagDialsWhenDaemonDown asserts -direct takes the
// direct-dial path when no daemon holds the device.
func TestRouteCommandDirectFlagDialsWhenDaemonDown(t *testing.T) {
	fakeProbe(t, false)
	fakeDial(t, &alwaysRespondingConn{})

	var directCalled bool
	err := routeCommand(defaultConfig(), cliFlags{direct: true},
		func(baseURL string) error { t.Fatal("daemon fn called, want direct path only"); return nil },
		func(d *divoom.Device) error { directCalled = true; return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !directCalled {
		t.Error("direct function was not called")
	}
}

// TestRouteCommandDirectFlagRefusedWhileDaemonUp guards a hardware hazard: the
// device accepts one RFCOMM channel at a time, and dialing a second one while
// the daemon holds the first wedges its Bluetooth stack until it is power-cycled
// (observed on real hardware, IOReturn 0xe00002d6). -direct must therefore be
// refused, not honored, while the daemon is reachable.
func TestRouteCommandDirectFlagRefusedWhileDaemonUp(t *testing.T) {
	fakeProbe(t, true)
	fakeDial(t, nil) // dialing at all would be the bug

	err := routeCommand(defaultConfig(), cliFlags{direct: true},
		func(baseURL string) error { t.Fatal("daemon fn called; -direct must refuse, not route"); return nil },
		func(d *divoom.Device) error {
			t.Fatal("direct dial attempted while daemon holds the device")
			return nil
		},
	)
	if err == nil {
		t.Fatal("want an error refusing -direct while the daemon is up, got nil")
	}
	if !strings.Contains(err.Error(), "daemon is running") {
		t.Errorf("error should explain the daemon holds the connection, got: %v", err)
	}
}

// TestBrightnessCommandRoutesThroughRealDaemon is an end-to-end check that
// the full CLI dispatch path (run -> cmdBrightness -> routeCommand) reaches
// a real daemon over real HTTP when one is listening at cfg's configured
// address, instead of ever dialing directly.
func TestBrightnessCommandRoutesThroughRealDaemon(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	fc := &fakeConn{} // from api_test.go; answers one ping roundtrip
	daemon := newServer(defaultConfig(), func(Config) (divoom.Transport, error) { return fc, nil })
	httpSrv := httptest.NewServer(daemon)
	defer httpSrv.Close()

	_, port, err := net.SplitHostPort(httpSrv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.ListenAddr = ":" + port
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	orig := dialFunc
	dialFunc = func(Config) (divoom.Transport, error) {
		t.Fatal("dialFunc called; expected routing through the running daemon")
		return nil, nil
	}
	t.Cleanup(func() { dialFunc = orig })

	stdout, stderr, code := runCapture(t, []string{"brightness", "50"})
	if code != 0 {
		t.Fatalf("exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	if fc.Len() == 0 {
		t.Error("fake device transport received no bytes; daemon never handled the command")
	}
}

// TestBrightnessCommandFallsBackWithoutDaemon asserts the same full CLI
// dispatch path dials directly when nothing is listening at cfg's
// configured address.
func TestBrightnessCommandFallsBackWithoutDaemon(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// A closed ephemeral port: guaranteed nothing answers the daemon probe.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.ListenAddr = ":" + port
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	fc := &alwaysRespondingConn{}
	fakeDial(t, fc)

	stdout, stderr, code := runCapture(t, []string{"brightness", "50"})
	if code != 0 {
		t.Fatalf("exit code = %d, stdout=%q stderr=%q", code, stdout, stderr)
	}
	if fc.Len() == 0 {
		t.Error("direct transport received no bytes")
	}
}

func TestDaemonTimeRequest(t *testing.T) {
	var method, path string
	var body []byte
	srv := captureRequest(t, &method, &path, &body)
	defer srv.Close()

	ts := time.Date(2026, 7, 12, 15, 4, 5, 0, time.UTC)
	if err := daemonTime(srv.URL, ts); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/api/time" {
		t.Errorf("got %s %s, want POST /api/time", method, path)
	}
	var got struct {
		Time string `json:"time"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	// The CLI's instant must survive the hop verbatim; the daemon must not
	// substitute its own clock.
	if got.Time != "2026-07-12T15:04:05Z" {
		t.Errorf("time = %q, want 2026-07-12T15:04:05Z", got.Time)
	}
}
