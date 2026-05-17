package conn

import "errors"

// Sentinel errors for the conn package.
var (
	// ErrMaxModules is returned when attempting to register a module beyond
	// the configured maximum capacity.
	ErrMaxModules = errors.New("compose: maximum module count reached")

	// ErrNameTaken is returned when attempting to register a module with a
	// name that is already in use by an active module.
	ErrNameTaken = errors.New("compose: module name already registered")

	// ErrNotFound is returned when a lookup or operation references a module
	// ID that does not exist in the registry.
	ErrNotFound = errors.New("compose: module not found")
)
