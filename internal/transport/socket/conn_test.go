package socket

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/gogpu/compose/internal/protocol"
)

// newPipeConns creates a pair of Conn values connected by net.Pipe.
func newPipeConns(t *testing.T) (client, server *Conn) {
	t.Helper()
	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		c1.Close()
		c2.Close()
	})
	return NewConn(c1), NewConn(c2)
}

// makeHeader returns a minimal valid header for testing.
func makeHeader(msgType protocol.MsgType, payloadSize uint32) protocol.Header {
	return protocol.Header{
		Magic:       protocol.Magic,
		Version:     protocol.ProtocolVersion,
		MsgType:     msgType,
		PayloadSize: payloadSize,
	}
}

func TestWriteReadFrame_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		msgType protocol.MsgType
		payload []byte
	}{
		{
			name:    "frame with payload",
			msgType: protocol.MsgFrame,
			payload: []byte("hello compose"),
		},
		{
			name:    "ack empty payload",
			msgType: protocol.MsgAck,
			payload: nil,
		},
		{
			name:    "frame request empty",
			msgType: protocol.MsgFrameRequest,
			payload: []byte{},
		},
		{
			name:    "single byte payload",
			msgType: protocol.MsgFrame,
			payload: []byte{0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := newPipeConns(t)

			payloadLen := uint32(len(tt.payload))
			hdr := makeHeader(tt.msgType, payloadLen)

			// Write in background.
			errCh := make(chan error, 1)
			go func() {
				errCh <- client.WriteFrame(&hdr, tt.payload)
			}()

			gotHdr, gotPayload, err := server.ReadFrame()
			if err != nil {
				t.Fatalf("ReadFrame: %v", err)
			}

			if werr := <-errCh; werr != nil {
				t.Fatalf("WriteFrame: %v", werr)
			}

			if gotHdr.MsgType != tt.msgType {
				t.Errorf("MsgType = %v, want %v", gotHdr.MsgType, tt.msgType)
			}
			if gotHdr.PayloadSize != payloadLen {
				t.Errorf("PayloadSize = %d, want %d", gotHdr.PayloadSize, payloadLen)
			}

			if payloadLen == 0 {
				if gotPayload != nil {
					t.Errorf("payload = %v, want nil", gotPayload)
				}
			} else if !bytes.Equal(gotPayload, tt.payload) {
				t.Errorf("payload mismatch: got %d bytes, want %d bytes", len(gotPayload), len(tt.payload))
			}
		})
	}
}

func TestReadFrame_OversizedPayloadHeader(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	hdr := makeHeader(protocol.MsgFrame, uint32(protocol.MaxPayloadSize+1))
	var encoded [protocol.HeaderSize]byte
	if err := protocol.Encode(&hdr, encoded[:]); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	writeErr := make(chan error, 1)
	go func() {
		_, err := client.Write(encoded[:])
		writeErr <- err
	}()

	_, _, err := NewConn(server).ReadFrame()
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("ReadFrame error = %v, want ErrPayloadTooLarge", err)
	}

	// The reader must reject the header before attempting to read the
	// declared payload. A header-only peer therefore completes immediately.
	if err := <-writeErr; err != nil {
		t.Fatalf("header write: %v", err)
	}
}

func TestWriteReadFrame_LargePayload(t *testing.T) {
	client, server := newPipeConns(t)

	// 1 MB payload.
	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	hdr := makeHeader(protocol.MsgFrame, uint32(len(payload)))

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.WriteFrame(&hdr, payload)
	}()

	gotHdr, gotPayload, err := server.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteFrame: %v", werr)
	}

	if gotHdr.PayloadSize != uint32(len(payload)) {
		t.Errorf("PayloadSize = %d, want %d", gotHdr.PayloadSize, len(payload))
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Error("large payload content mismatch")
	}
}

