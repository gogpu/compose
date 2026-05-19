package compose

import "image"

// Frame is the fundamental data unit: a rectangular pixel buffer from a module.
// Users construct it to publish (module side) or receive it in callbacks
// (compositor side).
//
// On the module side, ModuleID is ignored in PublishFrame — the server assigns
// the module's ID automatically. On the compositor side, OnFrame delivers a
// Frame with ModuleID populated.
type Frame struct {
	// ModuleID is the compositor-assigned identifier.
	// Ignored on publish (server assigns).
	ModuleID uint64

	// Name is a human-readable module name (e.g., "clock", "weather").
	// Set during handshake, included in received frames for convenience.
	Name string

	// Pixels is the RGBA premultiplied pixel buffer.
	// Stride is always Width * 4.
	Pixels []byte

	// Width and Height of the frame in pixels.
	Width  uint32
	Height uint32

	// DirtyRect is the sub-region that changed since the last frame.
	// Zero value means the entire frame is dirty (keyframe).
	DirtyRect image.Rectangle

	// Timestamp is a monotonic nanosecond timestamp from the module's clock.
	Timestamp int64

	// Sequence is the monotonically increasing frame counter assigned by
	// the publishing client. It is carried from the wire protocol header
	// and can be used for change detection (e.g., comparing against a
	// previously seen sequence number in Snapshot).
	Sequence uint64
}
