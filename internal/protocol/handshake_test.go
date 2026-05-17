package protocol

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestHandshakeSize(t *testing.T) {
	if HandshakeSize != 128 {
		t.Fatalf("HandshakeSize = %d, want 128", HandshakeSize)
	}
}

func TestHelloMsg_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  HelloMsg
	}{
		{
			name: "Typical",
			msg: func() HelloMsg {
				var m HelloMsg
				m.Magic = Magic
				m.Version = ProtocolVersion
				SetName(&m, "clock")
				m.Width = 400
				m.Height = 120
				m.PreferredFPS = 1
				m.Transport = TransportSocket
				return m
			}(),
		},
		{
			name: "AnimatedModule",
			msg: func() HelloMsg {
				var m HelloMsg
				m.Magic = Magic
				m.Version = ProtocolVersion
				SetName(&m, "notification-popup")
				m.Width = 1920
				m.Height = 1080
				m.PreferredFPS = 60
				m.Transport = TransportShm
				return m
			}(),
		},
		{
			name: "MaxValues",
			msg: func() HelloMsg {
				var m HelloMsg
				m.Magic = Magic
				m.Version = math.MaxUint16
				SetName(&m, strings.Repeat("x", 63))
				m.Width = math.MaxUint16
				m.Height = math.MaxUint16
				m.PreferredFPS = math.MaxUint16
				m.Transport = TransportType(0xFF)
				return m
			}(),
		},
		{
			name: "EmptyName",
			msg: func() HelloMsg {
				var m HelloMsg
				m.Magic = Magic
				m.Version = ProtocolVersion
				m.Width = 100
				m.Height = 100
				m.PreferredFPS = 30
				return m
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, HandshakeSize)
			if err := EncodeHello(&tt.msg, buf); err != nil {
				t.Fatalf("EncodeHello() error: %v", err)
			}

			got, err := DecodeHello(buf)
			if err != nil {
				t.Fatalf("DecodeHello() error: %v", err)
			}

			if got != tt.msg {
				t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, tt.msg)
			}
		})
	}
}

func TestWelcomeMsg_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  WelcomeMsg
	}{
		{
			name: "Accepted",
			msg: WelcomeMsg{
				Magic:      Magic,
				Version:    ProtocolVersion,
				ModuleID:   42,
				Accepted:   1,
				Transport:  TransportSocket,
				MinVersion: 1,
				MaxVersion: 1,
			},
		},
		{
			name: "Rejected",
			msg: WelcomeMsg{
				Magic:      Magic,
				Version:    ProtocolVersion,
				ModuleID:   0,
				Accepted:   0,
				Transport:  TransportSocket,
				MinVersion: 1,
				MaxVersion: 3,
			},
		},
		{
			name: "ShmGranted",
			msg: WelcomeMsg{
				Magic:      Magic,
				Version:    ProtocolVersion,
				ModuleID:   99,
				Accepted:   1,
				Transport:  TransportShm,
				MinVersion: 1,
				MaxVersion: 2,
			},
		},
		{
			name: "MaxValues",
			msg: WelcomeMsg{
				Magic:      Magic,
				Version:    math.MaxUint16,
				ModuleID:   math.MaxUint64,
				Accepted:   0xFF,
				Transport:  TransportType(0xFF),
				MinVersion: math.MaxUint16,
				MaxVersion: math.MaxUint16,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, HandshakeSize)
			if err := EncodeWelcome(&tt.msg, buf); err != nil {
				t.Fatalf("EncodeWelcome() error: %v", err)
			}

			got, err := DecodeWelcome(buf)
			if err != nil {
				t.Fatalf("DecodeWelcome() error: %v", err)
			}

			if got != tt.msg {
				t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, tt.msg)
			}
		})
	}
}

func TestEncodeHello_BufferTooSmall(t *testing.T) {
	msg := HelloMsg{Magic: Magic}
	tests := []struct {
		name string
		size int
	}{
		{"Zero", 0},
		{"Half", 64},
		{"AlmostFull", 127},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.size)
			err := EncodeHello(&msg, buf)
			if !errors.Is(err, ErrHandshakeBufTooSmall) {
				t.Errorf("EncodeHello() with %d-byte buf: got error %v, want ErrHandshakeBufTooSmall", tt.size, err)
			}
		})
	}
}

func TestDecodeHello_BufferTooSmall(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"Zero", 0},
		{"Half", 64},
		{"AlmostFull", 127},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.size)
			_, err := DecodeHello(buf)
			if !errors.Is(err, ErrHandshakeBufTooSmall) {
				t.Errorf("DecodeHello() with %d-byte buf: got error %v, want ErrHandshakeBufTooSmall", tt.size, err)
			}
		})
	}
}

