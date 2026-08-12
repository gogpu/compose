package protocol

// MaxPayloadSize is the maximum payload size accepted by the socket
// transport and decompression boundary.
//
// Header payload fields remain uint32 so Decode continues to parse every
// representable wire value. The limit is enforced only before a transport or
// codec allocation, keeping malformed frames from forcing unbounded memory
// use while allowing the protocol representation to remain forward-compatible.
const MaxPayloadSize = 64 * 1024 * 1024
