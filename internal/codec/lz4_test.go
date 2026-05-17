package codec

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestLZ4RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"single byte", []byte{0x42}},
		{"small text", []byte("hello world hello world hello world")},
		{"zeros 1KB", make([]byte, 1024)},
		{"gui frame 400x120", makeGUIPixels(400, 120)},
		{"repeated pattern", bytes.Repeat([]byte{0xAA, 0xBB, 0xCC, 0xDD}, 4096)},
	}

	c := LZ4()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.data

			// Encode with pre-allocated buffer.
			encBuf := make([]byte, c.MaxEncodedSize(len(src)))
			encoded, err := c.Encode(encBuf, src)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			// Decode with pre-allocated buffer.
			decBuf := make([]byte, len(src))
			decoded, err := c.Decode(decBuf, encoded)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if !bytes.Equal(decoded, src) {
				t.Errorf("round-trip mismatch: decoded %d bytes, want %d", len(decoded), len(src))
			}
		})
	}
}

func TestLZ4EmptyInput(t *testing.T) {
	c := LZ4()

	// Encode nil input.
	encoded, err := c.Encode(nil, nil)
	if err != nil {
		t.Fatalf("Encode(nil, nil): %v", err)
	}
	if encoded != nil {
		t.Errorf("Encode(nil, nil) = %v, want nil", encoded)
	}

	// Encode empty slice with dst.
	dst := make([]byte, 16)
	encoded, err = c.Encode(dst, []byte{})
	if err != nil {
		t.Fatalf("Encode(dst, []byte{}): %v", err)
	}
	if len(encoded) != 0 {
		t.Errorf("Encode empty: len = %d, want 0", len(encoded))
	}

	// Decode nil input.
	decoded, err := c.Decode(nil, nil)
	if err != nil {
		t.Fatalf("Decode(nil, nil): %v", err)
	}
	if decoded != nil {
		t.Errorf("Decode(nil, nil) = %v, want nil", decoded)
	}

	// Decode empty slice with dst.
	decoded, err = c.Decode(dst, []byte{})
	if err != nil {
		t.Fatalf("Decode(dst, []byte{}): %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("Decode empty: len = %d, want 0", len(decoded))
	}
}

func TestLZ4CompressionRatio(t *testing.T) {
	c := LZ4()

	// GUI pixel data should compress well (large flat color regions).
	src := makeGUIPixels(400, 120)
	encBuf := make([]byte, c.MaxEncodedSize(len(src)))
	encoded, err := c.Encode(encBuf, src)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	ratio := float64(len(encoded)) / float64(len(src))
	t.Logf("GUI pixels: %d -> %d bytes (ratio: %.3f, savings: %.1f%%)",
		len(src), len(encoded), ratio, (1-ratio)*100)

	// GUI pixel data with large flat regions should compress to at least 50%.
	if ratio > 0.50 {
		t.Errorf("compression ratio %.3f is worse than expected 0.50 for GUI data", ratio)
	}
}

func TestLZ4RandomData(t *testing.T) {
	c := LZ4()

	// Random data compresses poorly but must still round-trip correctly.
	src := make([]byte, 4096)
	if _, err := rand.Read(src); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	encBuf := make([]byte, c.MaxEncodedSize(len(src)))
	encoded, err := c.Encode(encBuf, src)
	if err != nil {
		t.Fatalf("Encode random: %v", err)
	}

	decBuf := make([]byte, len(src))
	decoded, err := c.Decode(decBuf, encoded)
	if err != nil {
		t.Fatalf("Decode random: %v", err)
	}
	if !bytes.Equal(decoded, src) {
		t.Error("round-trip mismatch for random data")
	}
}

func TestLZ4NilDst(t *testing.T) {
	c := LZ4()
	src := makeGUIPixels(400, 120)

	// nil dst on Encode forces allocation (slow path).
	encoded, err := c.Encode(nil, src)
	if err != nil {
		t.Fatalf("Encode(nil, src): %v", err)
	}

	// nil dst on Decode forces allocation with growth strategy.
	decoded, err := c.Decode(nil, encoded)
	if err != nil {
		t.Fatalf("Decode(nil, encoded): %v", err)
	}
	if !bytes.Equal(decoded, src) {
		t.Error("round-trip with nil dst: mismatch")
	}
}

func TestLZ4SmallDstEncode(t *testing.T) {
	c := LZ4()
	src := makeGUIPixels(400, 120)

	// dst too small for MaxEncodedSize -- forces reallocation.
	dst := make([]byte, 4)
	encoded, err := c.Encode(dst, src)
	if err != nil {
		t.Fatalf("Encode small dst: %v", err)
	}

	// Verify round-trip.
	decBuf := make([]byte, len(src))
	decoded, err := c.Decode(decBuf, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, src) {
		t.Error("round-trip mismatch with small encode dst")
	}
}

func TestLZ4SmallDstDecode(t *testing.T) {
	c := LZ4()
	src := makeGUIPixels(400, 120)

	// Encode normally.
	encBuf := make([]byte, c.MaxEncodedSize(len(src)))
	encoded, err := c.Encode(encBuf, src)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Decode with dst too small -- triggers retry growth.
	smallDst := make([]byte, 64)
	decoded, err := c.Decode(smallDst, encoded)
	if err != nil {
		t.Fatalf("Decode small dst: %v", err)
	}
	if !bytes.Equal(decoded, src) {
		t.Error("round-trip mismatch with small decode dst")
	}
}

func TestLZ4ID(t *testing.T) {
	c := LZ4()
	if c.ID() != IDLZ4 {
		t.Errorf("ID() = 0x%02x, want 0x%02x", c.ID(), IDLZ4)
	}
}

func TestLZ4MaxEncodedSize(t *testing.T) {
	c := LZ4()

	tests := []int{0, 1, 100, 1024, 192000}
	for _, srcLen := range tests {
		maxSize := c.MaxEncodedSize(srcLen)
		if maxSize < srcLen {
			t.Errorf("MaxEncodedSize(%d) = %d, want >= %d", srcLen, maxSize, srcLen)
		}
	}
}

func TestLZ4RoundTripVaryingSizes(t *testing.T) {
	c := LZ4()

	sizes := []int{1, 2, 3, 4, 8, 12, 15, 16, 17, 64, 128, 256, 1024, 8192}
	for _, size := range sizes {
		src := make([]byte, size)
		for i := range src {
			src[i] = byte(i*17 + 31)
		}

		encBuf := make([]byte, c.MaxEncodedSize(len(src)))
		encoded, err := c.Encode(encBuf, src)
		if err != nil {
			t.Fatalf("Encode size=%d: %v", size, err)
		}

		decBuf := make([]byte, len(src))
		decoded, err := c.Decode(decBuf, encoded)
		if err != nil {
			t.Fatalf("Decode size=%d: %v", size, err)
		}
		if !bytes.Equal(decoded, src) {
			t.Errorf("round-trip mismatch for size=%d", size)
		}
	}
}

func TestLZ4ConcurrentSafety(t *testing.T) {
	c := LZ4()
	src := makeGUIPixels(400, 120)
	done := make(chan struct{})

	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			encBuf := make([]byte, c.MaxEncodedSize(len(src)))
			decBuf := make([]byte, len(src))
			for j := 0; j < 50; j++ {
				encoded, err := c.Encode(encBuf, src)
				if err != nil {
					t.Errorf("concurrent Encode: %v", err)
					return
				}
				decoded, err := c.Decode(decBuf, encoded)
				if err != nil {
					t.Errorf("concurrent Decode: %v", err)
					return
				}
				if !bytes.Equal(decoded, src) {
					t.Errorf("concurrent round-trip mismatch")
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

// BenchmarkLZ4Encode benchmarks LZ4 encoding with a realistic GUI frame (192KB).
func BenchmarkLZ4Encode(b *testing.B) {
	c := LZ4()
	src := makeGUIPixels(400, 120) // 192,000 bytes
	dst := make([]byte, c.MaxEncodedSize(len(src)))

	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for b.Loop() {
		_, _ = c.Encode(dst, src)
	}
}

// BenchmarkLZ4Decode benchmarks LZ4 decoding with a realistic GUI frame.
func BenchmarkLZ4Decode(b *testing.B) {
	c := LZ4()
	src := makeGUIPixels(400, 120)
	encBuf := make([]byte, c.MaxEncodedSize(len(src)))
	encoded, err := c.Encode(encBuf, src)
	if err != nil {
		b.Fatalf("setup Encode: %v", err)
	}

	dst := make([]byte, len(src))
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for b.Loop() {
		_, _ = c.Decode(dst, encoded)
	}
}

// BenchmarkLZ4EncodeFullHD benchmarks LZ4 encoding with a 1920x1080 frame.
func BenchmarkLZ4EncodeFullHD(b *testing.B) {
	c := LZ4()
	src := makeGUIPixels(1920, 1080) // ~8.3 MB
	dst := make([]byte, c.MaxEncodedSize(len(src)))

	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for b.Loop() {
		_, _ = c.Encode(dst, src)
	}
}

// BenchmarkLZ4DecodeFullHD benchmarks LZ4 decoding with a 1920x1080 frame.
func BenchmarkLZ4DecodeFullHD(b *testing.B) {
	c := LZ4()
	src := makeGUIPixels(1920, 1080)
	encBuf := make([]byte, c.MaxEncodedSize(len(src)))
	encoded, err := c.Encode(encBuf, src)
	if err != nil {
		b.Fatalf("setup Encode: %v", err)
	}

	dst := make([]byte, len(src))
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for b.Loop() {
		_, _ = c.Decode(dst, encoded)
	}
}