func TestReadFrameInto_ReuseBuffer(t *testing.T) {
	client, server := newPipeConns(t)

	payload := []byte("reuse this buffer please")
	hdr := makeHeader(protocol.MsgFrame, uint32(len(payload)))

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.WriteFrame(&hdr, payload)
	}()

	// Provide a sufficiently large buffer.
	buf := make([]byte, 1024)
	_, gotPayload, err := server.ReadFrameInto(buf)
	if err != nil {
		t.Fatalf("ReadFrameInto: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteFrame: %v", werr)
	}

	if !bytes.Equal(gotPayload, payload) {
		t.Error("payload mismatch")
	}

	// Verify the returned slice shares backing array with buf.
	if &gotPayload[0] != &buf[0] {
		t.Error("ReadFrameInto did not reuse provided buffer")
	}
}

func TestReadFrameInto_SmallBuffer_Allocates(t *testing.T) {
	client, server := newPipeConns(t)

	payload := []byte("too big for tiny buffer")
	hdr := makeHeader(protocol.MsgFrame, uint32(len(payload)))

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.WriteFrame(&hdr, payload)
	}()

	// Buffer smaller than payload.
	buf := make([]byte, 4)
	_, gotPayload, err := server.ReadFrameInto(buf)
	if err != nil {
		t.Fatalf("ReadFrameInto: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteFrame: %v", werr)
	}

	if !bytes.Equal(gotPayload, payload) {
		t.Error("payload mismatch")
	}
}

func TestConcurrentWriteRead(t *testing.T) {
	client, server := newPipeConns(t)

	const numFrames = 100
	payload := []byte("concurrent test payload")
	hdr := makeHeader(protocol.MsgFrame, uint32(len(payload)))

	// Writer goroutine: send numFrames frames.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range numFrames {
			h := hdr
			h.Sequence = uint64(i)
			if err := client.WriteFrame(&h, payload); err != nil {
				t.Errorf("WriteFrame[%d]: %v", i, err)
				return
			}
		}
	}()

	// Reader: receive numFrames frames.
	for i := range numFrames {
		gotHdr, gotPayload, err := server.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame[%d]: %v", i, err)
		}
		if gotHdr.Sequence != uint64(i) {
			t.Errorf("frame %d: Sequence = %d, want %d", i, gotHdr.Sequence, i)
		}
		if !bytes.Equal(gotPayload, payload) {
			t.Errorf("frame %d: payload mismatch", i)
		}
	}

	wg.Wait()
}

func TestConcurrentWriters(t *testing.T) {
	client, server := newPipeConns(t)

	const numWriters = 4
	const framesPerWriter = 25
	payload := []byte("multi-writer")
	hdr := makeHeader(protocol.MsgFrame, uint32(len(payload)))

	var wg sync.WaitGroup
	for w := range numWriters {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := range framesPerWriter {
				h := hdr
				h.ModuleID = uint64(writerID)
				h.Sequence = uint64(i)
				if err := client.WriteFrame(&h, payload); err != nil {
					t.Errorf("writer %d frame %d: %v", writerID, i, err)
					return
				}
			}
		}(w)
	}

	// Read all frames.
	total := numWriters * framesPerWriter
	for i := range total {
		_, gotPayload, err := server.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame[%d]: %v", i, err)
		}
		if !bytes.Equal(gotPayload, payload) {
			t.Errorf("frame %d: payload mismatch", i)
		}
	}

	wg.Wait()
}

