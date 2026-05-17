package socket

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gogpu/compose/internal/protocol"
)

// testSocketPath returns a temporary Unix socket path for testing.
func testSocketPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.sock")
}

func TestListen_Accept_Close(t *testing.T) {
	addr := testSocketPath(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	// Accept in background.
	type result struct {
		conn *Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := ln.Accept()
		ch <- result{c, err}
	}()

	// Connect from client side.
	raw, err := net.Dial("unix", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer raw.Close()

	r := <-ch
	if r.err != nil {
		t.Fatalf("Accept: %v", r.err)
	}
	defer r.conn.Close()

	// Verify the accepted connection is usable.
	if r.conn.RemoteAddr() == nil {
		t.Error("accepted conn RemoteAddr is nil")
	}

	// Close listener.
	if err := ln.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Socket file should be removed.
	if _, err := os.Stat(addr); !os.IsNotExist(err) {
		t.Errorf("socket file still exists after Close: %v", err)
	}
}

func TestListen_StaleSocketCleanup(t *testing.T) {
	addr := testSocketPath(t)

	// Create a stale socket file.
	if err := os.WriteFile(addr, []byte("stale"), 0o600); err != nil {
		t.Fatalf("create stale file: %v", err)
	}

	// Listen should remove the stale file and succeed.
	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen with stale file: %v", err)
	}
	defer ln.Close()

	// Verify the listener is functional.
	ch := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
		ch <- err
	}()

	raw, err := net.Dial("unix", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	raw.Close()

	if err := <-ch; err != nil {
		t.Fatalf("Accept: %v", err)
	}
}

func TestListen_MultipleConnections(t *testing.T) {
	addr := testSocketPath(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	const numClients = 5

	// Accept goroutine.
	accepted := make([]*Conn, 0, numClients)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range numClients {
			c, err := ln.Accept()
			if err != nil {
				t.Errorf("Accept: %v", err)
				return
			}
			mu.Lock()
			accepted = append(accepted, c)
			mu.Unlock()
		}
	}()

	// Connect numClients clients.
	clients := make([]net.Conn, numClients)
	for i := range numClients {
		c, err := net.Dial("unix", addr)
		if err != nil {
			t.Fatalf("Dial[%d]: %v", i, err)
		}
		clients[i] = c
	}

	wg.Wait()

	mu.Lock()
	if len(accepted) != numClients {
		t.Errorf("accepted %d connections, want %d", len(accepted), numClients)
	}
	mu.Unlock()

	// Cleanup.
	for _, c := range clients {
		c.Close()
	}
	mu.Lock()
	for _, c := range accepted {
		c.Close()
	}
	mu.Unlock()
}

func TestListen_CloseWhileAcceptBlocking(t *testing.T) {
	addr := testSocketPath(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	// Start blocking Accept.
	ch := make(chan error, 1)
	go func() {
		_, err := ln.Accept()
		ch <- err
	}()

	// Give Accept time to block.
	time.Sleep(50 * time.Millisecond)

	// Close should unblock Accept.
	if err := ln.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-ch:
		if err == nil {
			t.Error("Accept should return error after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not unblock after Close (timeout)")
	}
}

func TestListen_Addr(t *testing.T) {
	addr := testSocketPath(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	got := ln.Addr()
	if got == nil {
		t.Fatal("Addr returned nil")
	}
	if got.Network() != "unix" {
		t.Errorf("Network = %q, want %q", got.Network(), "unix")
	}
}

func TestListen_AcceptHandshake(t *testing.T) {
	addr := testSocketPath(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	// Accept in background.
	type result struct {
		conn *Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := ln.Accept()
		ch <- result{c, err}
	}()

	// Connect and send Hello.
	dialer := NewDialer(addr)
	clientConn, err := dialer.Dial()
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer clientConn.Close()

	r := <-ch
	if r.err != nil {
		t.Fatalf("Accept: %v", r.err)
	}
	defer r.conn.Close()

	// Handshake: client Hello -> server Welcome.
	hello := protocol.HelloMsg{
		Magic:        protocol.Magic,
		Version:      protocol.ProtocolVersion,
		Width:        800,
		Height:       600,
		PreferredFPS: 30,
		Transport:    protocol.TransportSocket,
	}
	protocol.SetName(&hello, "gallery")

	errCh := make(chan error, 1)
	go func() {
		errCh <- clientConn.WriteHandshakeHello(&hello)
	}()

	gotHello, err := r.conn.ReadHandshakeHello()
	if err != nil {
		t.Fatalf("ReadHandshakeHello: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteHandshakeHello: %v", werr)
	}

	if protocol.GetName(&gotHello) != "gallery" {
		t.Errorf("name = %q, want %q", protocol.GetName(&gotHello), "gallery")
	}

	// Server responds with Welcome.
	welcome := protocol.WelcomeMsg{
		Magic:      protocol.Magic,
		Version:    protocol.ProtocolVersion,
		ModuleID:   1,
		Accepted:   1,
		Transport:  protocol.TransportSocket,
		MinVersion: 1,
		MaxVersion: 1,
	}

	go func() {
		errCh <- r.conn.WriteHandshakeWelcome(&welcome)
	}()

	gotWelcome, err := clientConn.ReadHandshakeWelcome()
	if err != nil {
		t.Fatalf("ReadHandshakeWelcome: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteHandshakeWelcome: %v", werr)
	}

	if gotWelcome.ModuleID != 1 {
		t.Errorf("ModuleID = %d, want 1", gotWelcome.ModuleID)
	}
	if gotWelcome.Accepted != 1 {
		t.Errorf("Accepted = %d, want 1", gotWelcome.Accepted)
	}
}

func TestListen_InvalidPath(t *testing.T) {
	// Path that is too long or invalid on most systems.
	addr := filepath.Join(t.TempDir(), string(make([]byte, 300)))
	_, err := Listen(addr)
	if err == nil {
		t.Fatal("expected error for invalid socket path, got nil")
	}
}
