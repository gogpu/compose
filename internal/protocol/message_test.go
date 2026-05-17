package protocol

import (
	"testing"
)

func TestMsgType_String(t *testing.T) {
	tests := []struct {
		name string
		m    MsgType
		want string
	}{
		{"Frame", MsgFrame, "Frame"},
		{"Handshake", MsgHandshake, "Handshake"},
		{"Ack", MsgAck, "Ack"},
		{"FrameRequest", MsgFrameRequest, "FrameRequest"},
		{"Resize", MsgResize, "Resize"},
		{"Disconnect", MsgDisconnect, "Disconnect"},
		{"Unknown_0xFF", MsgType(0xFF), "MsgType(0xFF)"},
		{"Unknown_0x00", MsgType(0x00), "MsgType(0x00)"},
		{"Unknown_0x07", MsgType(0x07), "MsgType(0x07)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.String()
			if got != tt.want {
				t.Errorf("MsgType(%d).String() = %q, want %q", tt.m, got, tt.want)
			}
		})
	}
}

func TestMsgType_Valid(t *testing.T) {
	tests := []struct {
		name string
		m    MsgType
		want bool
	}{
		{"Frame", MsgFrame, true},
		{"Handshake", MsgHandshake, true},
		{"Ack", MsgAck, true},
		{"FrameRequest", MsgFrameRequest, true},
		{"Resize", MsgResize, true},
		{"Disconnect", MsgDisconnect, true},
		{"Zero", MsgType(0x00), false},
		{"Unknown_0x07", MsgType(0x07), false},
		{"Unknown_0xFF", MsgType(0xFF), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.Valid()
			if got != tt.want {
				t.Errorf("MsgType(%d).Valid() = %v, want %v", tt.m, got, tt.want)
			}
		})
	}
}

func TestFlag_Operations(t *testing.T) {
	t.Run("Has", func(t *testing.T) {
		flags := FlagDirtyValid | FlagKeyframe
		if !Flag(flags).Has(FlagDirtyValid) {
			t.Error("expected FlagDirtyValid to be set")
		}
		if !Flag(flags).Has(FlagKeyframe) {
			t.Error("expected FlagKeyframe to be set")
		}
		if Flag(flags).Has(FlagCompressed) {
			t.Error("expected FlagCompressed to not be set")
		}
	})

	t.Run("Set", func(t *testing.T) {
		var f Flag
		f = f.Set(FlagCompressed)
		if !f.Has(FlagCompressed) {
			t.Error("expected FlagCompressed after Set")
		}
		f = f.Set(FlagDirtyValid)
		if !f.Has(FlagDirtyValid) {
			t.Error("expected FlagDirtyValid after Set")
		}
		// Setting again is idempotent.
		f = f.Set(FlagCompressed)
		if f != FlagCompressed|FlagDirtyValid {
			t.Errorf("expected 0x03, got 0x%02X", f)
		}
	})

	t.Run("Clear", func(t *testing.T) {
		f := FlagDirtyValid | FlagCompressed | FlagKeyframe
		f = f.Clear(FlagCompressed)
		if f.Has(FlagCompressed) {
			t.Error("expected FlagCompressed to be cleared")
		}
		if !f.Has(FlagDirtyValid) {
			t.Error("expected FlagDirtyValid to still be set")
		}
		if !f.Has(FlagKeyframe) {
			t.Error("expected FlagKeyframe to still be set")
		}
	})

	t.Run("ZeroHasNothing", func(t *testing.T) {
		var f Flag
		if f.Has(FlagDirtyValid) || f.Has(FlagCompressed) || f.Has(FlagKeyframe) {
			t.Error("zero Flag should have no flags set")
		}
	})
}

func TestFlag_String(t *testing.T) {
	tests := []struct {
		name string
		f    Flag
		want string
	}{
		{"DirtyValid", FlagDirtyValid, "DirtyValid"},
		{"Compressed", FlagCompressed, "Compressed"},
		{"Keyframe", FlagKeyframe, "Keyframe"},
		{"Unknown", Flag(0x10), "Flag(0x10)"},
		{"Combined", FlagDirtyValid | FlagCompressed, "Flag(0x03)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.String()
			if got != tt.want {
				t.Errorf("Flag(0x%02X).String() = %q, want %q", tt.f, got, tt.want)
			}
		})
	}
}

