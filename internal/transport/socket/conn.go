package socket

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/gogpu/compose/internal/protocol"
)

// bufferSize is the bufio.Reader buffer size.
// 256 KB handles most frames (including 192 KB payloads) in one syscall.
const bufferSize = 256 * 1024

// Conn wraps a net.Conn with framed header+payload read/write.
// Concurrent reads and writes are safe (separate locks).
// Concurrent reads from multiple goroutines are serialized by readMu.
// Concurrent writes from multiple goroutines are serialized by writeMu.
type Conn struct {
	raw     net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
	readMu  sync.Mutex
	hdrBuf  [protocol.HeaderSize]byte // reused scratch buffer for header encode/decode
	hsBuf   [protocol.HandshakeSize]byte
}

// NewConn wraps an existing network connection with framed I/O.
func NewConn(c net.Conn) *Conn {
	return &Conn{
		raw:    c,
		reader: bufio.NewReaderSize(c, bufferSize),
	}
}

// WriteFrame sends a header + payload atomically.
// The header is encoded into an internal buffer and written together with
// payload in a single locked section to prevent interleaving.
func (c *Conn) WriteFrame(hdr *protocol.Header, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := protocol.Encode(hdr, c.hdrBuf[:]); err != nil {
		return fmt.Errorf("socket: encode header: %w", err)
	}

	// Write header bytes.
	if _, err := c.raw.Write(c.hdrBuf[:]); err != nil {
		return fmt.Errorf("socket: write header: %w", err)
	}

	// Write payload if present.
	if len(payload) > 0 {
		if _, err := c.raw.Write(payload); err != nil {
			return fmt.Errorf("socket: write payload: %w", err)
		}
	}

	return nil
}

// ReadFrame reads a header + payload from the connection.
// The returned payload slice is freshly allocated if PayloadSize > 0.
func (c *Conn) ReadFrame() (protocol.Header, []byte, error) {
	return c.ReadFrameInto(nil)
}

// ReadFrameInto reads a header + payload into a caller-provided buffer.
// If buf is nil or too small for the header's PayloadSize, a new buffer
// is allocated. The returned slice may be a sub-slice of buf or a new
// allocation.
func (c *Conn) ReadFrameInto(buf []byte) (protocol.Header, []byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	// Read exactly HeaderSize bytes into scratch buffer.
	if _, err := io.ReadFull(c.reader, c.hdrBuf[:]); err != nil {
		return protocol.Header{}, nil, fmt.Errorf("socket: read header: %w", err)
	}

	hdr, err := protocol.Decode(c.hdrBuf[:])
	if err != nil {
		return protocol.Header{}, nil, fmt.Errorf("socket: decode header: %w", err)
	}

	// Validate the wire-sized field before converting it to int or allocating.
	// Header.Decode intentionally accepts the complete uint32 field range for
	// protocol compatibility; this boundary is where memory use is bounded.
	if hdr.PayloadSize > protocol.MaxPayloadSize {
		return protocol.Header{}, nil, fmt.Errorf("%w: declared %d bytes (limit %d)",
			ErrPayloadTooLarge, hdr.PayloadSize, protocol.MaxPayloadSize)
	}

	// Read payload.
	size := int(hdr.PayloadSize)
	if size == 0 {
		return hdr, nil, nil
	}

	// Reuse or allocate payload buffer.
	var payload []byte
	if len(buf) >= size {
		payload = buf[:size]
	} else {
		payload = make([]byte, size)
	}

	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return protocol.Header{}, nil, fmt.Errorf("socket: read payload (%d bytes): %w", size, err)
	}

	return hdr, payload, nil
}

// WriteHandshakeHello sends a HelloMsg on the connection.
func (c *Conn) WriteHandshakeHello(msg *protocol.HelloMsg) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := protocol.EncodeHello(msg, c.hsBuf[:]); err != nil {
		return fmt.Errorf("socket: encode hello: %w", err)
	}

	if _, err := c.raw.Write(c.hsBuf[:]); err != nil {
		return fmt.Errorf("socket: write hello: %w", err)
	}

	return nil
}

// ReadHandshakeHello reads a HelloMsg from the connection.
func (c *Conn) ReadHandshakeHello() (protocol.HelloMsg, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if _, err := io.ReadFull(c.reader, c.hsBuf[:]); err != nil {
		return protocol.HelloMsg{}, fmt.Errorf("socket: read hello: %w", err)
	}

	msg, err := protocol.DecodeHello(c.hsBuf[:])
	if err != nil {
		return protocol.HelloMsg{}, fmt.Errorf("socket: decode hello: %w", err)
	}

	return msg, nil
}

// WriteHandshakeWelcome sends a WelcomeMsg on the connection.
func (c *Conn) WriteHandshakeWelcome(msg *protocol.WelcomeMsg) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := protocol.EncodeWelcome(msg, c.hsBuf[:]); err != nil {
		return fmt.Errorf("socket: encode welcome: %w", err)
	}

	if _, err := c.raw.Write(c.hsBuf[:]); err != nil {
		return fmt.Errorf("socket: write welcome: %w", err)
	}

	return nil
}

// ReadHandshakeWelcome reads a WelcomeMsg from the connection.
func (c *Conn) ReadHandshakeWelcome() (protocol.WelcomeMsg, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if _, err := io.ReadFull(c.reader, c.hsBuf[:]); err != nil {
		return protocol.WelcomeMsg{}, fmt.Errorf("socket: read welcome: %w", err)
	}

	msg, err := protocol.DecodeWelcome(c.hsBuf[:])
	if err != nil {
		return protocol.WelcomeMsg{}, fmt.Errorf("socket: decode welcome: %w", err)
	}

	return msg, nil
}

// Close closes the underlying network connection.
func (c *Conn) Close() error {
	return c.raw.Close()
}

// RemoteAddr returns the remote address of the underlying connection.
func (c *Conn) RemoteAddr() net.Addr {
	return c.raw.RemoteAddr()
}
