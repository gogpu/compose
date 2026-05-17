package conn

import "time"

// State represents the lifecycle state of a module connection.
type State uint8

const (
	// StateConnecting indicates a connection has been initiated but the
	// handshake has not yet started.
	StateConnecting State = iota

	// StateHandshaking indicates the handshake is in progress.
	StateHandshaking

	// StateActive indicates the module is fully connected and sending frames.
	StateActive

	// StateDisconnected indicates the connection was lost or gracefully closed.
	StateDisconnected
)

// String returns a human-readable name for the state.
func (s State) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateHandshaking:
		return "handshaking"
	case StateActive:
		return "active"
	case StateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

// Module holds metadata about a connected module.
type Module struct {
	// ID is the compositor-assigned unique identifier for this module.
	// IDs are monotonically increasing and never reused within a process lifetime.
	ID uint64

	// Name is the human-readable module name (e.g., "clock", "weather").
	Name string

	// State is the current lifecycle state of the module connection.
	State State

	// Width is the frame width in pixels.
	Width uint16

	// Height is the frame height in pixels.
	Height uint16

	// FPS is the requested frame rate.
	FPS uint16

	// ConnectedAt is the time the module first connected.
	ConnectedAt time.Time

	// LastFrameAt is the time the most recent frame was received.
	LastFrameAt time.Time
}
