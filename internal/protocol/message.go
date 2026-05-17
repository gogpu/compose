package protocol

import "fmt"

// MsgType identifies the kind of message carried by a frame header.
type MsgType uint8

const (
	// MsgFrame carries a rendered pixel buffer from module to compositor.
	MsgFrame MsgType = 0x01

	// MsgHandshake is the initial connection negotiation message.
	MsgHandshake MsgType = 0x02

	// MsgAck acknowledges receipt of a frame (compositor to module).
	MsgAck MsgType = 0x03

	// MsgFrameRequest is sent by the compositor to request the next frame
	// from a module (pull-based flow control, Wayland pattern).
	MsgFrameRequest MsgType = 0x04

	// MsgResize notifies a module that its frame dimensions have changed.
	MsgResize MsgType = 0x05

	// MsgDisconnect signals a graceful disconnection.
	MsgDisconnect MsgType = 0x06
)

// String returns the human-readable name of the message type.
func (m MsgType) String() string {
	switch m {
	case MsgFrame:
		return "Frame"
	case MsgHandshake:
		return "Handshake"
	case MsgAck:
		return "Ack"
	case MsgFrameRequest:
		return "FrameRequest"
	case MsgResize:
		return "Resize"
	case MsgDisconnect:
		return "Disconnect"
	default:
		return fmt.Sprintf("MsgType(0x%02X)", uint8(m))
	}
}

// Valid reports whether m is a known message type.
func (m MsgType) Valid() bool {
	switch m {
	case MsgFrame, MsgHandshake, MsgAck, MsgFrameRequest, MsgResize, MsgDisconnect:
		return true
	default:
		return false
	}
}

// Flag is a bitfield carried in the header's Flags byte.
type Flag uint8

const (
	// FlagDirtyValid indicates that the DirtyRect fields contain valid data.
	// When not set, the entire frame is considered dirty (keyframe).
	FlagDirtyValid Flag = 1 << iota

	// FlagCompressed indicates that the payload is compressed.
	// The Compression field specifies the algorithm.
	FlagCompressed

	// FlagKeyframe marks the frame as a full keyframe (no delta dependency).
	FlagKeyframe
)

// Has reports whether the flag f is set in the bitmask flags.
func (f Flag) Has(flag Flag) bool {
	return f&flag != 0
}

// Set returns the bitmask with flag set.
func (f Flag) Set(flag Flag) Flag {
	return f | flag
}

// Clear returns the bitmask with flag cleared.
func (f Flag) Clear(flag Flag) Flag {
	return f &^ flag
}

// String returns the human-readable name of the flag bit.
func (f Flag) String() string {
	switch f {
	case FlagDirtyValid:
		return "DirtyValid"
	case FlagCompressed:
		return "Compressed"
	case FlagKeyframe:
		return "Keyframe"
	default:
		return fmt.Sprintf("Flag(0x%02X)", uint8(f))
	}
}

// PixelFormat identifies the pixel encoding of frame payload data.
type PixelFormat uint8

const (
	// PixelRGBA8 is 8-bit RGBA, 4 bytes per pixel, premultiplied alpha.
	PixelRGBA8 PixelFormat = 0x00

	// PixelBGRA8 is 8-bit BGRA, 4 bytes per pixel, premultiplied alpha.
	// Common on Windows/DX12 surfaces.
	PixelBGRA8 PixelFormat = 0x01
)

// String returns the human-readable name of the pixel format.
func (p PixelFormat) String() string {
	switch p {
	case PixelRGBA8:
		return "RGBA8"
	case PixelBGRA8:
		return "BGRA8"
	default:
		return fmt.Sprintf("PixelFormat(0x%02X)", uint8(p))
	}
}

// Valid reports whether p is a known pixel format.
func (p PixelFormat) Valid() bool {
	switch p {
	case PixelRGBA8, PixelBGRA8:
		return true
	default:
		return false
	}
}

// Compression identifies the payload compression algorithm.
type Compression uint8

const (
	// CompressionNone means the payload is uncompressed raw pixels.
	CompressionNone Compression = 0x00

	// CompressionLZ4 uses the LZ4 block compression algorithm.
	CompressionLZ4 Compression = 0x01

	// CompressionZstd uses the Zstandard compression algorithm.
	CompressionZstd Compression = 0x02
)

// String returns the human-readable name of the compression algorithm.
func (c Compression) String() string {
	switch c {
	case CompressionNone:
		return "None"
	case CompressionLZ4:
		return "LZ4"
	case CompressionZstd:
		return "Zstd"
	default:
		return fmt.Sprintf("Compression(0x%02X)", uint8(c))
	}
}

// Valid reports whether c is a known compression algorithm.
func (c Compression) Valid() bool {
	switch c {
	case CompressionNone, CompressionLZ4, CompressionZstd:
		return true
	default:
		return false
	}
}
