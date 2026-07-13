package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// daemonProbeTimeout bounds how long daemonAvailable waits to hear back
// from a local `divoom serve` daemon before assuming one isn't running.
// Every one-shot CLI command probes before deciding how to reach the
// device (see routeCommand in main.go), so this must stay small enough
// that the probe is unnoticeable when there's no daemon to answer it.
const daemonProbeTimeout = 300 * time.Millisecond

// daemonRequestTimeout bounds an actual daemon-routed command, once the
// probe has confirmed a daemon is listening. It's generous relative to
// daemonProbeTimeout because animation/text uploads can legitimately take
// several seconds over Bluetooth (see
// docs/superpowers/specs/hardware-smoke.md) — the daemon relays to the
// same slow link, it just skips the dial/ping cost first.
const daemonRequestTimeout = 30 * time.Second

// daemonClient is the shared HTTP client for daemon-routed commands
// (distinct from the short-timeout client daemonAvailable uses to probe).
var daemonClient = &http.Client{Timeout: daemonRequestTimeout}

// probeDaemon is the seam routeCommand (main.go) uses to decide whether a
// local daemon is reachable. It defaults to daemonAvailable; tests
// substitute a fake so routing logic can be exercised without a real HTTP
// listener.
var probeDaemon = daemonAvailable

// daemonBaseURL turns a configured listen address (e.g. ":8377" or
// "0.0.0.0:8377") into a URL for reaching that daemon from this same
// machine. It always targets the explicit loopback address rather than
// "localhost" or the configured host: the daemon being probed is
// necessarily running on this host, regardless of which interface its
// listener happens to bind.
func daemonBaseURL(listenAddr string) string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		port = strings.TrimPrefix(listenAddr, ":")
	}
	return "http://127.0.0.1:" + port
}

// daemonAvailable reports whether a `divoom serve` daemon is listening at
// cfg's configured address, via a short-timeout GET of its own status
// endpoint. It must never block noticeably: every one-shot command calls
// this to decide whether to route through the daemon or dial directly, so
// a slow probe would defeat the entire point of routing.
func daemonAvailable(cfg Config) bool {
	req, err := http.NewRequest(http.MethodGet, daemonBaseURL(cfg.ListenAddr)+"/api/status", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: daemonProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// daemonErrorBody mirrors the {"error": "..."} shape jsonError writes in
// api.go, so a daemon-side failure surfaces with the same message a direct
// dial's error would carry.
type daemonErrorBody struct {
	Error string `json:"error"`
}

// daemonResult translates a daemon HTTP response into a Go error: nil on
// 200, otherwise the JSON error message if present, else a generic status
// line.
func daemonResult(resp *http.Response) error {
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	var e daemonErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&e); err == nil && e.Error != "" {
		return fmt.Errorf("%s", e.Error)
	}
	return fmt.Errorf("daemon: unexpected status %s", resp.Status)
}

// daemonPostJSON posts body as JSON to path on the daemon at baseURL and
// translates the response via daemonResult.
func daemonPostJSON(baseURL, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := daemonClient.Post(baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("daemon %s: %w", path, err)
	}
	return daemonResult(resp)
}

// daemonBrightness posts to POST /api/brightness (see handleBrightness).
func daemonBrightness(baseURL string, value int) error {
	return daemonPostJSON(baseURL, "/api/brightness", map[string]any{"value": value})
}

// daemonScreen posts to POST /api/screen (see handleScreen).
func daemonScreen(baseURL string, on bool) error {
	return daemonPostJSON(baseURL, "/api/screen", map[string]any{"on": on})
}

// daemonLight posts to POST /api/light (see handleLight).
func daemonLight(baseURL string, rgb [3]uint8, brightness int) error {
	color := fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2])
	return daemonPostJSON(baseURL, "/api/light", map[string]any{"color": color, "brightness": brightness})
}

// daemonClock posts to POST /api/clock (see handleClock). It only exposes
// style and twentyFour since that's all the CLI's `clock` command accepts;
// the endpoint also takes weather/temp/calendar, which no command sets yet.
func daemonClock(baseURL string, style int, twentyFour bool) error {
	return daemonPostJSON(baseURL, "/api/clock", map[string]any{"style": style, "twentyFour": twentyFour})
}

// daemonTime posts to POST /api/time (see handleTime). The timestamp is sent as
// RFC3339 so the daemon applies exactly the instant the CLI resolved, rather
// than re-reading its own clock.
func daemonTime(baseURL string, ts time.Time) error {
	return daemonPostJSON(baseURL, "/api/time", map[string]any{"time": ts.Format(time.RFC3339)})
}

// daemonText posts to POST /api/text (see handleText).
func daemonText(baseURL, text string) error {
	return daemonPostJSON(baseURL, "/api/text", map[string]any{"text": text})
}

// daemonSendImage uploads an image file to the daemon's POST /api/image as
// multipart form data (field "file"). GIFs are tagged image/gif so handleImage takes its animated-GIF
// branch; everything else is tagged application/octet-stream, since
// image.Decode sniffs the format from content rather than Content-Type.
func daemonSendImage(baseURL, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	contentType := "application/octet-stream"
	if strings.HasSuffix(strings.ToLower(path), ".gif") {
		contentType = "image/gif"
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filepath.Base(path)))
	h.Set("Content-Type", contentType)
	part, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	resp, err := daemonClient.Post(baseURL+"/api/image", mw.FormDataContentType(), &body)
	if err != nil {
		return fmt.Errorf("daemon /api/image: %w", err)
	}
	return daemonResult(resp)
}
