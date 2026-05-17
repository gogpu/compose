package codec

import (
	"bytes"
	"testing"
)

func TestRawRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"single byte", []byte{0x42}},
		{"small", []byte("hello world")},
		{"zeros", make([]byte, 1024)},
		{"frame 400x120x4", makeGUIPixels(400, 120)},
	}

	c := Raw()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.data

			// Encode with pre-allocated buffer.
			encBuf := make([]byte, c.MaxEncodedSize(len(src)))
			encoded, err := c.Encode(encBuf, src)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			// Raw codec output should equal input.
			if !bytes.Equal(encoded, src) {
				t.Errorf("Encode output differs from input: got %d bytes, want %d", len(encoded), len(src))
			}

			// Decode with pre-allocated buffer.
			decBuf := make([]byte, len(src))
			decoded, err := c.Decode(decBuf, encoded)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if !bytes.Equal(decoded, src) {
				t.Errorf("round-trip mismatch: got %d bytes, want %d", len(decoded), len(src))
			}
		})
	}
}

func TestRawEncodeNilDst(t *testing.T) {
	c := Raw()
	src := []byte("test data")

	// nil dst forces allocation (slow path).
	encoded, err := c.Encode(nil, src)
	if err != nil {
		t.Fatalf("Encode(nil, src): %v", err)
	}
	if !bytes.Equal(encoded, src) {
		t.Error("Encode with nil dst: output differs from input")
	}
}

func TestRawDecodeNilDst(t *testing.T) {
	c := Raw()
	src := []byte("test data")

	decoded, err := c.Decode(nil, src)
	if err != nil {
		t.Fatalf("Decode(nil, src): %v", err)
	}
	if !bytes.Equal(decoded, src) {
		t.Error("Decode with nil dst: output differs from input")
	}
}

func TestRawEncodeEmptyInput(t *testing.T) {
	c := Raw()
	dst := make([]byte, 16)

	encoded, err := c.Encode(dst, nil)
	if err != nil {
		t.Fatalf("Encode(dst, nil): %v", err)
	}
	if len(encoded) != 0 {
		t.Errorf("Encode(dst, nil) len = %d, want 0", len(encoded))
	}
}

func TestRawDecodeEmptyInput(t *testing.T) {
	c := Raw()
	dst := make([]byte, 16)

	decoded, err := c.Decode(dst, nil)
	if err != nil {
		t.Fatalf("Decode(dst, nil): %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("Decode(dst, nil) len = %d, want 0", len(decoded))
	}
}

func TestRawEncodeSmallDst(t *testing.T) {
	c := Raw()
	src := []byte("longer test data that exceeds small buffer")
	dst := make([]byte, 4) // too small

	encoded, err := c.Encode(dst, src)
	if err != nil {
		t.Fatalf("Encode with small dst: %v", err)
	}
	if !bytes.Equal(encoded, src) {
		t.Error("Encode with small dst: output differs from input")
	}
}

func TestRawID(t *testing.T) {
	c := Raw()
	if c.ID() != IDRaw {
		t.Errorf("ID() = 0x%02x, want 0x%02x", c.ID(), IDRaw)
	}
}

func TestRawMaxEncodedSize(t *testing.T) {
	c := Raw()
	tests := []struct {
		srcLen int
		want   int
	}{
		{0, 0},
		{1, 1},
		{1024, 1024},
		{192000, 192000}, // 400*120*4
	}
	for _, tt := range tests {
		got := c.MaxEncodedSize(tt.srcLen)
		if got != tt.want {
			t.Errorf("MaxEncodedSize(%d) = %d, want %d", tt.srcLen, got, tt.want)
		}
	}
}

func TestRawEncodeLargeFrame(t *testing.T) {
	c := Raw()
	// Simulate a 1920x1080 RGBA frame (8.3 MB).
	src := make([]byte, 1920*1080*4)
	for i := range src {
		src[i] = byte(i % 256)
	}

	dst := make([]byte, c.MaxEncodedSize(len(src)))
	encoded, err := c.Encode(dst, src)
	if err != nil {
		t.Fatalf("Encode large frame: %v", err)
	}
	if !bytes.Equal(encoded, src) {
		t.Error("large frame encode: output differs from input")
	}
}

func TestRawConcurrentSafety(t *testing.T) {
	c := Raw()
	src := makeGUIPixels(400, 120)
	done := make(chan struct{})

	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			dst := make([]byte, c.MaxEncodedSize(len(src)))
			for j := 0; j < 100; j++ {
				encoded, err := c.Encode(dst, src)
				if err != nil {
					t.Errorf("concurrent Encode: %v", err)
					return
				}
				if !bytes.Equal(encoded, src) {
					t.Errorf("concurrent Encode: mismatch")
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

// BenchmarkRawEncode benchmarks raw codec encoding with a realistic GUI frame.
func BenchmarkRawEncode(b *testing.B) {
	c := Raw()
	src := makeGUIPixels(400, 120)
	dst := make([]byte, c.MaxEncodedSize(len(src)))

	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for b.Loop() {
		_, _ = c.Encode(dst, src)
	}
}

// BenchmarkRawDecode benchmarks raw codec decoding with a realistic GUI frame.
func BenchmarkRawDecode(b *testing.B) {
	c := Raw()
	src := makeGUIPixels(400, 120)
	dst := make([]byte, len(src))

	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for b.Loop() {
		_, _ = c.Decode(dst, src)
	}
}

// makeGUIPixels generates synthetic GUI-like pixel data with large flat color
// regions, simulating a typical desktop widget frame (good for compression).
func makeGUIPixels(width, height int) []byte {
	size := width * height * 4
	pixels := make([]byte, size)

	// Background: solid light gray (70% of frame).
	bgEnd := int(float64(height) * 0.7)
	for y := 0; y < bgEnd; y++ {
		for x := 0; x < width; x++ {
			off := (y*width + x) * 4
			pixels[off+0] = 0xF0 // R
			pixels[off+1] = 0xF0 // G
			pixels[off+2] = 0xF0 // B
			pixels[off+3] = 0xFF // A
		}
	}

	// Button region: solid blue.
	for y := bgEnd; y < bgEnd+30 && y < height; y++ {
		for x := 20; x < 120 && x < width; x++ {
			off := (y*width + x) * 4
			pixels[off+0] = 0x21 // R
			pixels[off+1] = 0x96 // G
			pixels[off+2] = 0xF3 // B
			pixels[off+3] = 0xFF // A
		}
	}

	// Footer: solid dark gray.
	for y := bgEnd + 30; y < height; y++ {
		for x := 0; x < width; x++ {
			off := (y*width + x) * 4
			pixels[off+0] = 0x30 // R
			pixels[off+1] = 0x30 // G
			pixels[off+2] = 0x30 // B
			pixels[off+3] = 0xFF // A
		}
	}

	return pixels
}