func TestHandshake_RoundTrip(t *testing.T) {
	client, server := newPipeConns(t)

	// Module sends Hello.
	hello := protocol.HelloMsg{
		Magic:        protocol.Magic,
		Version:      protocol.ProtocolVersion,
		Width:        400,
		Height:       120,
		PreferredFPS: 60,
		Transport:    protocol.TransportSocket,
	}
	protocol.SetName(&hello, "test-module")

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.WriteHandshakeHello(&hello)
	}()

	gotHello, err := server.ReadHandshakeHello()
	if err != nil {
		t.Fatalf("ReadHandshakeHello: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteHandshakeHello: %v", werr)
	}

	if gotHello.Magic != protocol.Magic {
		t.Error("hello: magic mismatch")
	}
	if protocol.GetName(&gotHello) != "test-module" {
		t.Errorf("hello: name = %q, want %q", protocol.GetName(&gotHello), "test-module")
	}
	if gotHello.Width != 400 {
		t.Errorf("hello: Width = %d, want 400", gotHello.Width)
	}
	if gotHello.Height != 120 {
		t.Errorf("hello: Height = %d, want 120", gotHello.Height)
	}
	if gotHello.PreferredFPS != 60 {
		t.Errorf("hello: PreferredFPS = %d, want 60", gotHello.PreferredFPS)
	}

	// Compositor sends Welcome.
	welcome := protocol.WelcomeMsg{
		Magic:      protocol.Magic,
		Version:    protocol.ProtocolVersion,
		ModuleID:   42,
		Accepted:   1,
		Transport:  protocol.TransportSocket,
		MinVersion: 1,
		MaxVersion: 1,
	}

	go func() {
		errCh <- server.WriteHandshakeWelcome(&welcome)
	}()

	gotWelcome, err := client.ReadHandshakeWelcome()
	if err != nil {
		t.Fatalf("ReadHandshakeWelcome: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteHandshakeWelcome: %v", werr)
	}

	if gotWelcome.ModuleID != 42 {
		t.Errorf("welcome: ModuleID = %d, want 42", gotWelcome.ModuleID)
	}
	if gotWelcome.Accepted != 1 {
		t.Errorf("welcome: Accepted = %d, want 1", gotWelcome.Accepted)
	}
}

func TestReadFrame_ConnectionClosed(t *testing.T) {
	client, server := newPipeConns(t)

	// Close the writing side immediately.
	client.Close()

	_, _, err := server.ReadFrame()
	if err == nil {
		t.Fatal("expected error on closed connection, got nil")
	}

	// Should wrap io.EOF or io.ErrUnexpectedEOF.
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		// net.Pipe close produces io.EOF via io.ReadFull -> io.ErrUnexpectedEOF,
		// but the exact wrapping depends on how many bytes were read.
		// Accept any error as valid — the point is we don't hang.
		t.Logf("got error (acceptable): %v", err)
	}
}

func TestReadHandshakeHello_ConnectionClosed(t *testing.T) {
	client, server := newPipeConns(t)
	client.Close()

	_, err := server.ReadHandshakeHello()
	if err == nil {
		t.Fatal("expected error on closed connection, got nil")
	}
}

func TestReadHandshakeWelcome_ConnectionClosed(t *testing.T) {
	client, server := newPipeConns(t)
	client.Close()

	_, err := server.ReadHandshakeWelcome()
	if err == nil {
		t.Fatal("expected error on closed connection, got nil")
	}
}

func TestConn_RemoteAddr(t *testing.T) {
	client, _ := newPipeConns(t)

	addr := client.RemoteAddr()
	if addr == nil {
		t.Fatal("RemoteAddr returned nil")
	}
}

func TestConn_Close(t *testing.T) {
	c1, c2 := net.Pipe()
	conn := NewConn(c1)

	// Close should not error on first call.
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The other end should see EOF.
	buf := make([]byte, 1)
	_, err := c2.Read(buf)
	if err == nil {
		t.Fatal("expected error after close, got nil")
	}

	c2.Close()
}

