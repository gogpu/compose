package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// HeaderSize is the fixed size in bytes of the wire frame header.
// It is designed to be cache-line aligned (64 bytes on modern CPUs).
const HeaderSize = 64

// Magic is the 4-byte protocol identifier at the start of every header.
// ASCII: "COMP" (0x43, 0x4F, 0x4D, 0x50).
var Magic = [4]byte{0x43, 0x4F, 0x4D, 0x50}

// ProtocolVersion is the current wire protocol version.
const ProtocolVersion uint16 = 1

// Header is the 64-byte frame header that precedes every message on the wire.
// All multi-byte fields are little-endian encoded.
type Header struct {
	// Magic is the protocol identifier (must be "COMP").
	Magic [4]byte

	// Version is the protocol version number.
	Version uint16

	// MsgType identifies the message kind (Frame, Handshake, Ack, etc.).
	MsgType MsgType

	// Flags is a bitfield (DirtyValid, Compressed, Keyframe).
	Flags Flag

	// ModuleID is the compositor-assigned module identifier.
	ModuleID uint64

	// Sequence is the monotonically increasing frame counter per module.
	Sequence uint64

	// TimestampNs is the monotonic clock timestamp in nanoseconds.
	TimestampNs int64

	// Width is the frame width in pixels.
	Width uint16

	// Height is the frame height in pixels.
	Height uint16

	// Stride is the number of bytes per row (typically Width * 4).
	Stride uint32

	// DirtyX is the X offset of the dirty rectangle.
	DirtyX uint16

	// DirtyY is the Y offset of the dirty rectangle.
	DirtyY uint16

	// DirtyW is the width of the dirty rectangle.
	DirtyW uint16

	// DirtyH is the height of the dirty rectangle.
	DirtyH uint16

	// PixelFormat identifies the pixel encoding (RGBA8, BGRA8).
	PixelFormat PixelFormat

	// Compression identifies the payload compression algorithm.
	Compression Compression

	// Reserved is padding for future use. Must be zero.
	Reserved [6]byte

	// PayloadSize is the number of payload bytes following this header.
	PayloadSize uint32

	// UncompressedSize is the original payload size before compression.
	// When Compression is None, this equals PayloadSize.
	UncompressedSize uint32
}

// Field byte offsets within the 64-byte header.
const (
	offMagic            = 0
	offVersion          = 4
	offMsgType          = 6
	offFlags            = 7
	offModuleID         = 8
	offSequence         = 16
	offTimestampNs      = 24
	offWidth            = 32
	offHeight           = 34
	offStride           = 36
	offDirtyX           = 40
	offDirtyY           = 42
	offDirtyW           = 44
	offDirtyH           = 46
	offPixelFormat      = 48
	offCompression      = 49
	offReserved         = 50
	offPayloadSize      = 56
	offUncompressedSize = 60
)

// Errors returned by Encode and Decode.
var (
	// ErrBufferTooSmall is returned when the provided buffer is smaller than HeaderSize.
	ErrBufferTooSmall = errors.New("protocol: buffer too small (need 64 bytes)")

	// ErrInvalidMagic is returned when the decoded magic bytes do not match "COMP".
	ErrInvalidMagic = errors.New("protocol: invalid magic (expected 0x434F4D50)")

	// ErrUnknownMsgType is returned when the decoded message type is not recognized.
	ErrUnknownMsgType = errors.New("protocol: unknown message type")
)

