package protocol

import (
	"errors"
	"math"
	"testing"
)

func TestHeaderSize(t *testing.T) {
	if HeaderSize != 64 {
		t.Fatalf("HeaderSize = %d, want 64", HeaderSize)
	}
}

func TestHeader_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		h    Header
	}{
		{
			name: "TypicalFrame",
			h: Header{
				Magic:            Magic,
				Version:          ProtocolVersion,
				MsgType:          MsgFrame,
				Flags:            FlagDirtyValid | FlagKeyframe,
				ModuleID:         42,
				Sequence:         1001,
				TimestampNs:      1_000_000_000,
				Width:            1920,
				Height:           1080,
				Stride:           1920 * 4,
				DirtyX:           100,
				DirtyY:           200,
				DirtyW:           300,
				DirtyH:           400,
				PixelFormat:      PixelRGBA8,
				Compression:      CompressionNone,
				PayloadSize:      1920 * 1080 * 4,
				UncompressedSize: 1920 * 1080 * 4,
			},
		},
		{
			name: "CompressedFrame",
			h: Header{
				Magic:            Magic,
				Version:          ProtocolVersion,
				MsgType:          MsgFrame,
				Flags:            FlagCompressed,
				ModuleID:         7,
				Sequence:         55,
				TimestampNs:      -12345678, // negative timestamp is valid
				Width:            800,
				Height:           600,
				Stride:           800 * 4,
				PixelFormat:      PixelBGRA8,
				Compression:      CompressionLZ4,
				PayloadSize:      100000,
				UncompressedSize: 800 * 600 * 4,
			},
		},
		{
			name: "Ack",
			h: Header{
				Magic:   Magic,
				Version: ProtocolVersion,
				MsgType: MsgAck,
			},
		},
		{
			name: "FrameRequest",
			h: Header{
				Magic:    Magic,
				Version:  ProtocolVersion,
				MsgType:  MsgFrameRequest,
				ModuleID: 99,
				Sequence: 12345,
			},
		},
		{
			name: "Disconnect",
			h: Header{
				Magic:    Magic,
				Version:  ProtocolVersion,
				MsgType:  MsgDisconnect,
				ModuleID: 3,
			},
		},
		{
			name: "Resize",
			h: Header{
				Magic:   Magic,
				Version: ProtocolVersion,
				MsgType: MsgResize,
				Width:   2560,
				Height:  1440,
				Stride:  2560 * 4,
			},
		},
		{
			name: "MaxValues",
			h: Header{
				Magic:            Magic,
				Version:          math.MaxUint16,
				MsgType:          MsgFrame,
				Flags:            Flag(0xFF),
				ModuleID:         math.MaxUint64,
				Sequence:         math.MaxUint64,
				TimestampNs:      math.MaxInt64,
				Width:            math.MaxUint16,
				Height:           math.MaxUint16,
				Stride:           math.MaxUint32,
				DirtyX:           math.MaxUint16,
				DirtyY:           math.MaxUint16,
				DirtyW:           math.MaxUint16,
				DirtyH:           math.MaxUint16,
				PixelFormat:      PixelFormat(0xFF),
				Compression:      Compression(0xFF),
				PayloadSize:      math.MaxUint32,
				UncompressedSize: math.MaxUint32,
			},
		},
		{
			name: "ZeroValue",
			h: Header{
				Magic:   Magic,
				MsgType: MsgFrame,
			},
		},
		{
			name: "NegativeTimestamp",
			h: Header{
				Magic:       Magic,
				Version:     ProtocolVersion,
				MsgType:     MsgHandshake,
				TimestampNs: math.MinInt64,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, HeaderSize)
			if err := Encode(&tt.h, buf); err != nil {
				t.Fatalf("Encode() error: %v", err)
			}

			got, err := Decode(buf)
			if err != nil {
				t.Fatalf("Decode() error: %v", err)
			}

			if got != tt.h {
				t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, tt.h)
			}
		})
	}
}

func TestEncode_BufferTooSmall(t *testing.T) {
	h := Header{Magic: Magic, MsgType: MsgFrame}
	tests := []struct {
		name string
		size int
	}{
		{"Zero", 0},
		{"One", 1},
		{"Half", 32},
		{"AlmostFull", 63},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.size)
			err := Encode(&h, buf)
			if !errors.Is(err, ErrBufferTooSmall) {
				t.Errorf("Encode() with %d-byte buf: got error %v, want ErrBufferTooSmall", tt.size, err)
			}
		})
	}
}

func TestDecode_BufferTooSmall(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"Zero", 0},
		{"One", 1},
		{"Half", 32},
		{"AlmostFull", 63},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.size)
			_, err := Decode(buf)
			if !errors.Is(err, ErrBufferTooSmall) {
				t.Errorf("Decode() with %d-byte buf: got error %v, want ErrBufferTooSmall", tt.size, err)
			}
		})
	}
}