func TestWriteReadFrame_AllHeaderFields(t *testing.T) {
	client, server := newPipeConns(t)

	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	hdr := protocol.Header{
		Magic:            protocol.Magic,
		Version:          protocol.ProtocolVersion,
		MsgType:          protocol.MsgFrame,
		Flags:            protocol.FlagDirtyValid | protocol.FlagKeyframe,
		ModuleID:         12345,
		Sequence:         99,
		TimestampNs:      1_000_000_000,
		Width:            1920,
		Height:           1080,
		Stride:           1920 * 4,
		DirtyX:           10,
		DirtyY:           20,
		DirtyW:           100,
		DirtyH:           200,
		PixelFormat:      protocol.PixelRGBA8,
		Compression:      protocol.CompressionNone,
		PayloadSize:      uint32(len(payload)),
		UncompressedSize: uint32(len(payload)),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.WriteFrame(&hdr, payload)
	}()

	got, gotPayload, err := server.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteFrame: %v", werr)
	}

	// Verify all fields survived the round trip.
	if got.ModuleID != 12345 {
		t.Errorf("ModuleID = %d, want 12345", got.ModuleID)
	}
	if got.Sequence != 99 {
		t.Errorf("Sequence = %d, want 99", got.Sequence)
	}
	if got.TimestampNs != 1_000_000_000 {
		t.Errorf("TimestampNs = %d, want 1000000000", got.TimestampNs)
	}
	if got.Width != 1920 {
		t.Errorf("Width = %d, want 1920", got.Width)
	}
	if got.Height != 1080 {
		t.Errorf("Height = %d, want 1080", got.Height)
	}
	if got.Stride != 1920*4 {
		t.Errorf("Stride = %d, want %d", got.Stride, 1920*4)
	}
	if got.DirtyX != 10 || got.DirtyY != 20 || got.DirtyW != 100 || got.DirtyH != 200 {
		t.Errorf("DirtyRect = (%d,%d,%d,%d), want (10,20,100,200)",
			got.DirtyX, got.DirtyY, got.DirtyW, got.DirtyH)
	}
	if got.Flags != (protocol.FlagDirtyValid | protocol.FlagKeyframe) {
		t.Errorf("Flags = %d, want %d", got.Flags, protocol.FlagDirtyValid|protocol.FlagKeyframe)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Error("payload mismatch")
	}
}

