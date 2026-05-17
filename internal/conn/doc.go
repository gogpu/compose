// Package conn provides connection lifecycle management for the compose library.
//
// It handles module ID allocation, name-to-ID mapping, hot-plug detection,
// graceful disconnect, and reconnection matching. The package is standalone
// with no internal dependencies.
//
// The core types are:
//
//   - [Registry] manages module ID allocation and lookup.
//   - [Manager] orchestrates the full module lifecycle (connect, handshake,
//     active, disconnect) and fires event callbacks.
//   - [Module] holds metadata about a connected module.
//   - [State] represents the lifecycle state of a module connection.
//
// All exported methods are safe for concurrent use from multiple goroutines.
package conn
