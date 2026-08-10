package compose

import (
	"bytes"
	"testing"

	"github.com/gogpu/compose/internal/codec"
	"github.com/gogpu/compose/internal/protocol"
)

func TestDecodePayloadRejectsOversizedUncompressedSize(t *testing.T) {
	hdr := protocol.Header{
		Flags:            protocol.FlagCompressed,
		Compression:      protocol.CompressionLZ4,
		UncompressedSize: ^uint32(0),
	}

	_, err := (&Server{}).decodePayload(hdr, []byte("compressed"))
	if err == nil {
		t.Fatal("decodePayload accepted an oversized uncompressed size")
	}
}

func TestDecodePayloadLZ4RoundTrip(t *testing.T) {
	src := bytes.Repeat([]byte{0xA5}, 4096)
	c := codec.LZ4()
	encoded, err := c.Encode(nil, src)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	hdr := protocol.Header{
		Flags:            protocol.FlagCompressed,
		Compression:      protocol.CompressionLZ4,
		UncompressedSize: uint32(len(src)),
	}
	decoded, err := (&Server{}).decodePayload(hdr, encoded)
	if err != nil {
		t.Fatalf("decodePayload: %v", err)
	}
	if !bytes.Equal(decoded, src) {
		t.Fatalf("decoded payload differs: got %d bytes, want %d", len(decoded), len(src))
	}
}

func TestDecodePayloadLZ4ZeroDeclaredSize(t *testing.T) {
	src := bytes.Repeat([]byte{0x5A}, 4096)
	c := codec.LZ4()
	encoded, err := c.Encode(nil, src)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// A zero declaration exercises the codec's nil-destination fallback. It
	// remains valid for callers that do not know the decoded size in advance.
	hdr := protocol.Header{
		Flags:       protocol.FlagCompressed,
		Compression: protocol.CompressionLZ4,
	}
	decoded, err := (&Server{}).decodePayload(hdr, encoded)
	if err != nil {
		t.Fatalf("decodePayload with zero declared size: %v", err)
	}
	if !bytes.Equal(decoded, src) {
		t.Fatalf("decoded payload differs: got %d bytes, want %d", len(decoded), len(src))
	}
}
