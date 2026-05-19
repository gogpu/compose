package compose

import (
	"fmt"
	"image"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogpu/compose/internal/codec"
	"github.com/gogpu/compose/internal/conn"
	"github.com/gogpu/compose/internal/flow"
	"github.com/gogpu/compose/internal/protocol"
	"github.com/gogpu/compose/internal/transport/socket"
)

// moduleConn tracks a per-module connection on the server side.
// Each moduleConn contains a mailbox slot that stores the most recent frame
// received from this module. The mailbox enables "latest frame wins" semantics
// for compositors that poll via Server.Snapshot() instead of (or in addition to)
// the OnFrame callback.
type moduleConn struct {
	conn     *socket.Conn
	moduleID uint64
	name     string

	// latestMu guards access to the mailbox slot (latest and seq).
	latestMu sync.Mutex

	// latest holds the most recently received frame, or nil if no frame
	// has been received yet. Overwritten on every receipt (latest wins).
	latest *Frame

	// seq is the wire protocol sequence number of the stored frame.
	// Used for change detection by Snapshot consumers.
	seq uint64
}

// storeLatest overwrites the mailbox with the given frame. Thread-safe.
func (mc *moduleConn) storeLatest(f Frame) {
	mc.latestMu.Lock()
	mc.latest = &f
	mc.seq = f.Sequence
	mc.latestMu.Unlock()
}

// loadLatest returns the current mailbox frame (may be nil) and its sequence.
// Thread-safe. Does not clear the mailbox — compositors may call Snapshot
// multiple times and the frame stays until overwritten by a newer one.
func (mc *moduleConn) loadLatest() *Frame {
	mc.latestMu.Lock()
	f := mc.latest
	mc.latestMu.Unlock()
	return f
}

// Server is the compositor-side endpoint that accepts module connections
// and delivers frames. All methods are safe for concurrent use.
type Server struct {
	listener *socket.Listener
	manager  *conn.Manager
	flow     *flow.Controller
	codec    codec.Codec

	mu           sync.RWMutex
	onFrame      func(Frame)
	onConnect    func(id uint64, name string)
	onDisconnect func(id uint64, name string)

	modulesMu sync.RWMutex
	modules   map[uint64]*moduleConn

	closed atomic.Bool
	done   chan struct{}
	wg     sync.WaitGroup
}

// Listen creates a Server that accepts module connections on the given
// Unix domain socket address. The server immediately begins accepting
// connections in a background goroutine.
//
// Use ServerOption functions to configure behavior:
//
//	srv, err := compose.Listen("/tmp/compose.sock",
//	    compose.WithMaxModules(8),
//	    compose.WithCompression("lz4"),
//	)
func Listen(addr string, opts ...ServerOption) (*Server, error) {
	cfg := defaultServerConfig()
	for _, o := range opts {
		o(&cfg)
	}

	ln, err := socket.Listen(addr)
	if err != nil {
		return nil, fmt.Errorf("compose: listen: %w", err)
	}

	c := resolveCodec(cfg.compression)

	s := &Server{
		listener: ln,
		manager:  conn.NewManager(cfg.maxModules),
		flow:     flow.New(),
		codec:    c,
		modules:  make(map[uint64]*moduleConn),
		done:     make(chan struct{}),
	}

	// Wire manager callbacks to server callbacks.
	s.manager.OnConnect(func(id uint64, name string) {
		s.mu.RLock()
		cb := s.onConnect
		s.mu.RUnlock()
		if cb != nil {
			cb(id, name)
		}
	})

	s.manager.OnDisconnect(func(id uint64, name string) {
		s.mu.RLock()
		cb := s.onDisconnect
		s.mu.RUnlock()
		if cb != nil {
			cb(id, name)
		}
	})

	// Start accept loop.
	s.wg.Add(1)
	go s.acceptLoop()

	return s, nil
}

// OnFrame registers a callback invoked when a module delivers a frame.
// The callback is called on an internal goroutine; it must not block for
// extended periods. Only one callback can be active; subsequent calls
// replace the previous one. Pass nil to remove.
func (s *Server) OnFrame(fn func(Frame)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFrame = fn
}

