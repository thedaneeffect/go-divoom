package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	divoom "github.com/thedaneeffect/go-divoom"
)

// pingCommandHex is the wire bytes for makeCommand(0x46, nil), i.e. the
// "get view" query Device.Ping sends as its write/read link-up barrier.
// server.device() now performs this roundtrip before handing a device to a
// handler, so every golden byte assertion below expects it prepended.
const pingCommandHex = "01030046490002"

// pingResponseHex is a real hardware reply to pingCommandHex, captured from
// a Pixoo Max over Bluetooth serial (see device_test.go in the root package).
const pingResponseHex = "011b00044655000001ff5000640001026400ffffff000100000024150c0602"

// fakeConn records writes and satisfies divoom.Transport. Read answers the
// first read after a write with the canned ping response, so Device.Ping's
// write/read barrier in server.device() succeeds against these tests.
type fakeConn struct {
	bytes.Buffer
	responded bool
}

func (f *fakeConn) Close() error { return nil }

func (f *fakeConn) Read(b []byte) (int, error) {
	if f.responded || f.Buffer.Len() == 0 {
		return 0, io.EOF
	}
	f.responded = true
	resp, err := hex.DecodeString(pingResponseHex)
	if err != nil {
		panic(err) // fixture is a constant; a decode failure is a test bug
	}
	return copy(b, resp), nil
}

func newTestServer(t *testing.T) (*server, *fakeConn) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeConn{}
	srv := newServer(defaultConfig(), func(Config) (divoom.Transport, error) { return fc, nil })
	return srv, fc
}

func TestBrightnessEndpoint(t *testing.T) {
	srv, fc := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/brightness", bytes.NewBufferString(`{"value":10}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	want, _ := hex.DecodeString(pingCommandHex + "010400740a820002")
	if !bytes.Equal(fc.Bytes(), want) {
		t.Errorf("wire bytes: got %x, want %x", fc.Bytes(), want)
	}
}

func TestBrightnessValidation(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/brightness", bytes.NewBufferString(`{"value":999}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDialFailureIs502(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := newServer(defaultConfig(), func(Config) (divoom.Transport, error) {
		return nil, fmt.Errorf("no device")
	})
	req := httptest.NewRequest("POST", "/api/brightness", bytes.NewBufferString(`{"value":10}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestStatusEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var got struct {
		Connected bool   `json:"connected"`
		Profile   string `json:"profile"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Connected || got.Profile != "Pixoo Max" {
		t.Errorf("got %+v", got)
	}
}

func TestLightBrightnessValidation(t *testing.T) {
	srv, fc := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/light", bytes.NewBufferString(`{"color":"#ffffff","brightness":200}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if fc.Len() != 0 {
		t.Errorf("transport received %d bytes, want 0", fc.Len())
	}
}

func TestLightExplicitZeroBrightness(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/light", bytes.NewBufferString(`{"color":"#ffffff","brightness":0}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status %d: %s", w.Code, w.Body)
	}
}

func TestClockStyleValidation(t *testing.T) {
	srv, fc := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/clock", bytes.NewBufferString(`{"style":99}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if fc.Len() != 0 {
		t.Errorf("transport received %d bytes, want 0", fc.Len())
	}
}

func TestConcurrentRequests(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := newServer(defaultConfig(), func(Config) (divoom.Transport, error) {
		return &fakeConn{}, nil
	})
	cfgJSON, err := json.Marshal(defaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var req *http.Request
			if i%2 == 0 {
				req = httptest.NewRequest("POST", "/api/brightness", bytes.NewBufferString(`{"value":50}`))
			} else {
				req = httptest.NewRequest("PUT", "/api/config", bytes.NewReader(cfgJSON))
			}
			srv.ServeHTTP(httptest.NewRecorder(), req)
		}(i)
	}
	wg.Wait()
}

func TestDevicesEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.scan = func() ([]foundDevice, string, error) {
		return []foundDevice{
			{mac: "AA:BB:CC:DD:EE:FF", name: "Pixoo-Max"},
			{mac: "AA:BB:CC:DD:EE:FF", name: "Some Other Device"},
		}, "", nil
	}

	req := httptest.NewRequest("GET", "/api/devices", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}

	var got struct {
		Devices []struct {
			Name string `json:"name"`
			MAC  string `json:"mac"`
		} `json:"devices"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Devices) != 2 || got.Devices[0].MAC != "AA:BB:CC:DD:EE:FF" || got.Devices[0].Name != "Pixoo-Max" {
		t.Errorf("got %+v", got)
	}
	if got.Note != "" {
		t.Errorf("note = %q, want empty", got.Note)
	}
}

// TestDevicesEndpointUnsupportedPlatform verifies the "not a bug" path: no
// scanner available for this OS/toolchain must come back as an empty list
// plus an explanatory note, never a 500.
func TestDevicesEndpointUnsupportedPlatform(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.scan = func() ([]foundDevice, string, error) {
		return nil, fallbackMessage, nil
	}

	req := httptest.NewRequest("GET", "/api/devices", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}

	var got struct {
		Devices []any  `json:"devices"`
		Note    string `json:"note"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Devices) != 0 {
		t.Errorf("devices = %+v, want empty", got.Devices)
	}
	if got.Note == "" {
		t.Error("note is empty, want an explanation")
	}
}

func TestDevicesEndpointScanError(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.scan = func() ([]foundDevice, string, error) {
		return nil, "", fmt.Errorf("blueutil --inquiry: exit status 1")
	}

	req := httptest.NewRequest("GET", "/api/devices", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestPutConfigRejectsGarbageMAC(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	fc := &fakeConn{}
	srv := newServer(defaultConfig(), func(Config) (divoom.Transport, error) { return fc, nil })

	before, err := os.ReadFile(filepath.Join(dir, "go-divoom", "config.json"))
	beforeExists := err == nil

	req := httptest.NewRequest("PUT", "/api/config", bytes.NewBufferString(`{"transport":"rfcomm","mac":"not-a-mac"}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}

	after, err := os.ReadFile(filepath.Join(dir, "go-divoom", "config.json"))
	afterExists := err == nil
	if beforeExists != afterExists || (beforeExists && string(before) != string(after)) {
		t.Errorf("config on disk changed: before(exists=%v)=%q after(exists=%v)=%q", beforeExists, before, afterExists, after)
	}
}

func TestPutConfigAcceptsValidMAC(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("PUT", "/api/config", bytes.NewBufferString(`{"transport":"rfcomm","mac":"AA:BB:CC:DD:EE:FF"}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}

func TestPutConfigRejectsEmptySerialPath(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("PUT", "/api/config", bytes.NewBufferString(`{"transport":"serial","serialPath":""}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPutConfigAcceptsSerialPath(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("PUT", "/api/config", bytes.NewBufferString(`{"transport":"serial","serialPath":"/dev/cu.Pixoo-Max"}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}

func TestPutConfigRejectsUnknownTransport(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("PUT", "/api/config", bytes.NewBufferString(`{"transport":"carrier-pigeon"}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestImageUpload(t *testing.T) {
	srv, fc := newTestServer(t)

	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "red.png")
	if err := png.Encode(fw, img); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/api/image", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	want, _ := hex.DecodeString(pingCommandHex + "01310044000a0a04aa2a0000000001ff00000000000000000000000000000000000000000000000000000000000000000000610202")
	if !bytes.Equal(fc.Bytes(), want) {
		t.Errorf("wire bytes:\ngot  %x\nwant %x", fc.Bytes(), want)
	}
}

func TestTextEndpointRequiresText(t *testing.T) {
	srv, fc := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/text", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if fc.Len() != 0 {
		t.Errorf("transport received %d bytes, want 0", fc.Len())
	}
}

// TestTextEndpointDefaultFont asserts a bare {"text": ...} request (no font)
// still uploads an animation, matching the pre-existing behavior.
func TestTextEndpointDefaultFont(t *testing.T) {
	srv, fc := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/text", bytes.NewBufferString(`{"text":"hi"}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if fc.Len() == 0 {
		t.Error("transport received no bytes")
	}
}

// TestTextEndpointInvalidFontIs400 asserts a font path handleText can't load
// is a client input error (400) caught by divoom.ValidateFont before the
// device is ever touched — not a 502, and critically, the device connection
// (fc) must receive zero bytes: a bad -font must never be indistinguishable
// from a device fault to server.withDevice's dropDevice, which would
// otherwise tear down a perfectly healthy connection over a client typo
// (reproduced against real hardware during development of this feature).
func TestTextEndpointInvalidFontIs400(t *testing.T) {
	srv, fc := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/text", bytes.NewBufferString(`{"text":"hi","font":"/does/not/exist.ttf"}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", w.Code, w.Body)
	}
	if fc.Len() != 0 {
		t.Errorf("transport received %d bytes, want 0 (device must never be touched for a font validation failure)", fc.Len())
	}
}
