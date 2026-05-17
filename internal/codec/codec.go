package codec

import "sync"

// Protocol compression identifiers.
const (
	IDRaw byte = 0x00
	IDLZ4 byte = 0x01
)

// Codec compresses and decompresses frame pixel data.
// Implementations must be safe for concurrent use.
// Encode/Decode must not allocate on the hot path when the caller provides
// a sufficiently sized destination buffer.
type Codec interface {
	// Encode compresses src into dst. Returns the compressed slice (sub-slice of dst).
	// dst must be large enough (use MaxEncodedSize to determine required capacity).
	// If dst is nil or too small, a new buffer is allocated (slow path).
	Encode(dst, src []byte) ([]byte, error)

	// Decode decompresses src into dst. Returns the decompressed slice.
	// dst must be large enough to hold the uncompressed data.
	// If dst is nil or too small, a new buffer is allocated (slow path).
	Decode(dst, src []byte) ([]byte, error)

	// ID returns the protocol compression identifier.
	ID() byte

	// MaxEncodedSize returns the maximum possible compressed size for input of
	// the given length. Use this to pre-allocate destination buffers.
	MaxEncodedSize(srcLen int) int
}

var (
	registryMu sync.RWMutex
	registry   = make(map[byte]Codec)
)

// Register adds a codec to the global registry. It is called during init()
// by each codec implementation. Panics if a codec with the same ID is already
// registered.
func Register(c Codec) {
	registryMu.Lock()
	defer registryMu.Unlock()

	id := c.ID()
	if _, exists := registry[id]; exists {
		panic("codec: duplicate registration for ID " + string(rune('0'+id)))
	}
	registry[id] = c
}

// Get returns the codec for the given protocol ID. Returns nil if no codec
// is registered for that ID.
func Get(id byte) Codec {
	registryMu.RLock()
	defer registryMu.RUnlock()

	return registry[id]
}

// resetRegistry clears the global registry. Used only in tests.
func resetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry = make(map[byte]Codec)
}