// OnConnect registers a callback invoked when a module completes the
// handshake and becomes active. Only one callback can be active;
// subsequent calls replace the previous one. Pass nil to remove.
func (s *Server) OnConnect(fn func(id uint64, name string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onConnect = fn

	// Also update the manager callback so it fires the new function.
	s.manager.OnConnect(func(id uint64, name string) {
		s.mu.RLock()
		cb := s.onConnect
		s.mu.RUnlock()
		if cb != nil {
			cb(id, name)
		}
	})
}

// OnDisconnect registers a callback invoked when a module disconnects
// or crashes. Only one callback can be active; subsequent calls replace
// the previous one. Pass nil to remove.
func (s *Server) OnDisconnect(fn func(id uint64, name string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDisconnect = fn

	// Also update the manager callback so it fires the new function.
	s.manager.OnDisconnect(func(id uint64, name string) {
		s.mu.RLock()
		cb := s.onDisconnect
		s.mu.RUnlock()
		if cb != nil {
			cb(id, name)
		}
	})
}

// RequestFrame sends a frame-request signal to the specified module
// (pull model). The module should respond with its next frame via
// PublishFrame.
//
// Returns ErrModuleNotFound if the module ID is not connected.
// Returns ErrClosed if the server has been shut down.
func (s *Server) RequestFrame(moduleID uint64) error {
	if s.closed.Load() {
		return ErrClosed
	}

	s.modulesMu.RLock()
	mc, ok := s.modules[moduleID]
	s.modulesMu.RUnlock()

	if !ok {
		return ErrModuleNotFound
	}

	hdr := &protocol.Header{
		Magic:   protocol.Magic,
		Version: protocol.ProtocolVersion,
		MsgType: protocol.MsgFrameRequest,
	}
	hdr.ModuleID = moduleID

	if err := mc.conn.WriteFrame(hdr, nil); err != nil {
		return fmt.Errorf("compose: request frame: %w", err)
	}

	s.flow.FrameRequested(moduleID)
	return nil
}

// Snapshot returns the latest frame from each connected module.
// Returns nil entries for modules that haven't sent any frames yet.
// The returned map is keyed by module ID.
//
// Snapshot does NOT clear the mailbox — the frame stays until overwritten
// by a newer one from the same module. Compositors may call Snapshot
// multiple times per render tick; each call returns the same frame until
// the module publishes a new one. Use Frame.Sequence for change detection.
//
// Thread-safe. Designed to be called once per compositor render tick.
func (s *Server) Snapshot() map[uint64]*Frame {
	s.modulesMu.RLock()
	result := make(map[uint64]*Frame, len(s.modules))
	for id, mc := range s.modules {
		result[id] = mc.loadLatest()
	}
	s.modulesMu.RUnlock()
	return result
}

// Close shuts down the server, disconnects all modules, and releases
// resources. After Close returns, no more callbacks will be invoked.
func (s *Server) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return ErrClosed
	}

	close(s.done)

	// Close the listener to unblock Accept().
	err := s.listener.Close()

	// Close all module connections.
	s.modulesMu.RLock()
	for _, mc := range s.modules {
		_ = mc.conn.Close() // best-effort
	}
	s.modulesMu.RUnlock()

	// Wait for all goroutines to finish.
	s.wg.Wait()

	if err != nil {
		return fmt.Errorf("compose: close server: %w", err)
	}
	return nil
}

// acceptLoop runs in a goroutine, accepting new module connections.
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		c, err := s.listener.Accept()
		if err != nil {
			// If server is closing, Accept error is expected.
			if s.closed.Load() {
				return
			}
			// Transient error — continue accepting.
			continue
		}

		s.wg.Add(1)
		go s.handleModule(c)
	}
}

// handleModule runs in a goroutine for each connected module.
// It performs the handshake, then enters a frame read loop.
func (s *Server) handleModule(c *socket.Conn) {
	defer s.wg.Done()

	// Read HelloMsg from module.
	hello, err := c.ReadHandshakeHello()
	if err != nil {
		_ = c.Close()
		return
	}

	name := protocol.GetName(&hello)

	// Register module via Manager (allocates ID, fires OnConnect callback).
	moduleID, err := s.manager.HandleConnect(name, hello.Width, hello.Height, hello.PreferredFPS)
	if err != nil {
		// Send rejection WelcomeMsg.
		welcome := &protocol.WelcomeMsg{
			Magic:    protocol.Magic,
			Version:  protocol.ProtocolVersion,
			Accepted: 0,
		}
		_ = c.WriteHandshakeWelcome(welcome) // best-effort
		_ = c.Close()
		return
	}

	// Register in flow controller.
	s.flow.AddModule(moduleID, hello.PreferredFPS)

	// Track the module connection.
	mc := &moduleConn{
		conn:     c,
		moduleID: moduleID,
		name:     name,
	}

	s.modulesMu.Lock()
	s.modules[moduleID] = mc
	s.modulesMu.Unlock()

	// Send acceptance WelcomeMsg.
	welcome := &protocol.WelcomeMsg{
		Magic:      protocol.Magic,
		Version:    protocol.ProtocolVersion,
		ModuleID:   moduleID,
		Accepted:   1,
		Transport:  protocol.TransportSocket,
		MinVersion: protocol.ProtocolVersion,
		MaxVersion: protocol.ProtocolVersion,
	}

	if err := c.WriteHandshakeWelcome(welcome); err != nil {
		s.cleanupModule(moduleID)
		_ = c.Close()
		return
	}

	// Enter frame read loop.
	s.readFrameLoop(mc)
}

