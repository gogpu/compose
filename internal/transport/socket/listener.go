package socket

import (
	"fmt"
	"net"
	"os"
)

// Listener accepts module connections on a Unix domain socket (AF_UNIX).
// On startup it removes any stale socket file left by a previous crash.
type Listener struct {
	ln   net.Listener
	addr string
}

// Listen creates a Unix domain socket listener at the given path.
//
// If a socket file already exists at addr, it is removed first. This is the
// standard Unix pattern to avoid "address already in use" after an unclean
// shutdown. On Windows, AF_UNIX is supported since Windows 10 version 1803.
func Listen(addr string) (*Listener, error) {
	// Remove stale socket file. Ignore errors — the file may not exist,
	// or the path may be invalid (net.Listen will catch that).
	_ = os.Remove(addr)

	ln, err := net.Listen("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("socket: listen %s: %w", addr, err)
	}

	return &Listener{
		ln:   ln,
		addr: addr,
	}, nil
}

// Accept waits for and returns the next module connection.
// The returned [Conn] is ready for handshake (WriteHandshakeWelcome /
// ReadHandshakeHello).
func (l *Listener) Accept() (*Conn, error) {
	c, err := l.ln.Accept()
	if err != nil {
		return nil, fmt.Errorf("socket: accept: %w", err)
	}
	return NewConn(c), nil
}

// Close stops accepting connections and removes the socket file.
func (l *Listener) Close() error {
	err := l.ln.Close()

	// Best-effort cleanup of socket file.
	_ = os.Remove(l.addr)

	if err != nil {
		return fmt.Errorf("socket: close listener: %w", err)
	}
	return nil
}

// Addr returns the listener's network address.
func (l *Listener) Addr() net.Addr {
	return l.ln.Addr()
}
