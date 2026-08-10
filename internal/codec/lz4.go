package codec

import (
	"fmt"
	"sync"

	"github.com/pierrec/lz4/v4"

	"github.com/gogpu/compose/internal/protocol"
)

func init() {
	Register(LZ4())
}

// LZ4 returns a codec using LZ4 block compression. It uses a pooled
// lz4.Compressor to avoid allocations on the hot path. The compressor
// hash table is reused across calls via sync.Pool.
//
// LZ4 provides fast compression with moderate ratios, making it ideal for
// frame pixel data that contains large flat color regions (typical in GUIs).
func LZ4() Codec {
	return &lz4Codec{
		pool: sync.Pool{
			New: func() any {
				var c lz4.Compressor
				return &c
			},
		},
	}
}

type lz4Codec struct {
	pool sync.Pool
}

// maxDecodeBuf preserves the codec's existing internal name while sourcing
// the bound from the shared protocol allocation policy.
const maxDecodeBuf = protocol.MaxPayloadSize

// maxDecodeRatio bounds the nil-destination fallback for a compressed block.
// LZ4's block format encodes matches with a 16-bit offset and bounded length
// extensions, so a few hundred times expansion is the practical upper range.
// Keeping the fallback proportional to the input also prevents tiny malformed
// blocks from forcing the global 64 MiB ceiling.
const maxDecodeRatio = 256

// Encode compresses src using LZ4 block compression. Returns a sub-slice of
// dst containing the compressed data. If dst is nil or too small, allocates a
// new buffer.
//
// If src is empty, returns an empty slice without error.
func (c *lz4Codec) Encode(dst, src []byte) ([]byte, error) {
	if len(src) == 0 {
		if dst == nil {
			return nil, nil
		}
		return dst[:0], nil
	}

	maxSize := lz4.CompressBlockBound(len(src))
	if cap(dst) < maxSize {
		dst = make([]byte, maxSize)
	} else {
		dst = dst[:maxSize]
	}

	compressor := c.pool.Get().(*lz4.Compressor)
	n, err := compressor.CompressBlock(src, dst)
	c.pool.Put(compressor)

	if err != nil {
		return nil, fmt.Errorf("codec: lz4 encode: %w", err)
	}

	return dst[:n], nil
}

// Decode decompresses an LZ4 block-compressed payload. Returns a sub-slice of
// dst containing the decompressed data. If dst is nil or too small, allocates
// a new buffer.
//
// The caller should provide a dst buffer sized to the expected uncompressed
// length (e.g., Width * Height * 4 for RGBA frames). When dst is nil or too
// small, an exponential growth strategy is used as a fallback.
func (c *lz4Codec) Decode(dst, src []byte) ([]byte, error) {
	if len(src) == 0 {
		if dst == nil {
			return nil, nil
		}
		return dst[:0], nil
	}

	if cap(dst) == 0 {
		// Caller didn't provide a buffer. Start with 10x compressed size
		// as initial guess. LZ4 GUI data often compresses 100:1 or better,
		// but 10x covers most cases in one attempt. Clamp before multiplying:
		// malformed input must not overflow int or allocate beyond the growth
		// limit before the limit below gets a chance to run.
		dst = make([]byte, initialDecodeSize(len(src)))
	} else {
		dst = dst[:cap(dst)]
	}
	maxSize := maxDecodeSize(len(src))

	for {
		n, err := lz4.UncompressBlock(src, dst)
		if err != nil {
			// If buffer might be too small, double and retry.
			if len(dst) < maxSize {
				next := len(dst) * 2
				if next <= len(dst) || next > maxSize {
					next = maxSize
				}
				dst = make([]byte, next)
				continue
			}
			return nil, fmt.Errorf("codec: lz4 decode: %w", err)
		}
		return dst[:n], nil
	}
}

// initialDecodeSize returns the first allocation used by the nil-destination
// fallback without allowing len(src)*10 to overflow or exceed the shared
// payload bound.
func initialDecodeSize(srcLen int) int {
	maxSize := maxDecodeSize(srcLen)
	if srcLen > maxDecodeBuf/10 {
		return maxSize
	}
	initial := srcLen * 10
	if initial > maxSize {
		return maxSize
	}
	return initial
}

// maxDecodeSize returns the largest fallback destination permitted for a
// compressed block of srcLen bytes. It is overflow-safe and never exceeds the
// shared protocol allocation bound.
func maxDecodeSize(srcLen int) int {
	if srcLen >= maxDecodeBuf/maxDecodeRatio {
		return maxDecodeBuf
	}
	return srcLen * maxDecodeRatio
}

// ID returns the LZ4 codec protocol identifier (0x01).
func (c *lz4Codec) ID() byte {
	return IDLZ4
}

// MaxEncodedSize returns the worst-case encoded size for LZ4 block compression.
func (c *lz4Codec) MaxEncodedSize(srcLen int) int {
	return lz4.CompressBlockBound(srcLen)
}
