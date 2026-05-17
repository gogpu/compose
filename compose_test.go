package compose

import (
	"image"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogpu/compose/internal/protocol"
)

// tempSocket returns a unique temporary Unix socket path.
// On Windows, uses a path under the temp directory.
func tempSocket(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "compose-test.sock")
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

// waitFor polls a condition function until it returns true or the timeout
// expires. Returns true if the condition was met.
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
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
	ok := waitFor(t, 2*time.Second, func() bool {
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

	ok := waitFor(t, 2*time.Second, func() bool {
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
	ok := waitFor(t, 2*time.Second, func() bool {
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

	ok := waitFor(t, 2*time.Second, func() bool {
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

	// Wait a moment for the server to process the connection.
	time.Sleep(50 * time.Millisecond)

	// Close the client — this triggers disconnect.
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close: %v", err)
	}

	ok := waitFor(t, 2*time.Second, func() bool {
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
	if err := srv.Close(); err != ErrClosed {
		t.Errorf("second Close = %v, want ErrClosed", err)
	}

	// RequestFrame after close should return ErrClosed.
	if err := srv.RequestFrame(1); err != ErrClosed {
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

	if err := client.Close(); err != ErrClosed {
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
	if err != ErrClosed {
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
	ok := waitFor(t, 2*time.Second, func() bool {
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

	ok = waitFor(t, 2*time.Second, func() bool {
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

	if err := srv.RequestFrame(999); err != ErrModuleNotFound {
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

	// Wait for first client to be registered.
	time.Sleep(50 * time.Millisecond)

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
	errors := []error{ErrClosed, ErrNotAccepted, ErrModuleNotFound, ErrMaxModules, ErrNameTaken}
	for i, e := range errors {
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
