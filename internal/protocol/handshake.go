package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// HandshakeSize is the fixed size in bytes of both HelloMsg and WelcomeMsg.
const HandshakeSize = 128

// TransportType identifies the preferred or granted transport mechanism.
type TransportType uint8

const (
	// TransportSocket indicates Unix domain socket transport.
	TransportSocket TransportType = 0

	// TransportShm indicates shared memory transport.
	TransportShm TransportType = 1
)

// String returns the human-readable name of the transport type.
func (t TransportType) String() string {
	switch t {
	case TransportSocket:
		return "Socket"
	case TransportShm:
		return "SharedMemory"
	default:
		return fmt.Sprintf("TransportType(%d)", uint8(t))
	}
}

// HelloMsg is sent by the module to the compositor during the handshake phase.
// Fixed 128 bytes.
//
// Layout: Magic(4) + Version(2) + Name(64) + Width(2) + Height(2) +
// PreferredFPS(2) + Transport(1) + Reserved(51) = 128
type HelloMsg struct {
	// Magic must be "COMP" (0x43, 0x4F, 0x4D, 0x50).
	Magic [4]byte

	// Version is the protocol version the module supports.
	Version uint16

	// Name is the null-terminated human-readable module name (max 63 chars + NUL).
	Name [64]byte

	// Width is the initial frame width in pixels.
	Width uint16

	// Height is the initial frame height in pixels.
	Height uint16

	// PreferredFPS is the module's preferred frame rate (e.g., 1 for clock, 60 for animation).
	PreferredFPS uint16

	// Transport is the module's preferred transport mechanism.
	Transport TransportType

	// Reserved is padding for future use. Must be zero.
	// Size: 128 - 4 - 2 - 64 - 2 - 2 - 2 - 1 = 51
	Reserved [51]byte
}

// HelloMsg field offsets.
const (
	helloOffMagic        = 0
	helloOffVersion      = 4
	helloOffName         = 6
	helloOffWidth        = 70
	helloOffHeight       = 72
	helloOffPreferredFPS = 74
	helloOffTransport    = 76
	helloOffReserved     = 77
)

// WelcomeMsg is sent by the compositor to the module after accepting the handshake.
// Fixed 128 bytes.
//
// Layout: Magic(4) + Version(2) + ModuleID(8) + Accepted(1) + Transport(1) +
// MinVersion(2) + MaxVersion(2) + Reserved(108) = 128
type WelcomeMsg struct {
	// Magic must be "COMP" (0x43, 0x4F, 0x4D, 0x50).
	Magic [4]byte

	// Version is the protocol version the compositor selected.
	Version uint16

	// ModuleID is the compositor-assigned unique identifier for this module.
	ModuleID uint64

	// Accepted is 1 if the connection was accepted, 0 if rejected.
	Accepted uint8

	// Transport is the transport mechanism the compositor granted.
	Transport TransportType

	// MinVersion is the minimum protocol version the compositor supports.
	MinVersion uint16

	// MaxVersion is the maximum protocol version the compositor supports.
	MaxVersion uint16

	// Reserved is padding for future use. Must be zero.
	// Size: 128 - 4 - 2 - 8 - 1 - 1 - 2 - 2 = 108
	Reserved [108]byte
}

// WelcomeMsg field offsets.
const (
	welcomeOffMagic      = 0
	welcomeOffVersion    = 4
	welcomeOffModuleID   = 6
	welcomeOffAccepted   = 14
	welcomeOffTransport  = 15
	welcomeOffMinVersion = 16
	welcomeOffMaxVersion = 18
	welcomeOffReserved   = 20
)

// Handshake errors.
var (
	// ErrHandshakeBufTooSmall is returned when the buffer is smaller than HandshakeSize.
	ErrHandshakeBufTooSmall = errors.New("protocol: handshake buffer too small (need 128 bytes)")

	// ErrHandshakeInvalidMagic is returned when handshake magic bytes are wrong.
	ErrHandshakeInvalidMagic = errors.New("protocol: handshake invalid magic (expected 0x434F4D50)")

	// ErrRejected is returned when the compositor rejected the connection.
	ErrRejected = errors.New("protocol: connection rejected by compositor")
)

// EncodeHello writes a HelloMsg into buf using little-endian byte order.
// buf must be at least HandshakeSize (128) bytes. Never allocates.
func EncodeHello(msg *HelloMsg, buf []byte) error {
	if len(buf) < HandshakeSize {
		return ErrHandshakeBufTooSmall
	}

	// Zero the buffer to ensure reserved bytes are clean.
	clear(buf[:HandshakeSize])

	// Magic
	buf[helloOffMagic] = msg.Magic[0]
	buf[helloOffMagic+1] = msg.Magic[1]
	buf[helloOffMagic+2] = msg.Magic[2]
	buf[helloOffMagic+3] = msg.Magic[3]

	// Version
	binary.LittleEndian.PutUint16(buf[helloOffVersion:], msg.Version)

	// Name (64 bytes, null-terminated)
	copy(buf[helloOffName:helloOffName+64], msg.Name[:])

	// Width
	binary.LittleEndian.PutUint16(buf[helloOffWidth:], msg.Width)

	// Height
	binary.LittleEndian.PutUint16(buf[helloOffHeight:], msg.Height)

	// PreferredFPS
	binary.LittleEndian.PutUint16(buf[helloOffPreferredFPS:], msg.PreferredFPS)

	// Transport
	buf[helloOffTransport] = uint8(msg.Transport)

	// Reserved already zeroed by clear()

	return nil
}

