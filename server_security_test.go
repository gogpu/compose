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

func TestDecodePayloadRejectsZeroDeclaredSize(t *testing.T) {
	hdr := protocol.Header{
		Flags:       protocol.FlagCompressed,
		Compression: protocol.CompressionLZ4,
	}
	if _, err := (&Server{}).decodePayload(hdr, []byte{0xFF}); err == nil {
		t.Fatal("decodePayload accepted a non-empty compressed payload with zero declared size")
	}
}
