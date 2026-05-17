package compose

import (
	"errors"

	"github.com/gogpu/compose/internal/conn"
)

// Sentinel errors returned by Server and Client.
var (
	// ErrClosed is returned when an operation is attempted on a closed
	// Server or Client.
	ErrClosed = errors.New("compose: server/client closed")

	// ErrNotAccepted is returned by Dial when the compositor rejects the
	// connection (e.g., due to capacity limits or policy).
	ErrNotAccepted = errors.New("compose: connection not accepted by compositor")

	// ErrModuleNotFound is returned by RequestFrame when the specified
	// module ID does not exist in the server's module table.
	ErrModuleNotFound = errors.New("compose: module not found")

	// ErrMaxModules is returned when the server cannot accept a new module
	// because the maximum module count has been reached.
	ErrMaxModules = conn.ErrMaxModules

	// ErrNameTaken is returned when a module with the same name is already
	// connected to the server.
	ErrNameTaken = conn.ErrNameTaken
)