// readFrameLoop reads frames from a module connection until EOF or error.
func (s *Server) readFrameLoop(mc *moduleConn) {
	defer func() {
		s.cleanupModule(mc.moduleID)
		_ = mc.conn.Close()
	}()

	for {
		// Check if server is shutting down.
		select {
		case <-s.done:
			return
		default:
		}

		hdr, payload, err := mc.conn.ReadFrame()
		if err != nil {
			// EOF or connection error — module disconnected.
			return
		}

		if hdr.MsgType == protocol.MsgDisconnect {
			return
		}

		if hdr.MsgType != protocol.MsgFrame {
			// Ignore non-frame messages in the frame read loop.
			continue
		}

		// Decompress payload if needed.
		pixels, err := s.decodePayload(hdr, payload)
		if err != nil {
			// Corrupted frame — skip it but keep the connection.
			continue
		}

		// Build Frame from header fields.
		frame := headerToFrame(hdr, pixels, mc.name)

		// Store in mailbox (latest-wins for Snapshot consumers).
		mc.storeLatest(frame)

		// Notify flow controller.
		s.flow.FrameDelivered(mc.moduleID)

		// Update registry last frame time.
		s.manager.Registry().UpdateLastFrame(mc.moduleID, time.Now())

		// Fire OnFrame callback (backward compatible — fires on every receipt).
		s.mu.RLock()
		cb := s.onFrame
		s.mu.RUnlock()

		if cb != nil {
			cb(frame)
		}
	}
}

// cleanupModule removes a module from all tracking structures and fires
// the OnDisconnect callback via Manager.
func (s *Server) cleanupModule(moduleID uint64) {
	s.modulesMu.Lock()
	delete(s.modules, moduleID)
	s.modulesMu.Unlock()

	s.flow.RemoveModule(moduleID)
	s.manager.HandleDisconnect(moduleID)
}

// decodePayload decompresses the frame payload based on the header's
// compression flag. Returns the raw RGBA pixel data.
func (s *Server) decodePayload(hdr protocol.Header, payload []byte) ([]byte, error) {
	if !hdr.Flags.Has(protocol.FlagCompressed) || hdr.Compression == protocol.CompressionNone {
		return payload, nil
	}

	// Look up the codec by compression ID.
	c := codec.Get(byte(hdr.Compression))
	if c == nil {
		return nil, fmt.Errorf("compose: unknown compression 0x%02X", hdr.Compression)
	}

	// Allocate destination buffer sized to uncompressed size.
	dst := make([]byte, hdr.UncompressedSize)
	decoded, err := c.Decode(dst, payload)
	if err != nil {
		return nil, fmt.Errorf("compose: decompress: %w", err)
	}

	return decoded, nil
}

// headerToFrame converts a protocol.Header and pixel payload into a Frame.
func headerToFrame(hdr protocol.Header, pixels []byte, name string) Frame {
	f := Frame{
		ModuleID:  hdr.ModuleID,
		Name:      name,
		Pixels:    pixels,
		Width:     uint32(hdr.Width),
		Height:    uint32(hdr.Height),
		Timestamp: hdr.TimestampNs,
		Sequence:  hdr.Sequence,
	}

	if hdr.Flags.Has(protocol.FlagDirtyValid) {
		f.DirtyRect = image.Rect(
			int(hdr.DirtyX),
			int(hdr.DirtyY),
			int(hdr.DirtyX)+int(hdr.DirtyW),
			int(hdr.DirtyY)+int(hdr.DirtyH),
		)
	}

	return f
}

// resolveCodec returns the appropriate codec for the given compression name.
func resolveCodec(name string) codec.Codec {
	switch name {
	case "lz4":
		return codec.LZ4()
	default:
		return codec.Raw()
	}
}