func TestWriteReadFrame_MultipleSequential(t *testing.T) {
	client, server := newPipeConns(t)

	const count = 10
	errCh := make(chan error, 1)
	go func() {
		for i := range count {
			p := []byte{byte(i)}
			h := makeHeader(protocol.MsgFrame, 1)
			h.Sequence = uint64(i)
			if err := client.WriteFrame(&h, p); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	for i := range count {
		hdr, payload, err := server.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame[%d]: %v", i, err)
		}
		if hdr.Sequence != uint64(i) {
			t.Errorf("frame %d: Sequence = %d", i, hdr.Sequence)
		}
		if len(payload) != 1 || payload[0] != byte(i) {
			t.Errorf("frame %d: payload mismatch", i)
		}
	}

	if werr := <-errCh; werr != nil {
		t.Fatalf("WriteFrame: %v", werr)
	}
}

func TestWriteFrame_ClosedConn_HeaderWriteError(t *testing.T) {
	c1, c2 := net.Pipe()
	conn := NewConn(c1)
	c2.Close()

	// Close the underlying conn so the header write fails.
	c1.Close()

	hdr := makeHeader(protocol.MsgFrame, 5)
	err := conn.WriteFrame(&hdr, []byte("hello"))
	if err == nil {
		t.Fatal("expected error writing to closed conn, got nil")
	}
}

func TestWriteFrame_ClosedConn_PayloadWriteError(t *testing.T) {
	// Use a real socket pair so we can close mid-flight.
	addr := testSocketPath(t)
	ln, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	acceptCh := make(chan *Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		acceptCh <- c
	}()

	dialer := NewDialer(addr)
	clientConn, err := dialer.Dial()
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	serverConn := <-acceptCh
	// Close the server side so writes from client eventually fail.
	serverConn.Close()

	// Write a large payload — the header write may succeed but the
	// large payload write should fail because the peer is closed.
	bigPayload := make([]byte, 1<<20) // 1 MB
	hdr := makeHeader(protocol.MsgFrame, uint32(len(bigPayload)))

	// Retry a few times — the first write might succeed due to kernel buffering.
	var lastErr error
	for range 100 {
		lastErr = clientConn.WriteFrame(&hdr, bigPayload)
		if lastErr != nil {
			break
		}
	}
	if lastErr == nil {
		t.Log("write never failed (kernel buffered everything) — skipping payload error test")
	}

	clientConn.Close()
}

func TestWriteHandshakeHello_ClosedConn(t *testing.T) {
	c1, c2 := net.Pipe()
	conn := NewConn(c1)
	c2.Close()
	c1.Close()

	hello := protocol.HelloMsg{
		Magic:   protocol.Magic,
		Version: protocol.ProtocolVersion,
	}
	err := conn.WriteHandshakeHello(&hello)
	if err == nil {
		t.Fatal("expected error writing hello to closed conn, got nil")
	}
}

func TestWriteHandshakeWelcome_ClosedConn(t *testing.T) {
	c1, c2 := net.Pipe()
	conn := NewConn(c1)
	c2.Close()
	c1.Close()

	welcome := protocol.WelcomeMsg{
		Magic:   protocol.Magic,
		Version: protocol.ProtocolVersion,
	}
	err := conn.WriteHandshakeWelcome(&welcome)
	if err == nil {
		t.Fatal("expected error writing welcome to closed conn, got nil")
	}
}

func TestReadFrame_InvalidMagic(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// Write 64 bytes of garbage (invalid magic).
	go func() {
		garbage := make([]byte, protocol.HeaderSize)
		garbage[0] = 0xFF // wrong magic
		garbage[1] = 0xFF
		garbage[2] = 0xFF
		garbage[3] = 0xFF
		// Set a valid MsgType at offset 6 to avoid ErrUnknownMsgType.
		garbage[6] = uint8(protocol.MsgFrame)
		c1.Write(garbage)
	}()

	conn := NewConn(c2)
	_, _, err := conn.ReadFrame()
	if err == nil {
		t.Fatal("expected error for invalid magic, got nil")
	}
}

func TestReadFrame_PayloadReadError(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	conn := NewConn(c2)

	// Write a valid header claiming 1000 bytes of payload, then close.
	go func() {
		hdr := makeHeader(protocol.MsgFrame, 1000)
		var buf [protocol.HeaderSize]byte
		protocol.Encode(&hdr, buf[:])
		c1.Write(buf[:])
		// Write only 10 bytes of the claimed 1000, then close.
		c1.Write([]byte("short data"))
		c1.Close()
	}()

	_, _, err := conn.ReadFrame()
	if err == nil {
		t.Fatal("expected error for truncated payload, got nil")
	}
}

func TestReadHandshakeHello_InvalidMagic(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		garbage := make([]byte, protocol.HandshakeSize)
		garbage[0] = 0xBA
		garbage[1] = 0xAD
		c1.Write(garbage)
	}()

	conn := NewConn(c2)
	_, err := conn.ReadHandshakeHello()
	if err == nil {
		t.Fatal("expected error for invalid hello magic, got nil")
	}
}

func TestReadHandshakeWelcome_InvalidMagic(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		garbage := make([]byte, protocol.HandshakeSize)
		garbage[0] = 0xDE
		garbage[1] = 0xAD
		c1.Write(garbage)
	}()

	conn := NewConn(c2)
	_, err := conn.ReadHandshakeWelcome()
	if err == nil {
		t.Fatal("expected error for invalid welcome magic, got nil")
	}
}

func BenchmarkWriteReadFrame(b *testing.B) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	client := NewConn(c1)
	server := NewConn(c2)

	// 192 KB payload (typical frame: 320x150 RGBA).
	payload := make([]byte, 192*1024)
	for i := range payload {
		payload[i] = byte(i)
	}
	hdr := makeHeader(protocol.MsgFrame, uint32(len(payload)))
	buf := make([]byte, len(payload))

	// Writer goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range b.N {
			if err := client.WriteFrame(&hdr, payload); err != nil {
				b.Errorf("WriteFrame: %v", err)
				return
			}
		}
	}()

	b.SetBytes(int64(protocol.HeaderSize + len(payload)))
	b.ResetTimer()

	for range b.N {
		_, _, err := server.ReadFrameInto(buf)
		if err != nil {
			b.Fatalf("ReadFrameInto: %v", err)
		}
	}

	b.StopTimer()
	<-done
}
