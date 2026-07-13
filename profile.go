package divoom

// Profile describes a Divoom device model's protocol parameters.
type Profile struct {
	Name       string
	ScreenSize int // native display resolution (square)
	SmallSize  int // smaller resolution the device upscales itself, 0 if none
	ChunkSize  int // max bytes per animation chunk command
	// WideCounters selects the animation chunk header width:
	// u32LE total size + u16LE index (Pixoo Max) vs u16LE + u8.
	WideCounters bool
}

// PixooMax is the Divoom Pixoo Max (32x32).
var PixooMax = Profile{
	Name:         "Pixoo Max",
	ScreenSize:   32,
	SmallSize:    16,
	ChunkSize:    200,
	WideCounters: true,
}
