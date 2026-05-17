// Package codec provides frame payload compression and decompression for the
// compose protocol. It defines a Codec interface with pluggable implementations
// and a global registry for protocol-level codec negotiation.
//
// Implementations must be safe for concurrent use. The Encode and Decode methods
// accept caller-provided destination buffers to avoid allocations on the hot path.
// When the caller provides a sufficiently sized buffer, zero allocations occur.
//
// Available codecs:
//   - Raw (ID 0x00): Pass-through copy, no compression.
//   - LZ4 (ID 0x01): LZ4 block compression via github.com/pierrec/lz4/v4.
//
// Registration happens automatically via init() in each codec's source file.
// Use Get(id) to retrieve a codec by its protocol identifier.
package codec
