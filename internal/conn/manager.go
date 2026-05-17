package conn

import "sync"

// Manager orchestrates module lifecycle: connect, handshake, active, disconnect.
// It wraps a Registry and adds event callbacks plus reconnection matching.
// All methods are safe for concurrent access from multiple goroutines.
type Manager struct {
	registry *Registry

	mu           sync.RWMutex
	onConnect    func(id uint64, name string)
	onDisconnect func(id uint64, name string)
}

// NewManager creates a Manager with the given max module count.
// The maxModules parameter must be positive; values less than 1 are clamped to 1.
func NewManager(maxModules int) *Manager {
	return &Manager{
		registry: NewRegistry(maxModules),
	}
}

// HandleConnect processes a new module connection.
// It allocates an ID, transitions the module to Active state, and fires
// the OnConnect callback if registered.
//
// If a module with the same name was previously disconnected and unregistered,
// it is treated as a reconnection — a new ID is allocated but the name slot
// is reused seamlessly.
//
// Returns ErrMaxModules if the registry is at capacity.
// Returns ErrNameTaken if a module with the same name is currently active.
func (m *Manager) HandleConnect(name string, width, height, fps uint16) (uint64, error) {
	id, err := m.registry.Register(name, width, height, fps)
	if err != nil {
		return 0, err
	}

	// Transition directly to Active (handshake completed at this point).
	m.registry.SetState(id, StateActive)

	// Fire callback outside the registry lock to avoid deadlocks in user code.
	m.mu.RLock()
	cb := m.onConnect
	m.mu.RUnlock()

	if cb != nil {
		cb(id, name)
	}

	return id, nil
}

// HandleDisconnect processes a module disconnection (graceful or crash).
// It transitions the module to Disconnected state, fires the OnDisconnect
// callback, and removes the module from the registry so its name can be reused.
//
// If the module ID does not exist, this is a no-op.
func (m *Manager) HandleDisconnect(id uint64) {
	// Look up the module before unregistering so we have the name for the callback.
	mod, exists := m.registry.Lookup(id)
	if !exists {
		return
	}

	m.registry.SetState(id, StateDisconnected)
	m.registry.Unregister(id)

	// Fire callback after state transition.
	m.mu.RLock()
	cb := m.onDisconnect
	m.mu.RUnlock()

	if cb != nil {
		cb(id, mod.Name)
	}
}

// OnConnect sets the callback fired when a module becomes active.
// Only one callback can be set; subsequent calls replace the previous one.
// Passing nil removes the callback.
func (m *Manager) OnConnect(fn func(id uint64, name string)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.onConnect = fn
}

// OnDisconnect sets the callback fired when a module disconnects.
// Only one callback can be set; subsequent calls replace the previous one.
// Passing nil removes the callback.
func (m *Manager) OnDisconnect(fn func(id uint64, name string)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.onDisconnect = fn
}

// Registry returns the underlying registry for lookups.
func (m *Manager) Registry() *Registry {
	return m.registry
}
