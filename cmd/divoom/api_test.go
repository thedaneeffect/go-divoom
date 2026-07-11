package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thedaneeffect/go-divoom/pkg/divoom"
)

// fakeConn records writes and satisfies divoom.Transport.
type fakeConn struct{ bytes.Buffer }

func (f *fakeConn) Close() error { return nil }

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
	want, _ := hex.DecodeString("010400740a820002")
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

func TestImageUpload(t *testing.T) {
	srv, fc := newTestServer(t)

	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
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
	want, _ := hex.DecodeString("01310044000a0a04aa2a0000000001ff00000000000000000000000000000000000000000000000000000000000000000000610202")
	if !bytes.Equal(fc.Bytes(), want) {
		t.Errorf("wire bytes:\ngot  %x\nwant %x", fc.Bytes(), want)
	}
}