// DecodeHello reads a HelloMsg from buf and validates the magic bytes.
// buf must be at least HandshakeSize (128) bytes. Never allocates.
func DecodeHello(buf []byte) (HelloMsg, error) {
	if len(buf) < HandshakeSize {
		return HelloMsg{}, ErrHandshakeBufTooSmall
	}

	var msg HelloMsg

	// Magic
	msg.Magic[0] = buf[helloOffMagic]
	msg.Magic[1] = buf[helloOffMagic+1]
	msg.Magic[2] = buf[helloOffMagic+2]
	msg.Magic[3] = buf[helloOffMagic+3]

	if msg.Magic != Magic {
		return HelloMsg{}, fmt.Errorf("%w: got [0x%02X 0x%02X 0x%02X 0x%02X]",
			ErrHandshakeInvalidMagic, msg.Magic[0], msg.Magic[1], msg.Magic[2], msg.Magic[3])
	}

	// Version
	msg.Version = binary.LittleEndian.Uint16(buf[helloOffVersion:])

	// Name
	copy(msg.Name[:], buf[helloOffName:helloOffName+64])

	// Width
	msg.Width = binary.LittleEndian.Uint16(buf[helloOffWidth:])

	// Height
	msg.Height = binary.LittleEndian.Uint16(buf[helloOffHeight:])

	// PreferredFPS
	msg.PreferredFPS = binary.LittleEndian.Uint16(buf[helloOffPreferredFPS:])

	// Transport
	msg.Transport = TransportType(buf[helloOffTransport])

	// Reserved
	copy(msg.Reserved[:], buf[helloOffReserved:helloOffReserved+51])

	return msg, nil
}

// EncodeWelcome writes a WelcomeMsg into buf using little-endian byte order.
// buf must be at least HandshakeSize (128) bytes. Never allocates.
func EncodeWelcome(msg *WelcomeMsg, buf []byte) error {
	if len(buf) < HandshakeSize {
		return ErrHandshakeBufTooSmall
	}

	// Zero the buffer to ensure reserved bytes are clean.
	clear(buf[:HandshakeSize])

	// Magic
	buf[welcomeOffMagic] = msg.Magic[0]
	buf[welcomeOffMagic+1] = msg.Magic[1]
	buf[welcomeOffMagic+2] = msg.Magic[2]
	buf[welcomeOffMagic+3] = msg.Magic[3]

	// Version
	binary.LittleEndian.PutUint16(buf[welcomeOffVersion:], msg.Version)

	// ModuleID
	binary.LittleEndian.PutUint64(buf[welcomeOffModuleID:], msg.ModuleID)

	// Accepted
	buf[welcomeOffAccepted] = msg.Accepted

	// Transport
	buf[welcomeOffTransport] = uint8(msg.Transport)

	// MinVersion
	binary.LittleEndian.PutUint16(buf[welcomeOffMinVersion:], msg.MinVersion)

	// MaxVersion
	binary.LittleEndian.PutUint16(buf[welcomeOffMaxVersion:], msg.MaxVersion)

	// Reserved already zeroed by clear()

	return nil
}

// DecodeWelcome reads a WelcomeMsg from buf and validates the magic bytes.
// buf must be at least HandshakeSize (128) bytes. Never allocates.
func DecodeWelcome(buf []byte) (WelcomeMsg, error) {
	if len(buf) < HandshakeSize {
		return WelcomeMsg{}, ErrHandshakeBufTooSmall
	}

	var msg WelcomeMsg

	// Magic
	msg.Magic[0] = buf[welcomeOffMagic]
	msg.Magic[1] = buf[welcomeOffMagic+1]
	msg.Magic[2] = buf[welcomeOffMagic+2]
	msg.Magic[3] = buf[welcomeOffMagic+3]

	if msg.Magic != Magic {
		return WelcomeMsg{}, fmt.Errorf("%w: got [0x%02X 0x%02X 0x%02X 0x%02X]",
			ErrHandshakeInvalidMagic, msg.Magic[0], msg.Magic[1], msg.Magic[2], msg.Magic[3])
	}

	// Version
	msg.Version = binary.LittleEndian.Uint16(buf[welcomeOffVersion:])

	// ModuleID
	msg.ModuleID = binary.LittleEndian.Uint64(buf[welcomeOffModuleID:])

	// Accepted
	msg.Accepted = buf[welcomeOffAccepted]

	// Transport
	msg.Transport = TransportType(buf[welcomeOffTransport])

	// MinVersion
	msg.MinVersion = binary.LittleEndian.Uint16(buf[welcomeOffMinVersion:])

	// MaxVersion
	msg.MaxVersion = binary.LittleEndian.Uint16(buf[welcomeOffMaxVersion:])

	// Reserved
	copy(msg.Reserved[:], buf[welcomeOffReserved:welcomeOffReserved+108])

	return msg, nil
}

// SetName copies a string name into the HelloMsg's fixed-size Name field.
// The name is null-terminated. If name is longer than 63 bytes, it is truncated.
func SetName(msg *HelloMsg, name string) {
	// Clear the name field first.
	clear(msg.Name[:])

	// Copy up to 63 bytes (leaving room for null terminator).
	maxLen := len(msg.Name) - 1
	n := len(name)
	if n > maxLen {
		n = maxLen
	}
	copy(msg.Name[:n], name)
	// The field is already zero-filled, so null terminator is implicit.
}

// GetName reads the null-terminated string from the HelloMsg's Name field.
func GetName(msg *HelloMsg) string {
	for i, b := range msg.Name {
		if b == 0 {
			return string(msg.Name[:i])
		}
	}
	// No null terminator found — return all 64 bytes as string.
	return string(msg.Name[:])
}
