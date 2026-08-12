package compose

import (
	"fmt"
	"image"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogpu/compose/internal/codec"
	"github.com/gogpu/compose/internal/protocol"
	"github.com/gogpu/compose/internal/transport/socket"
)

// saturateUint16 converts v to uint16, clamping to math.MaxUint16 on overflow.
func saturateUint16(v int) uint16 {
	return uint16(min(max(v, 0), math.MaxUint16)) //nolint:gosec // clamped to [0, MaxUint16]
}

// saturateUint16from32 converts v to uint16, clamping to math.MaxUint16 on overflow.
func saturateUint16from32(v uint32) uint16 {
	return uint16(min(v, math.MaxUint16)) //nolint:gosec // clamped to [0, MaxUint16]
}

// saturateUint32 converts v to uint32, clamping to math.MaxUint32 on overflow.
func saturateUint32(v int) uint32 {
	if v <= 0 {
		return 0
	}
	if uint64(v) > uint64(math.MaxUint32) {
		return math.MaxUint32
	}
	return uint32(v) //nolint:gosec // checked to be in [0, MaxUint32]
}

// Client is the module-side endpoint that connects to a compositor and
// publishes frames. All methods are safe for concurrent use.
type Client struct {
	conn     *socket.Conn
	moduleID uint64
	name     string
	codec    codec.Codec

	mu             sync.RWMutex
	onFrameRequest func()

	closed atomic.Bool
	done   chan struct{}
	wg     sync.WaitGroup
	seq    atomic.Uint64
}

// Dial creates a Client that connects to a compositor at the given Unix
// domain socket address. Dial performs the handshake immediately: it sends
// a HelloMsg and reads the WelcomeMsg. If the compositor rejects the
// connection, ErrNotAccepted is returned.
//
// Use ClientOption functions to configure the module:
//
//	client, err := compose.Dial("/tmp/compose.sock",
//	    compose.WithName("clock"),
//	    compose.WithFrameSize(400, 120),
//	    compose.WithFPS(1),
//	)
func Dial(addr string, opts ...ClientOption) (*Client, error) {
	cfg := defaultClientConfig()
	for _, o := range opts {
		o(&cfg)
	}

	dialer := socket.NewDialer(addr)
	sc, err := dialer.Dial()
	if err != nil {
		return nil, fmt.Errorf("compose: dial: %w", err)
	}

	// Build and send HelloMsg.
	hello := &protocol.HelloMsg{
		Magic:        protocol.Magic,
		Version:      protocol.ProtocolVersion,
		Width:        saturateUint16from32(cfg.width),
		Height:       saturateUint16from32(cfg.height),
		PreferredFPS: cfg.fps,
		Transport:    protocol.TransportSocket,
	}
	protocol.SetName(hello, cfg.name)

	if err := sc.WriteHandshakeHello(hello); err != nil {
		_ = sc.Close()
		return nil, fmt.Errorf("compose: send hello: %w", err)
	}

	// Read WelcomeMsg.
	welcome, err := sc.ReadHandshakeWelcome()
	if err != nil {
		_ = sc.Close()
		return nil, fmt.Errorf("compose: read welcome: %w", err)
	}

	if welcome.Accepted == 0 {
		_ = sc.Close()
		return nil, ErrNotAccepted
	}

	c := &Client{
		conn:     sc,
		moduleID: welcome.ModuleID,
		name:     cfg.name,
		codec:    codec.Raw(), // client always sends raw; server handles decompression
		done:     make(chan struct{}),
	}

	// Start reader goroutine for FrameRequest messages.
	c.wg.Add(1)
	go c.readLoop()

	return c, nil
}

// PublishFrame sends a frame to the compositor.
// The frame's ModuleID is automatically set to this client's assigned ID.
//
// Returns ErrClosed if the client has been shut down.
func (c *Client) PublishFrame(f Frame) error {
	if c.closed.Load() {
		return ErrClosed
	}

	seq := c.seq.Add(1)
	pixels := f.Pixels
	uncompressedSize := saturateUint32(len(pixels))

	// Snapshot the codec while holding the client lock, then release it before
	// doing the potentially expensive encode. Keeping the snapshot local makes
	// the compression flags and payload use the same codec even if
	// SetCompression runs concurrently.
	c.mu.RLock()
	frameCodec := c.codec
	c.mu.RUnlock()

	// Compress if codec is not raw.
	var flags protocol.Flag
	compressionID := protocol.CompressionNone

	codecID := frameCodec.ID()
	if codecID != codec.IDRaw {
		maxSize := frameCodec.MaxEncodedSize(len(pixels))
		dst := make([]byte, maxSize)
		compressed, err := frameCodec.Encode(dst, pixels)
		if err != nil {
			return fmt.Errorf("compose: compress frame: %w", err)
		}
		pixels = compressed
		flags = flags.Set(protocol.FlagCompressed)
		compressionID = protocol.Compression(codecID)
	}

	// Set dirty rect flags.
	if !f.DirtyRect.Empty() {
		flags = flags.Set(protocol.FlagDirtyValid)
	} else {
		flags = flags.Set(protocol.FlagKeyframe)
	}

	hdr := &protocol.Header{
		Magic:            protocol.Magic,
		Version:          protocol.ProtocolVersion,
		MsgType:          protocol.MsgFrame,
		Flags:            flags,
		ModuleID:         c.moduleID,
		Sequence:         seq,
		TimestampNs:      f.Timestamp,
		Width:            saturateUint16from32(f.Width),
		Height:           saturateUint16from32(f.Height),
		Stride:           f.Width * 4,
		PixelFormat:      protocol.PixelRGBA8,
		Compression:      compressionID,
		PayloadSize:      saturateUint32(len(pixels)),
		UncompressedSize: uncompressedSize,
	}

	// Set dirty rect fields if valid.
	if flags.Has(protocol.FlagDirtyValid) {
		hdr.DirtyX = saturateUint16(f.DirtyRect.Min.X)
		hdr.DirtyY = saturateUint16(f.DirtyRect.Min.Y)
		hdr.DirtyW = saturateUint16(f.DirtyRect.Dx())
		hdr.DirtyH = saturateUint16(f.DirtyRect.Dy())
	}

	// Use monotonic timestamp if caller did not set one.
	if hdr.TimestampNs == 0 {
		hdr.TimestampNs = time.Now().UnixNano()
	}

	return c.conn.WriteFrame(hdr, pixels)
}

