package flow

import (
	"sync"
	"time"
)

// defaultFPS is used when a module specifies 0 FPS (static content).
const defaultFPS = 1

// missedThreshold is the number of consecutive missed frames before the
// controller halves the effective request rate for a module.
const missedThreshold = 3

// moduleState holds per-module frame pacing state.
type moduleState struct {
	targetFPS    uint16        // module's preferred FPS (from handshake)
	interval     time.Duration // base interval: 1/targetFPS
	lastRequest  time.Time     // when we last sent FrameRequest
	lastDelivery time.Time     // when we last received a frame
	pending      bool          // true if FrameRequest sent but no frame received yet
	missedCount  int           // consecutive missed requests (for adaptive rate)
}

// effectiveInterval returns the current interval accounting for adaptive rate
// reduction. After missedThreshold consecutive misses, the interval doubles.
func (ms *moduleState) effectiveInterval() time.Duration {
	if ms.missedCount >= missedThreshold {
		return ms.interval * 2
	}
	return ms.interval
}

// Option configures a Controller.
type Option func(*Controller)

// WithClock sets a custom time source for the controller. Intended for testing
// to provide deterministic time progression without time.Sleep.
func WithClock(fn func() time.Time) Option {
	return func(c *Controller) {
		c.now = fn
	}
}

// Controller manages pull-based frame pacing for connected modules.
// The compositor uses it to decide when to request frames from each module.
// All methods are safe for concurrent access from multiple goroutines.
type Controller struct {
	mu      sync.Mutex
	modules map[uint64]*moduleState
	now     func() time.Time
}

// New creates a flow controller. Use WithClock to inject a custom time source
// for testing.
func New(opts ...Option) *Controller {
	c := &Controller{
		modules: make(map[uint64]*moduleState),
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// AddModule registers a module with its preferred FPS.
// If fps is 0, it defaults to 1 FPS (suitable for static content like a clock).
func (c *Controller) AddModule(moduleID uint64, fps uint16) {
	if fps == 0 {
		fps = defaultFPS
	}

	interval := time.Second / time.Duration(fps)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.modules[moduleID] = &moduleState{
		targetFPS: fps,
		interval:  interval,
	}
}

// RemoveModule unregisters a module. If the module ID does not exist, this is
// a no-op.
func (c *Controller) RemoveModule(moduleID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.modules, moduleID)
}

// ShouldRequest returns true if it is time to request a frame from the
// specified module. The compositor should call this on each render tick.
//
// Returns false if:
//   - The module ID is not registered
//   - A request is already pending (module has not responded yet)
//   - Not enough time has elapsed since the last request (respecting target FPS
//     and adaptive rate reduction)
func (c *Controller) ShouldRequest(moduleID uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	ms, ok := c.modules[moduleID]
	if !ok {
		return false
	}

	if ms.pending {
		return false
	}

	now := c.now()

	// First request: always allow if no request has been made yet.
	if ms.lastRequest.IsZero() {
		return true
	}

	elapsed := now.Sub(ms.lastRequest)
	return elapsed >= ms.effectiveInterval()
}

// FrameRequested marks that a FrameRequest was sent to the specified module.
// Records the request time and sets the pending flag. If the module ID does not
// exist, this is a no-op.
func (c *Controller) FrameRequested(moduleID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ms, ok := c.modules[moduleID]
	if !ok {
		return
	}

	ms.lastRequest = c.now()
	ms.pending = true
}

// FrameDelivered marks that a frame was received from the specified module.
// Resets the pending flag, records the delivery time, and clears the missed
// count. If the module ID does not exist, this is a no-op.
func (c *Controller) FrameDelivered(moduleID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ms, ok := c.modules[moduleID]
	if !ok {
		return
	}

	ms.lastDelivery = c.now()
	ms.pending = false
	ms.missedCount = 0
}

// FrameMissed marks that the specified module failed to deliver a frame in
// time. After missedThreshold (3) consecutive misses, the controller halves
// the effective request rate by doubling the interval. This prevents wasting
// bandwidth on slow or stuck modules. If the module ID does not exist, this
// is a no-op.
func (c *Controller) FrameMissed(moduleID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ms, ok := c.modules[moduleID]
	if !ok {
		return
	}

	ms.missedCount++
	ms.pending = false
}

// PendingModules returns the count of modules with pending (unanswered)
// frame requests.
func (c *Controller) PendingModules() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for _, ms := range c.modules {
		if ms.pending {
			count++
		}
	}
	return count
}

// ModuleCount returns the total number of registered modules.
func (c *Controller) ModuleCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.modules)
}