func TestPixelFormat_String(t *testing.T) {
	tests := []struct {
		name string
		p    PixelFormat
		want string
	}{
		{"RGBA8", PixelRGBA8, "RGBA8"},
		{"BGRA8", PixelBGRA8, "BGRA8"},
		{"Unknown", PixelFormat(0x99), "PixelFormat(0x99)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.String()
			if got != tt.want {
				t.Errorf("PixelFormat(0x%02X).String() = %q, want %q", tt.p, got, tt.want)
			}
		})
	}
}

func TestPixelFormat_Valid(t *testing.T) {
	tests := []struct {
		name string
		p    PixelFormat
		want bool
	}{
		{"RGBA8", PixelRGBA8, true},
		{"BGRA8", PixelBGRA8, true},
		{"Unknown", PixelFormat(0x02), false},
		{"Max", PixelFormat(0xFF), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Valid()
			if got != tt.want {
				t.Errorf("PixelFormat(0x%02X).Valid() = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}

func TestCompression_String(t *testing.T) {
	tests := []struct {
		name string
		c    Compression
		want string
	}{
		{"None", CompressionNone, "None"},
		{"LZ4", CompressionLZ4, "LZ4"},
		{"Zstd", CompressionZstd, "Zstd"},
		{"Unknown", Compression(0xAB), "Compression(0xAB)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c.String()
			if got != tt.want {
				t.Errorf("Compression(0x%02X).String() = %q, want %q", tt.c, got, tt.want)
			}
		})
	}
}

func TestCompression_Valid(t *testing.T) {
	tests := []struct {
		name string
		c    Compression
		want bool
	}{
		{"None", CompressionNone, true},
		{"LZ4", CompressionLZ4, true},
		{"Zstd", CompressionZstd, true},
		{"Unknown_0x03", Compression(0x03), false},
		{"Unknown_0xFF", Compression(0xFF), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c.Valid()
			if got != tt.want {
				t.Errorf("Compression(0x%02X).Valid() = %v, want %v", tt.c, got, tt.want)
			}
		})
	}
}

func TestFlagValues(t *testing.T) {
	// Verify flag bit positions match the spec.
	if FlagDirtyValid != 0x01 {
		t.Errorf("FlagDirtyValid = 0x%02X, want 0x01", FlagDirtyValid)
	}
	if FlagCompressed != 0x02 {
		t.Errorf("FlagCompressed = 0x%02X, want 0x02", FlagCompressed)
	}
	if FlagKeyframe != 0x04 {
		t.Errorf("FlagKeyframe = 0x%02X, want 0x04", FlagKeyframe)
	}
}

func TestMsgTypeValues(t *testing.T) {
	// Verify message type values match the spec.
	if MsgFrame != 0x01 {
		t.Errorf("MsgFrame = 0x%02X, want 0x01", MsgFrame)
	}
	if MsgHandshake != 0x02 {
		t.Errorf("MsgHandshake = 0x%02X, want 0x02", MsgHandshake)
	}
	if MsgAck != 0x03 {
		t.Errorf("MsgAck = 0x%02X, want 0x03", MsgAck)
	}
	if MsgFrameRequest != 0x04 {
		t.Errorf("MsgFrameRequest = 0x%02X, want 0x04", MsgFrameRequest)
	}
	if MsgResize != 0x05 {
		t.Errorf("MsgResize = 0x%02X, want 0x05", MsgResize)
	}
	if MsgDisconnect != 0x06 {
		t.Errorf("MsgDisconnect = 0x%02X, want 0x06", MsgDisconnect)
	}
}

func TestPixelFormatValues(t *testing.T) {
	if PixelRGBA8 != 0x00 {
		t.Errorf("PixelRGBA8 = 0x%02X, want 0x00", PixelRGBA8)
	}
	if PixelBGRA8 != 0x01 {
		t.Errorf("PixelBGRA8 = 0x%02X, want 0x01", PixelBGRA8)
	}
}

func TestCompressionValues(t *testing.T) {
	if CompressionNone != 0x00 {
		t.Errorf("CompressionNone = 0x%02X, want 0x00", CompressionNone)
	}
	if CompressionLZ4 != 0x01 {
		t.Errorf("CompressionLZ4 = 0x%02X, want 0x01", CompressionLZ4)
	}
	if CompressionZstd != 0x02 {
		t.Errorf("CompressionZstd = 0x%02X, want 0x02", CompressionZstd)
	}
}