func TestDecode_InvalidMagic(t *testing.T) {
	tests := []struct {
		name  string
		magic [4]byte
	}{
		{"AllZeros", [4]byte{0, 0, 0, 0}},
		{"WrongFirst", [4]byte{0x00, 0x4F, 0x4D, 0x50}},
		{"WrongLast", [4]byte{0x43, 0x4F, 0x4D, 0x00}},
		{"Reversed", [4]byte{0x50, 0x4D, 0x4F, 0x43}},
		{"AllOnes", [4]byte{0xFF, 0xFF, 0xFF, 0xFF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, HeaderSize)
			// Manually write magic to bypass Encode validation.
			buf[0] = tt.magic[0]
			buf[1] = tt.magic[1]
			buf[2] = tt.magic[2]
			buf[3] = tt.magic[3]
			buf[offMsgType] = uint8(MsgFrame)

			_, err := Decode(buf)
			if err == nil {
				t.Errorf("Decode() with magic %v: expected error, got nil", tt.magic)
			}
		})
	}
}

func TestDecode_UnknownMsgType(t *testing.T) {
	tests := []struct {
		name    string
		msgType uint8
	}{
		{"Zero", 0x00},
		{"Seven", 0x07},
		{"Max", 0xFF},
		{"JustAbove", 0x08},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, HeaderSize)
			// Valid magic.
			buf[0] = Magic[0]
			buf[1] = Magic[1]
			buf[2] = Magic[2]
			buf[3] = Magic[3]
			buf[offMsgType] = tt.msgType

			_, err := Decode(buf)
			if err == nil {
				t.Errorf("Decode() with MsgType 0x%02X: expected error, got nil", tt.msgType)
			}
		})
	}
}

func TestEncode_LargerBuffer(t *testing.T) {
	// Encode into a buffer larger than 64 bytes should work and not corrupt beyond.
	h := Header{
		Magic:   Magic,
		Version: ProtocolVersion,
		MsgType: MsgFrame,
	}
	buf := make([]byte, 128)
	for i := range buf {
		buf[i] = 0xAA // sentinel
	}

	if err := Encode(&h, buf); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	// Bytes beyond 64 should be untouched.
	for i := HeaderSize; i < len(buf); i++ {
		if buf[i] != 0xAA {
			t.Errorf("buf[%d] = 0x%02X, want 0xAA (sentinel should be untouched)", i, buf[i])
		}
	}
}

func TestDecode_ExactBuffer(t *testing.T) {
	// Decode from a buffer that is exactly 64 bytes.
	h := Header{
		Magic:       Magic,
		Version:     ProtocolVersion,
		MsgType:     MsgFrame,
		ModuleID:    42,
		PayloadSize: 1024,
	}
	buf := make([]byte, HeaderSize)
	if err := Encode(&h, buf); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	got, err := Decode(buf)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	if got.ModuleID != 42 {
		t.Errorf("ModuleID = %d, want 42", got.ModuleID)
	}
	if got.PayloadSize != 1024 {
		t.Errorf("PayloadSize = %d, want 1024", got.PayloadSize)
	}
}

func TestHeader_ReservedZeroed(t *testing.T) {
	// After encoding, reserved bytes should be zero.
	h := Header{
		Magic:   Magic,
		Version: ProtocolVersion,
		MsgType: MsgFrame,
	}
	buf := make([]byte, HeaderSize)
	// Fill with non-zero to prove Encode zeros them.
	for i := range buf {
		buf[i] = 0xFF
	}

	if err := Encode(&h, buf); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	for i := offReserved; i < offReserved+6; i++ {
		if buf[i] != 0 {
			t.Errorf("reserved byte buf[%d] = 0x%02X, want 0x00", i, buf[i])
		}
	}
}

func TestMagicBytes(t *testing.T) {
	// Verify magic is "COMP" in ASCII.
	if Magic != [4]byte{'C', 'O', 'M', 'P'} {
		t.Errorf("Magic = %v, want {0x43, 0x4F, 0x4D, 0x50} ('COMP')", Magic)
	}
}

func BenchmarkHeaderEncode(b *testing.B) {
	h := Header{
		Magic:            Magic,
		Version:          ProtocolVersion,
		MsgType:          MsgFrame,
		Flags:            FlagDirtyValid | FlagCompressed,
		ModuleID:         42,
		Sequence:         1001,
		TimestampNs:      1_000_000_000,
		Width:            1920,
		Height:           1080,
		Stride:           1920 * 4,
		DirtyX:           100,
		DirtyY:           200,
		DirtyW:           300,
		DirtyH:           400,
		PixelFormat:      PixelRGBA8,
		Compression:      CompressionLZ4,
		PayloadSize:      100000,
		UncompressedSize: 1920 * 1080 * 4,
	}
	buf := make([]byte, HeaderSize)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Encode(&h, buf)
	}
}

func BenchmarkHeaderDecode(b *testing.B) {
	h := Header{
		Magic:            Magic,
		Version:          ProtocolVersion,
		MsgType:          MsgFrame,
		Flags:            FlagDirtyValid | FlagCompressed,
		ModuleID:         42,
		Sequence:         1001,
		TimestampNs:      1_000_000_000,
		Width:            1920,
		Height:           1080,
		Stride:           1920 * 4,
		DirtyX:           100,
		DirtyY:           200,
		DirtyW:           300,
		DirtyH:           400,
		PixelFormat:      PixelRGBA8,
		Compression:      CompressionLZ4,
		PayloadSize:      100000,
		UncompressedSize: 1920 * 1080 * 4,
	}
	buf := make([]byte, HeaderSize)
	_ = Encode(&h, buf)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Decode(buf)
	}
}
