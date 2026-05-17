package socket

import (
	"testing"
	"time"

	"github.com/gogpu/compose/internal/protocol"
)

func TestNewDialer(t *testing.T) {
	d := NewDialer("/tmp/test.sock")
	if d.addr != "/tmp/test.sock" {
		t.Errorf("addr = %q, want %q", d.addr, "/tmp/test.sock")
	}
	if d.timeout != defaultDialTimeout {
		t.Errorf("timeout = %v, want %v", d.timeout, defaultDialTimeout)
	}
}

func TestDial_Success(t *testing.T) {
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

	dialer := NewDialer(addr)
	conn, err := dialer.Dial()
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	r := <-ch
	if r.err != nil {
		t.Fatalf("Accept: %v", r.err)
	}
	t.Cleanup(func() { _ = r.conn.Close() })
}

func TestDial_Timeout(t *testing.T) {
	// Use a non-existent socket path.
	addr := testSocketPath(t)

	dialer := NewDialer(addr)
	_, err := dialer.DialWithTimeout(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected error dialing non-existent socket, got nil")
	}
}

func TestDialWithTimeout_CustomTimeout(t *testing.T) {
	addr := testSocketPath(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	// Accept in background.
	ch := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
		ch <- err
	}()

	dialer := NewDialer(addr)
	conn, err := dialer.DialWithTimeout(2 * time.Second)
	if err != nil {
		t.Fatalf("DialWithTimeout: %v", err)
	}
	conn.Close()

	if err := <-ch; err != nil {
		t.Fatalf("Accept: %v", err)
	}
}

func TestDial_FullHandshake(t *testing.T) {
	addr := testSocketPath(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	// Server Accept goroutine.
	type result struct {
		conn *Conn
		err  error
	}
	acceptCh := make(chan result, 1)
	go func() {
		c, err := ln.Accept()
		acceptCh <- result{c, err}
	}()

	// Client connects.
	dialer := NewDialer(addr)
	clientConn, err := dialer.Dial()
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer clientConn.Close()

	r := <-acceptCh
	if r.err != nil {
		t.Fatalf("Accept: %v", r.err)
	}
	serverConn := r.conn
	defer serverConn.Close()

	// Full handshake: Hello then Welcome.
	hello := protocol.HelloMsg{
		Magic:        protocol.Magic,
		Version:      protocol.ProtocolVersion,
		Width:        320,
		Height:       240,
		PreferredFPS: 1,
		Transport:    protocol.TransportSocket,
	}
	protocol.SetName(&hello, "clock")

	errCh := make(chan error, 1)
	go func() {
		errCh <- clientConn.WriteHandshakeHello(&hello)
	}()

	gotHello, err := serverConn.ReadHandshakeHello()
	if err != nil {
		t.Fatalf("ReadHandshakeHello: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteHandshakeHello: %v", werr)
	}

	if protocol.GetName(&gotHello) != "clock" {
		t.Errorf("name = %q, want %q", protocol.GetName(&gotHello), "clock")
	}

	welcome := protocol.WelcomeMsg{
		Magic:      protocol.Magic,
		Version:    protocol.ProtocolVersion,
		ModuleID:   7,
		Accepted:   1,
		Transport:  protocol.TransportSocket,
		MinVersion: 1,
		MaxVersion: 1,
	}

	go func() {
		errCh <- serverConn.WriteHandshakeWelcome(&welcome)
	}()

	gotWelcome, err := clientConn.ReadHandshakeWelcome()
	if err != nil {
		t.Fatalf("ReadHandshakeWelcome: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteHandshakeWelcome: %v", werr)
	}

	if gotWelcome.ModuleID != 7 {
		t.Errorf("ModuleID = %d, want 7", gotWelcome.ModuleID)
	}
	if gotWelcome.Accepted != 1 {
		t.Errorf("Accepted = %d, want 1", gotWelcome.Accepted)
	}

	// Post-handshake: send a frame.
	payload := []byte("pixel data here")
	hdr := protocol.Header{
		Magic:       protocol.Magic,
		Version:     protocol.ProtocolVersion,
		MsgType:     protocol.MsgFrame,
		ModuleID:    7,
		Sequence:    1,
		PayloadSize: uint32(len(payload)),
	}

	go func() {
		errCh <- clientConn.WriteFrame(&hdr, payload)
	}()

	gotHdr, gotPayload, err := serverConn.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteFrame: %v", werr)
	}

	if gotHdr.ModuleID != 7 {
		t.Errorf("frame ModuleID = %d, want 7", gotHdr.ModuleID)
	}
	if string(gotPayload) != "pixel data here" {
		t.Errorf("frame payload = %q, want %q", gotPayload, "pixel data here")
	}
}

func TestDial_ZeroTimeout(t *testing.T) {
	addr := testSocketPath(t)

	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	ch := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
		ch <- err
	}()

	dialer := NewDialer(addr)
	// Zero timeout = no deadline.
	conn, err := dialer.DialWithTimeout(0)
	if err != nil {
		t.Fatalf("DialWithTimeout(0): %v", err)
	}
	conn.Close()

	if err := <-ch; err != nil {
		t.Fatalf("Accept: %v", err)
	}
}
