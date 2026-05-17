package socket

import (
	"fmt"
	"net"
	"time"
)

// defaultDialTimeout is the default connection timeout.
const defaultDialTimeout = 5 * time.Second

// Dialer connects to a compositor's Unix domain socket.
// Reconnection with backoff belongs in higher layers (internal/conn.Manager);
// the dialer is a simple one-shot connect helper.
type Dialer struct {
	addr    string
	timeout time.Duration
}

// NewDialer creates a dialer for the given Unix domain socket address.
// The default timeout is 5 seconds.
func NewDialer(addr string) *Dialer {
	return &Dialer{
		addr:    addr,
		timeout: defaultDialTimeout,
	}
}

// Dial connects to the compositor using the default timeout.
// The returned [Conn] is ready for handshake (WriteHandshakeHello /
// ReadHandshakeWelcome).
func (d *Dialer) Dial() (*Conn, error) {
	return d.DialWithTimeout(d.timeout)
}

// DialWithTimeout connects to the compositor with a custom timeout.
// A zero or negative timeout means no deadline.
func (d *Dialer) DialWithTimeout(timeout time.Duration) (*Conn, error) {
	c, err := net.DialTimeout("unix", d.addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("socket: dial %s: %w", d.addr, err)
	}
	return NewConn(c), nil
}
