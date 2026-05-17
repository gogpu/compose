// Package socket provides a Unix domain socket transport for the compose
// protocol. It implements framed read/write of [protocol.Header] + payload
// messages over a single [net.Conn], along with server-side listening and
// client-side dialing.
//
// # Conn
//
// [Conn] wraps a [net.Conn] with framed I/O. Each frame on the wire is a
// 64-byte header (see [protocol.HeaderSize]) followed by PayloadSize bytes
// of payload. Reads and writes are independently locked, so concurrent
// producers and consumers are safe on the same connection.
//
// # Listener
//
// [Listener] binds a Unix domain socket (AF_UNIX) and accepts incoming
// connections. On startup it removes any stale socket file left by a
// previous crash, preventing "address already in use" errors.
//
// # Dialer
//
// [Dialer] connects to a compositor's Unix domain socket. It is intentionally
// simple — reconnection with backoff belongs in higher layers
// (internal/conn.Manager), keeping the transport layer focused on
// establishing a single connection.
//
// # Platform support
//
// Unix domain sockets are supported on Linux, macOS, and Windows 10 1803+
// (AF_UNIX). No CGO required on any platform.
package socket
