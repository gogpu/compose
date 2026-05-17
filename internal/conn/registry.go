package conn

import (
	"sync"
	"time"
)

// Registry manages module ID allocation and name-to-ID mapping.
// All methods are safe for concurrent access from multiple goroutines.
type Registry struct {
	mu         sync.RWMutex
	modules    map[uint64]*Module
	nameIndex  map[string]uint64
	nextID     uint64
	maxModules int
}

// NewRegistry creates a registry with the given maximum capacity.
// The maxModules parameter must be positive; values less than 1 are
// clamped to 1.
func NewRegistry(maxModules int) *Registry {
	if maxModules < 1 {
		maxModules = 1
	}
	return &Registry{
		modules:    make(map[uint64]*Module),
		nameIndex:  make(map[string]uint64),
		nextID:     1, // IDs start at 1, never 0
		maxModules: maxModules,
	}
}

// Register allocates a new module ID and stores the module.
// Returns ErrMaxModules if at capacity.
// Returns ErrNameTaken if a module with the same name is already active.
func (r *Registry) Register(name string, width, height, fps uint16) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.modules) >= r.maxModules {
		return 0, ErrMaxModules
	}

	if _, exists := r.nameIndex[name]; exists {
		return 0, ErrNameTaken
	}

	id := r.nextID
	r.nextID++

	mod := &Module{
		ID:          id,
		Name:        name,
		State:       StateConnecting,
		Width:       width,
		Height:      height,
		FPS:         fps,
		ConnectedAt: time.Now(),
	}

	r.modules[id] = mod
	r.nameIndex[name] = id

	return id, nil
}

// Unregister removes a module from the registry, freeing its slot.
// The module's name becomes available for reuse. If the module ID does
// not exist, this is a no-op.
func (r *Registry) Unregister(id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mod, exists := r.modules[id]
	if !exists {
		return
	}

	delete(r.nameIndex, mod.Name)
	delete(r.modules, id)
}

// Lookup returns the module by ID. Returns (nil, false) if not found.
func (r *Registry) Lookup(id uint64) (*Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mod, exists := r.modules[id]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent data races on the caller's side.
	cp := *mod
	return &cp, true
}

// LookupByName returns the module by name. Returns (nil, false) if not found.
func (r *Registry) LookupByName(name string) (*Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, exists := r.nameIndex[name]
	if !exists {
		return nil, false
	}

	mod, exists := r.modules[id]
	if !exists {
		return nil, false
	}

	cp := *mod
	return &cp, true
}

// SetState updates a module's state. If the module ID does not exist,
// this is a no-op.
func (r *Registry) SetState(id uint64, state State) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if mod, exists := r.modules[id]; exists {
		mod.State = state
	}
}

// UpdateLastFrame updates the last frame timestamp for a module.
// If the module ID does not exist, this is a no-op.
func (r *Registry) UpdateLastFrame(id uint64, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if mod, exists := r.modules[id]; exists {
		mod.LastFrameAt = t
	}
}

// Count returns the number of currently registered modules.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.modules)
}

// All returns a snapshot of all registered modules. The returned slice
// contains copies; modifying them does not affect the registry.
func (r *Registry) All() []*Module {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Module, 0, len(r.modules))
	for _, mod := range r.modules {
		cp := *mod
		result = append(result, &cp)
	}
	return result
}