// OnFrameRequest registers a callback invoked when the compositor requests
// a frame. This enables pull-based rendering: the module renders only when
// asked. The callback is called on an internal goroutine; it must not block
// for extended periods. Only one callback can be active; subsequent calls
// replace the previous one. Pass nil to remove.
func (c *Client) OnFrameRequest(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onFrameRequest = fn
}

// Close disconnects from the compositor and releases resources.
// After Close returns, no more callbacks will be invoked.
func (c *Client) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}

	close(c.done)

	// Send graceful disconnect message (best-effort).
	hdr := &protocol.Header{
		Magic:    protocol.Magic,
		Version:  protocol.ProtocolVersion,
		MsgType:  protocol.MsgDisconnect,
		ModuleID: c.moduleID,
	}
	_ = c.conn.WriteFrame(hdr, nil)

	err := c.conn.Close()

	c.wg.Wait()

	if err != nil {
		return fmt.Errorf("compose: close client: %w", err)
	}
	return nil
}

// ModuleID returns the compositor-assigned module identifier.
// This is valid after Dial returns successfully.
func (c *Client) ModuleID() uint64 {
	return c.moduleID
}

// readLoop runs in a goroutine, reading control messages from the
// compositor. Currently handles FrameRequest messages.
func (c *Client) readLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		hdr, _, err := c.conn.ReadFrame()
		if err != nil {
			// Connection closed or error — stop reading.
			return
		}

		switch hdr.MsgType {
		case protocol.MsgFrameRequest:
			c.mu.RLock()
			cb := c.onFrameRequest
			c.mu.RUnlock()
			if cb != nil {
				cb()
			}

		case protocol.MsgDisconnect:
			return

		default:
			// Ignore unknown message types for forward compatibility.
		}
	}
}

// SetCompression sets the codec used for frame payload compression.
// This allows the compositor to negotiate compression during or after
// handshake. Supported values: "lz4". Any other value uses raw.
func (c *Client) SetCompression(algo string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.codec = resolveCodec(algo)
}

// frameToHeader converts a public Frame to a protocol.Header.
// Used in PublishFrame; extracted here for testing.
func frameToHeader(f Frame, moduleID uint64, seq uint64, c codec.Codec) protocol.Header {
	var flags protocol.Flag
	if !f.DirtyRect.Empty() {
		flags = flags.Set(protocol.FlagDirtyValid)
	} else {
		flags = flags.Set(protocol.FlagKeyframe)
	}

	if c.ID() != codec.IDRaw {
		flags = flags.Set(protocol.FlagCompressed)
	}

	hdr := protocol.Header{
		Magic:            protocol.Magic,
		Version:          protocol.ProtocolVersion,
		MsgType:          protocol.MsgFrame,
		Flags:            flags,
		ModuleID:         moduleID,
		Sequence:         seq,
		TimestampNs:      f.Timestamp,
		Width:            saturateUint16from32(f.Width),
		Height:           saturateUint16from32(f.Height),
		Stride:           f.Width * 4,
		PixelFormat:      protocol.PixelRGBA8,
		Compression:      protocol.Compression(c.ID()),
		PayloadSize:      saturateUint32(len(f.Pixels)),
		UncompressedSize: saturateUint32(len(f.Pixels)),
	}

	if flags.Has(protocol.FlagDirtyValid) {
		dr := f.DirtyRect.Canon()
		hdr.DirtyX = saturateUint16(dr.Min.X)
		hdr.DirtyY = saturateUint16(dr.Min.Y)
		hdr.DirtyW = saturateUint16(dr.Dx())
		hdr.DirtyH = saturateUint16(dr.Dy())
	}

	return hdr
}

// isDirtyRectValid reports whether the dirty rect represents a valid
// sub-region (non-empty).
func isDirtyRectValid(r image.Rectangle) bool {
	return !r.Empty()
}
