package divoom

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// fakeTransport records writes; shared by device tests.
type fakeTransport struct{ bytes.Buffer }

func (f *fakeTransport) Close() error { return nil }

func TestDialSerialOpensFile(t *testing.T) {
	// A regular file stands in for /dev/cu.*; DialSerial is a thin open.
	path := filepath.Join(t.TempDir(), "fakeport")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tr, err := DialSerial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if _, err := tr.Write([]byte{0x01, 0x02}); err != nil {
		t.Fatal(err)
	}
}

func TestDialSerialMissing(t *testing.T) {
	if _, err := DialSerial("/nonexistent/port"); err == nil {
		t.Error("expected error for missing port")
	}
}