func TestEncodeWelcome_BufferTooSmall(t *testing.T) {
	msg := WelcomeMsg{Magic: Magic}
	tests := []struct {
		name string
		size int
	}{
		{"Zero", 0},
		{"Half", 64},
		{"AlmostFull", 127},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.size)
			err := EncodeWelcome(&msg, buf)
			if !errors.Is(err, ErrHandshakeBufTooSmall) {
				t.Errorf("EncodeWelcome() with %d-byte buf: got error %v, want ErrHandshakeBufTooSmall", tt.size, err)
			}
		})
	}
}

func TestDecodeWelcome_BufferTooSmall(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"Zero", 0},
		{"Half", 64},
		{"AlmostFull", 127},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.size)
			_, err := DecodeWelcome(buf)
			if !errors.Is(err, ErrHandshakeBufTooSmall) {
				t.Errorf("DecodeWelcome() with %d-byte buf: got error %v, want ErrHandshakeBufTooSmall", tt.size, err)
			}
		})
	}
}

func TestDecodeHello_InvalidMagic(t *testing.T) {
	tests := []struct {
		name  string
		magic [4]byte
	}{
		{"AllZeros", [4]byte{0, 0, 0, 0}},
		{"WrongFirst", [4]byte{0x00, 0x4F, 0x4D, 0x50}},
		{"Reversed", [4]byte{0x50, 0x4D, 0x4F, 0x43}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, HandshakeSize)
			buf[0] = tt.magic[0]
			buf[1] = tt.magic[1]
			buf[2] = tt.magic[2]
			buf[3] = tt.magic[3]

			_, err := DecodeHello(buf)
			if err == nil {
				t.Error("DecodeHello() with invalid magic: expected error, got nil")
			}
		})
	}
}

func TestDecodeWelcome_InvalidMagic(t *testing.T) {
	tests := []struct {
		name  string
		magic [4]byte
	}{
		{"AllZeros", [4]byte{0, 0, 0, 0}},
		{"WrongLast", [4]byte{0x43, 0x4F, 0x4D, 0x00}},
		{"AllOnes", [4]byte{0xFF, 0xFF, 0xFF, 0xFF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, HandshakeSize)
			buf[0] = tt.magic[0]
			buf[1] = tt.magic[1]
			buf[2] = tt.magic[2]
			buf[3] = tt.magic[3]

			_, err := DecodeWelcome(buf)
			if err == nil {
				t.Error("DecodeWelcome() with invalid magic: expected error, got nil")
			}
		})
	}
}