// Encode writes the header h into buf using little-endian byte order.
// buf must be at least HeaderSize (64) bytes. Encode never allocates.
func Encode(h *Header, buf []byte) error {
	if len(buf) < HeaderSize {
		return ErrBufferTooSmall
	}

	// Magic (4 bytes)
	buf[offMagic] = h.Magic[0]
	buf[offMagic+1] = h.Magic[1]
	buf[offMagic+2] = h.Magic[2]
	buf[offMagic+3] = h.Magic[3]

	// Version (2 bytes)
	binary.LittleEndian.PutUint16(buf[offVersion:], h.Version)

	// MsgType (1 byte)
	buf[offMsgType] = uint8(h.MsgType)

	// Flags (1 byte)
	buf[offFlags] = uint8(h.Flags)

	// ModuleID (8 bytes)
	binary.LittleEndian.PutUint64(buf[offModuleID:], h.ModuleID)

	// Sequence (8 bytes)
	binary.LittleEndian.PutUint64(buf[offSequence:], h.Sequence)

	// TimestampNs (8 bytes, signed as uint64 bit pattern)
	binary.LittleEndian.PutUint64(buf[offTimestampNs:], uint64(h.TimestampNs)) //nolint:gosec // bit-cast int64→uint64, no overflow

	// Width (2 bytes)
	binary.LittleEndian.PutUint16(buf[offWidth:], h.Width)

	// Height (2 bytes)
	binary.LittleEndian.PutUint16(buf[offHeight:], h.Height)

	// Stride (4 bytes)
	binary.LittleEndian.PutUint32(buf[offStride:], h.Stride)

	// DirtyX (2 bytes)
	binary.LittleEndian.PutUint16(buf[offDirtyX:], h.DirtyX)

	// DirtyY (2 bytes)
	binary.LittleEndian.PutUint16(buf[offDirtyY:], h.DirtyY)

	// DirtyW (2 bytes)
	binary.LittleEndian.PutUint16(buf[offDirtyW:], h.DirtyW)

	// DirtyH (2 bytes)
	binary.LittleEndian.PutUint16(buf[offDirtyH:], h.DirtyH)

	// PixelFormat (1 byte)
	buf[offPixelFormat] = uint8(h.PixelFormat)

	// Compression (1 byte)
	buf[offCompression] = uint8(h.Compression)

	// Reserved (6 bytes, zero)
	buf[offReserved] = 0
	buf[offReserved+1] = 0
	buf[offReserved+2] = 0
	buf[offReserved+3] = 0
	buf[offReserved+4] = 0
	buf[offReserved+5] = 0

	// PayloadSize (4 bytes)
	binary.LittleEndian.PutUint32(buf[offPayloadSize:], h.PayloadSize)

	// UncompressedSize (4 bytes)
	binary.LittleEndian.PutUint32(buf[offUncompressedSize:], h.UncompressedSize)

	return nil
}

// Decode reads a header from buf and validates the magic bytes and message type.
// buf must be at least HeaderSize (64) bytes. Decode never allocates.
func Decode(buf []byte) (Header, error) {
	if len(buf) < HeaderSize {
		return Header{}, ErrBufferTooSmall
	}

	var h Header

	// Magic (4 bytes)
	h.Magic[0] = buf[offMagic]
	h.Magic[1] = buf[offMagic+1]
	h.Magic[2] = buf[offMagic+2]
	h.Magic[3] = buf[offMagic+3]

	if h.Magic != Magic {
		return Header{}, fmt.Errorf("%w: got [0x%02X 0x%02X 0x%02X 0x%02X]",
			ErrInvalidMagic, h.Magic[0], h.Magic[1], h.Magic[2], h.Magic[3])
	}

	// Version (2 bytes)
	h.Version = binary.LittleEndian.Uint16(buf[offVersion:])

	// MsgType (1 byte)
	h.MsgType = MsgType(buf[offMsgType])
	if !h.MsgType.Valid() {
		return Header{}, fmt.Errorf("%w: 0x%02X", ErrUnknownMsgType, uint8(h.MsgType))
	}

	// Flags (1 byte)
	h.Flags = Flag(buf[offFlags])

	// ModuleID (8 bytes)
	h.ModuleID = binary.LittleEndian.Uint64(buf[offModuleID:])

	// Sequence (8 bytes)
	h.Sequence = binary.LittleEndian.Uint64(buf[offSequence:])

	// TimestampNs (8 bytes)
	h.TimestampNs = int64(binary.LittleEndian.Uint64(buf[offTimestampNs:])) //nolint:gosec // bit-cast uint64→int64, no overflow

	// Width (2 bytes)
	h.Width = binary.LittleEndian.Uint16(buf[offWidth:])

	// Height (2 bytes)
	h.Height = binary.LittleEndian.Uint16(buf[offHeight:])

	// Stride (4 bytes)
	h.Stride = binary.LittleEndian.Uint32(buf[offStride:])

	// DirtyX (2 bytes)
	h.DirtyX = binary.LittleEndian.Uint16(buf[offDirtyX:])

	// DirtyY (2 bytes)
	h.DirtyY = binary.LittleEndian.Uint16(buf[offDirtyY:])

	// DirtyW (2 bytes)
	h.DirtyW = binary.LittleEndian.Uint16(buf[offDirtyW:])

	// DirtyH (2 bytes)
	h.DirtyH = binary.LittleEndian.Uint16(buf[offDirtyH:])

	// PixelFormat (1 byte)
	h.PixelFormat = PixelFormat(buf[offPixelFormat])

	// Compression (1 byte)
	h.Compression = Compression(buf[offCompression])

	// Reserved (6 bytes)
	copy(h.Reserved[:], buf[offReserved:offReserved+6])

	// PayloadSize (4 bytes)
	h.PayloadSize = binary.LittleEndian.Uint32(buf[offPayloadSize:])

	// UncompressedSize (4 bytes)
	h.UncompressedSize = binary.LittleEndian.Uint32(buf[offUncompressedSize:])

	return h, nil
}
