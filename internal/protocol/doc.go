// Package protocol defines the wire format for gogpu/compose inter-process
// communication. It is the leaf package in the internal dependency graph —
// imported by transport implementations but importing nothing internal itself.
//
// The protocol is built around a fixed 64-byte binary frame header that
// precedes every message on the wire. Headers use little-endian byte order
// and are designed to be cache-line aligned on modern hardware.
//
// All encode/decode functions operate on caller-provided byte buffers to
// achieve zero allocations on the hot path. This is critical for sustained
// 60 FPS frame delivery.
//
// Wire format version: 1
// Magic bytes: 0x43 0x4F 0x4D 0x50 ("COMP")
package protocol