func TestSetName_GetName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Short", "clock", "clock"},
		{"Empty", "", ""},
		{"ExactMax", strings.Repeat("a", 63), strings.Repeat("a", 63)},
		{"Overflow", strings.Repeat("b", 100), strings.Repeat("b", 63)},
		{"Unicode", "часы", "часы"},
		{"WithSpaces", "my module name", "my module name"},
		{"SingleChar", "x", "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg HelloMsg
			SetName(&msg, tt.input)
			got := GetName(&msg)
			if got != tt.want {
				t.Errorf("SetName/GetName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetName_NullTerminated(t *testing.T) {
	var msg HelloMsg
	SetName(&msg, "test")

	// Verify null terminator exists at position 4.
	if msg.Name[4] != 0 {
		t.Errorf("Name[4] = 0x%02X, want 0x00 (null terminator)", msg.Name[4])
	}
	// Verify rest is zeroed.
	for i := 5; i < len(msg.Name); i++ {
		if msg.Name[i] != 0 {
			t.Errorf("Name[%d] = 0x%02X, want 0x00", i, msg.Name[i])
		}
	}
}

func TestSetName_ClearsOldName(t *testing.T) {
	var msg HelloMsg
	SetName(&msg, strings.Repeat("x", 63))
	SetName(&msg, "ab")

	got := GetName(&msg)
	if got != "ab" {
		t.Errorf("after overwrite, GetName() = %q, want %q", got, "ab")
	}
	// Verify bytes after "ab" are zero.
	if msg.Name[2] != 0 {
		t.Errorf("Name[2] = 0x%02X, want 0x00 after overwrite", msg.Name[2])
	}
}

func TestGetName_NoNullTerminator(t *testing.T) {
	// Edge case: all 64 bytes are non-zero (no null terminator).
	var msg HelloMsg
	for i := range msg.Name {
		msg.Name[i] = 'z'
	}

	got := GetName(&msg)
	if len(got) != 64 {
		t.Errorf("GetName() with no NUL: len = %d, want 64", len(got))
	}
}

func TestTransportType_String(t *testing.T) {
	tests := []struct {
		name string
		tr   TransportType
		want string
	}{
		{"Socket", TransportSocket, "Socket"},
		{"Shm", TransportShm, "SharedMemory"},
		{"Unknown", TransportType(99), "TransportType(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tr.String()
			if got != tt.want {
				t.Errorf("TransportType(%d).String() = %q, want %q", tt.tr, got, tt.want)
			}
		})
	}
}

func TestEncodeHello_ReservedZeroed(t *testing.T) {
	msg := HelloMsg{
		Magic:   Magic,
		Version: ProtocolVersion,
	}
	SetName(&msg, "test")

	buf := make([]byte, HandshakeSize)
	// Fill with non-zero.
	for i := range buf {
		buf[i] = 0xFF
	}

	if err := EncodeHello(&msg, buf); err != nil {
		t.Fatalf("EncodeHello() error: %v", err)
	}

	// Verify reserved bytes are zeroed.
	for i := helloOffReserved; i < helloOffReserved+51; i++ {
		if buf[i] != 0 {
			t.Errorf("reserved byte buf[%d] = 0x%02X, want 0x00", i, buf[i])
		}
	}
}

func TestEncodeWelcome_ReservedZeroed(t *testing.T) {
	msg := WelcomeMsg{
		Magic:    Magic,
		Version:  ProtocolVersion,
		ModuleID: 1,
		Accepted: 1,
	}

	buf := make([]byte, HandshakeSize)
	// Fill with non-zero.
	for i := range buf {
		buf[i] = 0xFF
	}

	if err := EncodeWelcome(&msg, buf); err != nil {
		t.Fatalf("EncodeWelcome() error: %v", err)
	}

	// Verify reserved bytes are zeroed.
	for i := welcomeOffReserved; i < welcomeOffReserved+108; i++ {
		if buf[i] != 0 {
			t.Errorf("reserved byte buf[%d] = 0x%02X, want 0x00", i, buf[i])
		}
	}
}

func TestHelloMsg_LargerBuffer(t *testing.T) {
	msg := HelloMsg{
		Magic:   Magic,
		Version: ProtocolVersion,
	}
	SetName(&msg, "test")

	buf := make([]byte, 256)
	for i := range buf {
		buf[i] = 0xAA
	}

	if err := EncodeHello(&msg, buf); err != nil {
		t.Fatalf("EncodeHello() error: %v", err)
	}

	// Bytes beyond HandshakeSize should be untouched.
	for i := HandshakeSize; i < len(buf); i++ {
		if buf[i] != 0xAA {
			t.Errorf("buf[%d] = 0x%02X, want 0xAA (sentinel should be untouched)", i, buf[i])
		}
	}
}

func TestWelcomeMsg_LargerBuffer(t *testing.T) {
	msg := WelcomeMsg{
		Magic:    Magic,
		Version:  ProtocolVersion,
		ModuleID: 7,
		Accepted: 1,
	}

	buf := make([]byte, 256)
	for i := range buf {
		buf[i] = 0xBB
	}

	if err := EncodeWelcome(&msg, buf); err != nil {
		t.Fatalf("EncodeWelcome() error: %v", err)
	}

	// Bytes beyond HandshakeSize should be untouched.
	for i := HandshakeSize; i < len(buf); i++ {
		if buf[i] != 0xBB {
			t.Errorf("buf[%d] = 0x%02X, want 0xBB (sentinel should be untouched)", i, buf[i])
		}
	}
}

func BenchmarkEncodeHello(b *testing.B) {
	msg := HelloMsg{
		Magic:        Magic,
		Version:      ProtocolVersion,
		Width:        400,
		Height:       120,
		PreferredFPS: 60,
		Transport:    TransportSocket,
	}
	SetName(&msg, "benchmark-module")
	buf := make([]byte, HandshakeSize)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = EncodeHello(&msg, buf)
	}
}

func BenchmarkDecodeHello(b *testing.B) {
	msg := HelloMsg{
		Magic:        Magic,
		Version:      ProtocolVersion,
		Width:        400,
		Height:       120,
		PreferredFPS: 60,
		Transport:    TransportSocket,
	}
	SetName(&msg, "benchmark-module")
	buf := make([]byte, HandshakeSize)
	_ = EncodeHello(&msg, buf)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = DecodeHello(buf)
	}
}

func BenchmarkEncodeWelcome(b *testing.B) {
	msg := WelcomeMsg{
		Magic:      Magic,
		Version:    ProtocolVersion,
		ModuleID:   42,
		Accepted:   1,
		Transport:  TransportSocket,
		MinVersion: 1,
		MaxVersion: 1,
	}
	buf := make([]byte, HandshakeSize)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = EncodeWelcome(&msg, buf)
	}
}

func BenchmarkDecodeWelcome(b *testing.B) {
	msg := WelcomeMsg{
		Magic:      Magic,
		Version:    ProtocolVersion,
		ModuleID:   42,
		Accepted:   1,
		Transport:  TransportSocket,
		MinVersion: 1,
		MaxVersion: 1,
	}
	buf := make([]byte, HandshakeSize)
	_ = EncodeWelcome(&msg, buf)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = DecodeWelcome(buf)
	}
}
