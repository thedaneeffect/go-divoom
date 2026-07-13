//go:build !(darwin && cgo)

package divoom

// RunEventLoop runs f and returns when it does. Only macOS needs a real
// event loop (IOBluetooth delivers RFCOMM events through the process main
// queue); everywhere else this is a plain call so callers can use one
// portable entry point.
func RunEventLoop(f func()) {
	f()
}
