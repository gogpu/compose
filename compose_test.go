package compose

import (
	"errors"
	"image"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogpu/compose/internal/protocol"
)

// tempSocket returns a short temporary Unix socket path.
// macOS limits Unix socket paths to 104 bytes. t.TempDir() on macOS CI
// produces paths like /var/folders/.../TestName.../compose.sock which
// easily exceeds this limit. We use os.CreateTemp with a short prefix
// to guarantee a short path under /tmp (or %TEMP% on Windows).
func tempSocket(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "cs-*.sock")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()
	os.Remove(path) // Remove the file; we need the path for the socket.
	t.Cleanup(func() { os.Remove(path) })
	return path
}

// makePixels creates a simple RGBA pixel buffer of the given dimensions
// filled with the specified byte value.
func makePixels(w, h uint32, fill byte) []byte {
	pixels := make([]byte, w*h*4)
	for i := range pixels {
		pixels[i] = fill
	}
	return pixels
}

// waitFor polls a condition function until it returns true or 5 seconds elapse.
// Returns true if the condition was met. The generous timeout accommodates slow
// CI runners (macOS shared, Ubuntu containers) where goroutine scheduling may
// introduce significant delays.
func waitFor(t *testing.T, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestListenAndDial(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := Dial(addr, WithName("test-module"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if client.ModuleID() == 0 {
		t.Error("ModuleID should be non-zero after successful Dial")
	}
}

func TestFrameRoundTrip(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var received atomic.Value

	srv.OnFrame(func(f Frame) {
		received.Store(f)
	})

	client, err := Dial(addr, WithName("painter"), WithFrameSize(10, 10))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Publish a frame.
	pixels := makePixels(10, 10, 0xAA)
	ts := time.Now().UnixNano()

	err = client.PublishFrame(Frame{
		Pixels:    pixels,
		Width:     10,
		Height:    10,
		Timestamp: ts,
	})
	if err != nil {
		t.Fatalf("PublishFrame: %v", err)
	}

	// Wait for frame to arrive.
	ok := waitFor(t, func() bool {
		return received.Load() != nil
	})
	if !ok {
		t.Fatal("timed out waiting for frame")
	}

	f := received.Load().(Frame)

	if f.ModuleID == 0 {
		t.Error("received frame ModuleID should be non-zero")
	}
	if f.Name != "painter" {
		t.Errorf("received frame Name = %q, want %q", f.Name, "painter")
	}
	if f.Width != 10 {
		t.Errorf("received frame Width = %d, want %d", f.Width, 10)
	}
	if f.Height != 10 {
		t.Errorf("received frame Height = %d, want %d", f.Height, 10)
	}
	if len(f.Pixels) != 400 {
		t.Errorf("received frame Pixels len = %d, want %d", len(f.Pixels), 400)
	}
	if f.Timestamp != ts {
		t.Errorf("received frame Timestamp = %d, want %d", f.Timestamp, ts)
	}

	// Verify pixel content.
	for i, b := range f.Pixels {
		if b != 0xAA {
			t.Errorf("pixel[%d] = 0x%02X, want 0xAA", i, b)
			break
		}
	}
}

func TestFrameWithDirtyRect(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var received atomic.Value

	srv.OnFrame(func(f Frame) {
		received.Store(f)
	})

	client, err := Dial(addr, WithName("dirty"), WithFrameSize(100, 100))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	dirty := image.Rect(10, 20, 50, 80)

	err = client.PublishFrame(Frame{
		Pixels:    makePixels(100, 100, 0xFF),
		Width:     100,
		Height:    100,
		DirtyRect: dirty,
	})
	if err != nil {
		t.Fatalf("PublishFrame: %v", err)
	}

	ok := waitFor(t, func() bool {
		return received.Load() != nil
	})
	if !ok {
		t.Fatal("timed out waiting for frame")
	}

	f := received.Load().(Frame)

	if f.DirtyRect != dirty {
		t.Errorf("DirtyRect = %v, want %v", f.DirtyRect, dirty)
	}
}

func TestMultipleClients(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr, WithMaxModules(4))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var mu sync.Mutex
	framesByModule := make(map[uint64]Frame)

	srv.OnFrame(func(f Frame) {
		mu.Lock()
		framesByModule[f.ModuleID] = f
		mu.Unlock()
	})

	// Connect three clients.
	clients := make([]*Client, 3)
	names := []string{"alpha", "beta", "gamma"}
	for i, name := range names {
		c, dialErr := Dial(addr, WithName(name), WithFrameSize(8, 8))
		if dialErr != nil {
			t.Fatalf("Dial(%s): %v", name, dialErr)
		}
		clients[i] = c
		t.Cleanup(func() { _ = c.Close() })
	}

	// Each client publishes a frame with a unique fill byte.
	for i, c := range clients {
		fill := byte(i + 1)
		pubErr := c.PublishFrame(Frame{
			Pixels: makePixels(8, 8, fill),
			Width:  8,
			Height: 8,
		})
		if pubErr != nil {
			t.Fatalf("PublishFrame(%s): %v", names[i], pubErr)
		}
	}

	// Wait for all three frames.
	ok := waitFor(t, func() bool {
		mu.Lock()
		n := len(framesByModule)
		mu.Unlock()
		return n >= 3
	})
	if !ok {
		mu.Lock()
		n := len(framesByModule)
		mu.Unlock()
		t.Fatalf("timed out: received %d/3 frames", n)
	}

	// Verify each client got a unique module ID.
	ids := make(map[uint64]bool)
	mu.Lock()
	for id := range framesByModule {
		ids[id] = true
	}
	mu.Unlock()
	if len(ids) != 3 {
		t.Errorf("expected 3 unique module IDs, got %d", len(ids))
	}
}

func TestOnConnectCallback(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var connectedName atomic.Value
	var connectedID atomic.Uint64

	srv.OnConnect(func(id uint64, name string) {
		connectedID.Store(id)
		connectedName.Store(name)
	})

	client, err := Dial(addr, WithName("connector"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ok := waitFor(t, func() bool {
		return connectedName.Load() != nil
	})
	if !ok {
		t.Fatal("timed out waiting for OnConnect")
	}

	if name := connectedName.Load().(string); name != "connector" {
		t.Errorf("OnConnect name = %q, want %q", name, "connector")
	}
	if id := connectedID.Load(); id == 0 {
		t.Error("OnConnect ID should be non-zero")
	}
}

func TestOnDisconnectCallback(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var disconnectedName atomic.Value
	var disconnectedID atomic.Uint64

	srv.OnDisconnect(func(id uint64, name string) {
		disconnectedID.Store(id)
		disconnectedName.Store(name)
	})

	client, err := Dial(addr, WithName("leaver"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Wait for the server to fully process the connection. On CI runners,
	// goroutine scheduling can be slow, so 200ms gives ample headroom.
	time.Sleep(200 * time.Millisecond)

	// Close the client — this triggers disconnect.
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close: %v", err)
	}

	ok := waitFor(t, func() bool {
		return disconnectedName.Load() != nil
	})
	if !ok {
		t.Fatal("timed out waiting for OnDisconnect")
	}

	if name := disconnectedName.Load().(string); name != "leaver" {
		t.Errorf("OnDisconnect name = %q, want %q", name, "leaver")
	}
	if id := disconnectedID.Load(); id == 0 {
		t.Error("OnDisconnect ID should be non-zero")
	}
}

func TestServerClose(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	client, err := Dial(addr, WithName("stranded"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Close server — should disconnect all clients.
	if err := srv.Close(); err != nil {
		t.Fatalf("srv.Close: %v", err)
	}

	// Double close should return ErrClosed.
	if err := srv.Close(); !errors.Is(err, ErrClosed) {
		t.Errorf("second Close = %v, want ErrClosed", err)
	}

	// RequestFrame after close should return ErrClosed.
	if err := srv.RequestFrame(1); !errors.Is(err, ErrClosed) {
		t.Errorf("RequestFrame after close = %v, want ErrClosed", err)
	}

	// Socket file should be removed.
	if _, err := os.Stat(addr); !os.IsNotExist(err) {
		t.Errorf("socket file still exists after Close")
	}
}

func TestClientDoubleClose(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := Dial(addr, WithName("closer"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	if err := client.Close(); !errors.Is(err, ErrClosed) {
		t.Errorf("second Close = %v, want ErrClosed", err)
	}
}

func TestPublishAfterClose(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := Dial(addr, WithName("early-close"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err = client.PublishFrame(Frame{
		Pixels: makePixels(1, 1, 0),
		Width:  1,
		Height: 1,
	})
	if !errors.Is(err, ErrClosed) {
		t.Errorf("PublishFrame after close = %v, want ErrClosed", err)
	}
}

func TestRequestFrameTrigger(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var requestCount atomic.Int32

	// Track connected module ID.
	var moduleID atomic.Uint64
	srv.OnConnect(func(id uint64, _ string) {
		moduleID.Store(id)
	})

	client, err := Dial(addr, WithName("puller"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	client.OnFrameRequest(func() {
		requestCount.Add(1)
	})

	// Wait for connection to be fully established.
	ok := waitFor(t, func() bool {
		return moduleID.Load() != 0
	})
	if !ok {
		t.Fatal("timed out waiting for connection")
	}

	id := moduleID.Load()

	// Server requests a frame.
	if err := srv.RequestFrame(id); err != nil {
		t.Fatalf("RequestFrame: %v", err)
	}

	ok = waitFor(t, func() bool {
		return requestCount.Load() >= 1
	})
	if !ok {
		t.Fatal("timed out waiting for OnFrameRequest callback")
	}

	if n := requestCount.Load(); n != 1 {
		t.Errorf("request count = %d, want 1", n)
	}
}

func TestRequestFrameUnknownModule(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	if err := srv.RequestFrame(999); !errors.Is(err, ErrModuleNotFound) {
		t.Errorf("RequestFrame(999) = %v, want ErrModuleNotFound", err)
	}
}

func TestDialNonExistentServer(t *testing.T) {
	_, err := Dial("/tmp/compose-nonexistent-test.sock")
	if err == nil {
		t.Fatal("Dial to non-existent server should fail")
	}
}

func TestWithMaxModulesLimit(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr, WithMaxModules(1))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	// First client should succeed.
	c1, err := Dial(addr, WithName("first"))
	if err != nil {
		t.Fatalf("Dial first: %v", err)
	}
	t.Cleanup(func() { _ = c1.Close() })

	// Wait for the first client to be fully registered on the server.
	// On CI runners, goroutine scheduling can be slow.
	time.Sleep(200 * time.Millisecond)

	// Second client should be rejected.
	_, err = Dial(addr, WithName("second"))
	if err == nil {
		t.Fatal("Dial second should fail with max modules = 1")
	}
}

func TestFrameSequenceNumbers(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := Dial(addr, WithName("sequencer"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Publish three frames and verify the internal sequence counter increments.
	for i := 0; i < 3; i++ {
		pubErr := client.PublishFrame(Frame{
			Pixels: makePixels(2, 2, byte(i)),
			Width:  2,
			Height: 2,
		})
		if pubErr != nil {
			t.Fatalf("PublishFrame %d: %v", i, pubErr)
		}
	}

	// The sequence counter should be 3 after three frames.
	if seq := client.seq.Load(); seq != 3 {
		t.Errorf("seq = %d, want 3", seq)
	}
}

func TestFunctionalOptionsDefaults(t *testing.T) {
	// Verify server config defaults.
	sCfg := defaultServerConfig()
	if sCfg.maxModules != 16 {
		t.Errorf("default maxModules = %d, want 16", sCfg.maxModules)
	}
	if sCfg.compression != "" {
		t.Errorf("default compression = %q, want empty", sCfg.compression)
	}

	// Verify client config defaults.
	cCfg := defaultClientConfig()
	if cCfg.name != "module" {
		t.Errorf("default name = %q, want %q", cCfg.name, "module")
	}
	if cCfg.width != 400 {
		t.Errorf("default width = %d, want 400", cCfg.width)
	}
	if cCfg.height != 300 {
		t.Errorf("default height = %d, want 300", cCfg.height)
	}
	if cCfg.fps != 1 {
		t.Errorf("default fps = %d, want 1", cCfg.fps)
	}
}

func TestFunctionalOptionsApply(t *testing.T) {
	sCfg := defaultServerConfig()
	WithMaxModules(32)(&sCfg)
	WithCompression("lz4")(&sCfg)

	if sCfg.maxModules != 32 {
		t.Errorf("maxModules = %d, want 32", sCfg.maxModules)
	}
	if sCfg.compression != "lz4" {
		t.Errorf("compression = %q, want %q", sCfg.compression, "lz4")
	}

	cCfg := defaultClientConfig()
	WithName("clock")(&cCfg)
	WithFrameSize(800, 600)(&cCfg)
	WithFPS(60)(&cCfg)

	if cCfg.name != "clock" {
		t.Errorf("name = %q, want %q", cCfg.name, "clock")
	}
	if cCfg.width != 800 {
		t.Errorf("width = %d, want 800", cCfg.width)
	}
	if cCfg.height != 600 {
		t.Errorf("height = %d, want 600", cCfg.height)
	}
	if cCfg.fps != 60 {
		t.Errorf("fps = %d, want 60", cCfg.fps)
	}
}

func TestWithMaxModulesClamping(t *testing.T) {
	cfg := defaultServerConfig()
	WithMaxModules(0)(&cfg)
	if cfg.maxModules != 1 {
		t.Errorf("maxModules after clamp(0) = %d, want 1", cfg.maxModules)
	}

	WithMaxModules(-5)(&cfg)
	if cfg.maxModules != 1 {
		t.Errorf("maxModules after clamp(-5) = %d, want 1", cfg.maxModules)
	}
}

func TestResolveCodec(t *testing.T) {
	raw := resolveCodec("")
	if raw.ID() != 0x00 {
		t.Errorf("resolveCodec(\"\") ID = 0x%02X, want 0x00", raw.ID())
	}

	lz4 := resolveCodec("lz4")
	if lz4.ID() != 0x01 {
		t.Errorf("resolveCodec(\"lz4\") ID = 0x%02X, want 0x01", lz4.ID())
	}

	unknown := resolveCodec("zstd")
	if unknown.ID() != 0x00 {
		t.Errorf("resolveCodec(\"zstd\") ID = 0x%02X, want 0x00 (raw fallback)", unknown.ID())
	}
}

func TestIsDirtyRectValid(t *testing.T) {
	tests := []struct {
		name  string
		rect  image.Rectangle
		valid bool
	}{
		{"zero", image.Rectangle{}, false},
		{"valid", image.Rect(0, 0, 10, 10), true},
		{"empty", image.Rect(5, 5, 5, 5), false},
		// image.Rect canonicalizes, so Rect(10,10,5,5) becomes Rect(5,5,10,10)
		// which is valid. Use raw struct to create a truly empty rect.
		{"point", image.Rect(5, 5, 5, 5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDirtyRectValid(tt.rect)
			if got != tt.valid {
				t.Errorf("isDirtyRectValid(%v) = %v, want %v", tt.rect, got, tt.valid)
			}
		})
	}
}

func TestFrameToHeader(t *testing.T) {
	f := Frame{
		Pixels:    makePixels(10, 10, 0xFF),
		Width:     10,
		Height:    10,
		DirtyRect: image.Rect(2, 3, 8, 9),
		Timestamp: 123456789,
	}

	hdr := frameToHeader(f, 42, 7, resolveCodec(""))

	if hdr.ModuleID != 42 {
		t.Errorf("ModuleID = %d, want 42", hdr.ModuleID)
	}
	if hdr.Sequence != 7 {
		t.Errorf("Sequence = %d, want 7", hdr.Sequence)
	}
	if hdr.Width != 10 {
		t.Errorf("Width = %d, want 10", hdr.Width)
	}
	if hdr.Height != 10 {
		t.Errorf("Height = %d, want 10", hdr.Height)
	}
	if hdr.Stride != 40 {
		t.Errorf("Stride = %d, want 40", hdr.Stride)
	}
	if hdr.TimestampNs != 123456789 {
		t.Errorf("TimestampNs = %d, want 123456789", hdr.TimestampNs)
	}
	if hdr.DirtyX != 2 || hdr.DirtyY != 3 || hdr.DirtyW != 6 || hdr.DirtyH != 6 {
		t.Errorf("DirtyRect = (%d,%d,%d,%d), want (2,3,6,6)",
			hdr.DirtyX, hdr.DirtyY, hdr.DirtyW, hdr.DirtyH)
	}
}

func TestSentinelErrors(t *testing.T) {
	// Verify sentinel errors are distinct and not nil.
	sentinels := []error{ErrClosed, ErrNotAccepted, ErrModuleNotFound, ErrMaxModules, ErrNameTaken}
	for i, e := range sentinels {
		if e == nil {
			t.Errorf("sentinel error %d is nil", i)
		}
	}

	// ErrMaxModules and ErrNameTaken should be the same objects as conn package exports.
	if ErrMaxModules.Error() != "compose: maximum module count reached" {
		t.Errorf("ErrMaxModules message = %q", ErrMaxModules.Error())
	}
	if ErrNameTaken.Error() != "compose: module name already registered" {
		t.Errorf("ErrNameTaken message = %q", ErrNameTaken.Error())
	}
}

func TestHeaderToFrame(t *testing.T) {
	pixels := makePixels(8, 8, 0xBB)

	tests := []struct {
		name      string
		flags     uint8
		dirtyX    uint16
		dirtyY    uint16
		dirtyW    uint16
		dirtyH    uint16
		wantDirty image.Rectangle
	}{
		{
			name:      "no dirty rect (keyframe)",
			flags:     0,
			wantDirty: image.Rectangle{},
		},
		{
			name:      "with dirty rect",
			flags:     0x01, // FlagDirtyValid
			dirtyX:    2,
			dirtyY:    3,
			dirtyW:    4,
			dirtyH:    5,
			wantDirty: image.Rect(2, 3, 6, 8),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hdr := buildTestHeader(tt.flags, tt.dirtyX, tt.dirtyY, tt.dirtyW, tt.dirtyH)

			f := headerToFrame(hdr, pixels, "test")

			if f.ModuleID != 1 {
				t.Errorf("ModuleID = %d, want 1", f.ModuleID)
			}
			if f.Name != "test" {
				t.Errorf("Name = %q, want %q", f.Name, "test")
			}
			if f.Width != 8 {
				t.Errorf("Width = %d, want 8", f.Width)
			}
			if f.Height != 8 {
				t.Errorf("Height = %d, want 8", f.Height)
			}
			if f.DirtyRect != tt.wantDirty {
				t.Errorf("DirtyRect = %v, want %v", f.DirtyRect, tt.wantDirty)
			}
		})
	}
}

// buildTestHeader creates a protocol.Header with commonly used test values.
func buildTestHeader(flags uint8, dirtyX, dirtyY, dirtyW, dirtyH uint16) protocol.Header {
	return protocol.Header{
		Magic:       protocol.Magic,
		Version:     protocol.ProtocolVersion,
		MsgType:     protocol.MsgFrame,
		Flags:       protocol.Flag(flags),
		ModuleID:    1,
		Sequence:    1,
		TimestampNs: time.Now().UnixNano(),
		Width:       8,
		Height:      8,
		Stride:      32,
		DirtyX:      dirtyX,
		DirtyY:      dirtyY,
		DirtyW:      dirtyW,
		DirtyH:      dirtyH,
		PixelFormat: protocol.PixelRGBA8,
		Compression: protocol.CompressionNone,
		PayloadSize: 256,
	}
}

// ---------------------------------------------------------------------------
// Mailbox / Snapshot tests (ADR-002)
// ---------------------------------------------------------------------------

func TestSnapshotBasic(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := Dial(addr, WithName("snap-basic"), WithFrameSize(4, 4))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	pixels := makePixels(4, 4, 0xDD)
	if err := client.PublishFrame(Frame{
		Pixels: pixels,
		Width:  4,
		Height: 4,
	}); err != nil {
		t.Fatalf("PublishFrame: %v", err)
	}

	// Wait for the frame to propagate through the read loop.
	var snap map[uint64]*Frame
	ok := waitFor(t, func() bool {
		snap = srv.Snapshot()
		for _, f := range snap {
			if f != nil {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Fatal("timed out waiting for Snapshot to contain a frame")
	}

	// Verify exactly one module with a non-nil frame.
	var frame *Frame
	for _, f := range snap {
		if f != nil {
			frame = f
		}
	}
	if frame == nil {
		t.Fatal("Snapshot returned no non-nil frames")
	}
	if frame.Width != 4 || frame.Height != 4 {
		t.Errorf("frame dimensions = %dx%d, want 4x4", frame.Width, frame.Height)
	}
	if frame.Sequence != 1 {
		t.Errorf("frame Sequence = %d, want 1", frame.Sequence)
	}
}

func TestSnapshotLatestWins(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	// Track frame count via OnFrame to know when all 3 arrive.
	var frameCount atomic.Int32
	srv.OnFrame(func(_ Frame) {
		frameCount.Add(1)
	})

	client, err := Dial(addr, WithName("snap-latest"), WithFrameSize(2, 2))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Publish 3 frames rapidly with different fill bytes.
	for i := byte(1); i <= 3; i++ {
		if pubErr := client.PublishFrame(Frame{
			Pixels: makePixels(2, 2, i),
			Width:  2,
			Height: 2,
		}); pubErr != nil {
			t.Fatalf("PublishFrame %d: %v", i, pubErr)
		}
	}

	// Wait for all 3 frames to be processed.
	ok := waitFor(t, func() bool {
		return frameCount.Load() >= 3
	})
	if !ok {
		t.Fatalf("timed out: received %d/3 frames", frameCount.Load())
	}

	snap := srv.Snapshot()
	var frame *Frame
	for _, f := range snap {
		if f != nil {
			frame = f
		}
	}
	if frame == nil {
		t.Fatal("Snapshot returned no non-nil frames")
	}

	// Latest frame (3rd publish) should have sequence 3.
	if frame.Sequence != 3 {
		t.Errorf("Snapshot Sequence = %d, want 3 (latest)", frame.Sequence)
	}

	// Verify pixel content matches the last fill byte (0x03).
	for i, b := range frame.Pixels {
		if b != 0x03 {
			t.Errorf("pixel[%d] = 0x%02X, want 0x03 (latest frame)", i, b)
			break
		}
	}
}

func TestSnapshotNoFrames(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var moduleID atomic.Uint64
	srv.OnConnect(func(id uint64, _ string) {
		moduleID.Store(id)
	})

	client, err := Dial(addr, WithName("snap-empty"), WithFrameSize(2, 2))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Wait for the connection to be established.
	ok := waitFor(t, func() bool {
		return moduleID.Load() != 0
	})
	if !ok {
		t.Fatal("timed out waiting for connection")
	}

	// Snapshot before any frames published should have nil entry.
	snap := srv.Snapshot()
	id := moduleID.Load()
	f, exists := snap[id]
	if !exists {
		t.Fatalf("Snapshot missing module %d", id)
	}
	if f != nil {
		t.Errorf("Snapshot for module with no frames should be nil, got Sequence=%d", f.Sequence)
	}
}

func TestSnapshotMultipleModules(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr, WithMaxModules(4))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var frameCount atomic.Int32
	srv.OnFrame(func(_ Frame) {
		frameCount.Add(1)
	})

	// Connect two clients with different frame sizes.
	c1, err := Dial(addr, WithName("mod-a"), WithFrameSize(4, 4))
	if err != nil {
		t.Fatalf("Dial mod-a: %v", err)
	}
	t.Cleanup(func() { _ = c1.Close() })

	c2, err := Dial(addr, WithName("mod-b"), WithFrameSize(8, 8))
	if err != nil {
		t.Fatalf("Dial mod-b: %v", err)
	}
	t.Cleanup(func() { _ = c2.Close() })

	// Publish from both.
	if pubErr := c1.PublishFrame(Frame{
		Pixels: makePixels(4, 4, 0xAA),
		Width:  4,
		Height: 4,
	}); pubErr != nil {
		t.Fatalf("PublishFrame mod-a: %v", pubErr)
	}

	if pubErr := c2.PublishFrame(Frame{
		Pixels: makePixels(8, 8, 0xBB),
		Width:  8,
		Height: 8,
	}); pubErr != nil {
		t.Fatalf("PublishFrame mod-b: %v", pubErr)
	}

	// Wait for both frames.
	ok := waitFor(t, func() bool {
		return frameCount.Load() >= 2
	})
	if !ok {
		t.Fatalf("timed out: received %d/2 frames", frameCount.Load())
	}

	snap := srv.Snapshot()

	// Should have two entries, both non-nil.
	nonNil := 0
	for _, f := range snap {
		if f != nil {
			nonNil++
		}
	}
	if nonNil != 2 {
		t.Errorf("Snapshot has %d non-nil entries, want 2", nonNil)
	}

	// Verify different dimensions.
	dims := make(map[uint32]bool)
	for _, f := range snap {
		if f != nil {
			dims[f.Width] = true
		}
	}
	if !dims[4] || !dims[8] {
		t.Errorf("expected widths 4 and 8 in snapshot, got %v", dims)
	}
}

func TestSnapshotWithPull(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var moduleID atomic.Uint64
	srv.OnConnect(func(id uint64, _ string) {
		moduleID.Store(id)
	})

	client, err := Dial(addr, WithName("snap-pull"), WithFrameSize(2, 2))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Set up pull-based: respond to FrameRequest with a publish.
	client.OnFrameRequest(func() {
		_ = client.PublishFrame(Frame{
			Pixels: makePixels(2, 2, 0xEE),
			Width:  2,
			Height: 2,
		})
	})

	// Wait for the connection.
	ok := waitFor(t, func() bool {
		return moduleID.Load() != 0
	})
	if !ok {
		t.Fatal("timed out waiting for connection")
	}

	// Server requests a frame (pull model).
	id := moduleID.Load()
	if reqErr := srv.RequestFrame(id); reqErr != nil {
		t.Fatalf("RequestFrame: %v", reqErr)
	}

	// Wait for the pulled frame to appear in Snapshot.
	ok = waitFor(t, func() bool {
		snap := srv.Snapshot()
		f := snap[id]
		return f != nil
	})
	if !ok {
		t.Fatal("timed out waiting for pull frame in Snapshot")
	}

	snap := srv.Snapshot()
	f := snap[id]
	if f.Width != 2 || f.Height != 2 {
		t.Errorf("pulled frame dimensions = %dx%d, want 2x2", f.Width, f.Height)
	}
	if f.Pixels[0] != 0xEE {
		t.Errorf("pulled frame pixel[0] = 0x%02X, want 0xEE", f.Pixels[0])
	}
}

func TestSnapshotConcurrent(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	client, err := Dial(addr, WithName("snap-race"), WithFrameSize(2, 2))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Parallel writers: push frames from multiple goroutines.
	const writers = 4
	const framesPerWriter = 10
	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < framesPerWriter; i++ {
				_ = client.PublishFrame(Frame{
					Pixels: makePixels(2, 2, byte(id*framesPerWriter+i)),
					Width:  2,
					Height: 2,
				})
			}
		}(w)
	}

	// Parallel reader: call Snapshot concurrently with writes.
	const readers = 4
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < framesPerWriter*2; i++ {
				snap := srv.Snapshot()
				// Snapshot must never panic, and all entries are either nil or valid.
				for _, f := range snap {
					if f != nil {
						_ = f.Sequence
						_ = f.Width
					}
				}
			}
		}()
	}

	wg.Wait()
}

func TestOnFrameStillFires(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	var callbackCount atomic.Int32
	srv.OnFrame(func(_ Frame) {
		callbackCount.Add(1)
	})

	client, err := Dial(addr, WithName("snap-compat"), WithFrameSize(2, 2))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Publish 5 frames.
	const n = 5
	for i := 0; i < n; i++ {
		if pubErr := client.PublishFrame(Frame{
			Pixels: makePixels(2, 2, byte(i)),
			Width:  2,
			Height: 2,
		}); pubErr != nil {
			t.Fatalf("PublishFrame %d: %v", i, pubErr)
		}
	}

	// Wait for all 5 callbacks to fire.
	ok := waitFor(t, func() bool {
		return callbackCount.Load() >= n
	})
	if !ok {
		t.Fatalf("OnFrame fired %d/%d times", callbackCount.Load(), n)
	}

	// Verify: OnFrame fires for EVERY frame, not just latest.
	if got := callbackCount.Load(); got != n {
		t.Errorf("OnFrame count = %d, want %d (every frame)", got, n)
	}

	// Snapshot returns only the latest.
	snap := srv.Snapshot()
	var frame *Frame
	for _, f := range snap {
		if f != nil {
			frame = f
		}
	}
	if frame == nil {
		t.Fatal("Snapshot returned no frames")
	}
	if frame.Sequence != n {
		t.Errorf("Snapshot Sequence = %d, want %d (latest)", frame.Sequence, n)
	}
}

func TestHeaderToFrameSequence(t *testing.T) {
	pixels := makePixels(4, 4, 0xFF)
	hdr := protocol.Header{
		Magic:       protocol.Magic,
		Version:     protocol.ProtocolVersion,
		MsgType:     protocol.MsgFrame,
		ModuleID:    42,
		Sequence:    99,
		TimestampNs: 1234,
		Width:       4,
		Height:      4,
		Stride:      16,
	}

	f := headerToFrame(hdr, pixels, "seqtest")

	if f.Sequence != 99 {
		t.Errorf("Frame.Sequence = %d, want 99", f.Sequence)
	}
	if f.ModuleID != 42 {
		t.Errorf("Frame.ModuleID = %d, want 42", f.ModuleID)
	}
}
