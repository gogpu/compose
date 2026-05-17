package flow

import (
	"sync"
	"testing"
	"time"
)

// testClock returns a clock function and an advance function.
// The clock starts at a fixed epoch. Advance moves the clock forward
// by the given duration. All time progression is deterministic.
func testClock() (now func() time.Time, advance func(d time.Duration)) {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return func() time.Time {
			return t
		}, func(d time.Duration) {
			t = t.Add(d)
		}
}

func TestNewController(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.ModuleCount() != 0 {
		t.Errorf("ModuleCount() = %d, want 0", c.ModuleCount())
	}
	if c.PendingModules() != 0 {
		t.Errorf("PendingModules() = %d, want 0", c.PendingModules())
	}
}

func TestNewControllerWithClock(t *testing.T) {
	clock, _ := testClock()
	c := New(WithClock(clock))
	if c == nil {
		t.Fatal("New(WithClock) returned nil")
	}
	if c.now == nil {
		t.Fatal("clock function not set")
	}
}

func TestAddModule(t *testing.T) {
	c := New()

	c.AddModule(1, 60)
	if c.ModuleCount() != 1 {
		t.Errorf("ModuleCount() = %d, want 1", c.ModuleCount())
	}

	c.AddModule(2, 30)
	if c.ModuleCount() != 2 {
		t.Errorf("ModuleCount() = %d, want 2", c.ModuleCount())
	}
}

func TestAddModuleZeroFPS(t *testing.T) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 0)

	c.mu.Lock()
	ms := c.modules[1]
	c.mu.Unlock()

	if ms.targetFPS != 1 {
		t.Errorf("targetFPS = %d, want 1 (default for 0 FPS)", ms.targetFPS)
	}
	if ms.interval != time.Second {
		t.Errorf("interval = %v, want %v", ms.interval, time.Second)
	}
}

func TestAddModuleOverwrite(t *testing.T) {
	c := New()

	c.AddModule(1, 60)
	c.AddModule(1, 30)

	if c.ModuleCount() != 1 {
		t.Errorf("ModuleCount() = %d, want 1 (overwrite, not duplicate)", c.ModuleCount())
	}

	c.mu.Lock()
	ms := c.modules[1]
	c.mu.Unlock()

	if ms.targetFPS != 30 {
		t.Errorf("targetFPS = %d, want 30 (overwritten)", ms.targetFPS)
	}
}

func TestRemoveModule(t *testing.T) {
	c := New()

	c.AddModule(1, 60)
	c.AddModule(2, 30)

	c.RemoveModule(1)
	if c.ModuleCount() != 1 {
		t.Errorf("ModuleCount() = %d, want 1 after remove", c.ModuleCount())
	}

	c.RemoveModule(2)
	if c.ModuleCount() != 0 {
		t.Errorf("ModuleCount() = %d, want 0 after remove all", c.ModuleCount())
	}
}

func TestRemoveUnknownModule(t *testing.T) {
	c := New()

	// Should not panic.
	c.RemoveModule(999)

	if c.ModuleCount() != 0 {
		t.Errorf("ModuleCount() = %d, want 0", c.ModuleCount())
	}
}

func TestShouldRequestFirstTime(t *testing.T) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 60)

	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false for first request, want true")
	}
}

func TestShouldRequestUnknownModule(t *testing.T) {
	c := New()

	if c.ShouldRequest(999) {
		t.Error("ShouldRequest(999) = true for unknown module, want false")
	}
}

func TestShouldRequestRespectsInterval(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 10) // 10 FPS = 100ms interval

	// First request: allowed.
	if !c.ShouldRequest(1) {
		t.Fatal("ShouldRequest() = false for first request")
	}

	c.FrameRequested(1)
	c.FrameDelivered(1)

	// Immediately after delivery: not enough time has passed.
	if c.ShouldRequest(1) {
		t.Error("ShouldRequest() = true immediately after request, want false")
	}

	// Advance 50ms (half the interval): still too early.
	advance(50 * time.Millisecond)
	if c.ShouldRequest(1) {
		t.Error("ShouldRequest() = true at 50ms (half interval), want false")
	}

	// Advance another 50ms (total 100ms = interval): should be allowed.
	advance(50 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false at 100ms (full interval), want true")
	}
}

func TestShouldRequestBlocksWhilePending(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 1) // 1 FPS = 1s interval

	// First request.
	if !c.ShouldRequest(1) {
		t.Fatal("ShouldRequest() = false for first request")
	}
	c.FrameRequested(1)

	// Even after the interval, pending blocks the request.
	advance(2 * time.Second)
	if c.ShouldRequest(1) {
		t.Error("ShouldRequest() = true while pending, want false")
	}

	// After delivery, pending is cleared.
	c.FrameDelivered(1)

	// Now enough time has passed.
	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false after delivery + elapsed interval, want true")
	}
}

func TestFrameRequestedSetsTime(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 10) // 100ms interval

	advance(500 * time.Millisecond)
	c.FrameRequested(1)

	c.mu.Lock()
	ms := c.modules[1]
	lr := ms.lastRequest
	c.mu.Unlock()

	expected := time.Date(2026, 1, 1, 0, 0, 0, 500_000_000, time.UTC)
	if !lr.Equal(expected) {
		t.Errorf("lastRequest = %v, want %v", lr, expected)
	}
}

func TestFrameRequestedUnknownModule(t *testing.T) {
	c := New()

	// Should not panic.
	c.FrameRequested(999)
}

func TestFrameDeliveredClearsPending(t *testing.T) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 60)

	c.FrameRequested(1)
	if c.PendingModules() != 1 {
		t.Errorf("PendingModules() = %d after request, want 1", c.PendingModules())
	}

	c.FrameDelivered(1)
	if c.PendingModules() != 0 {
		t.Errorf("PendingModules() = %d after delivery, want 0", c.PendingModules())
	}
}

func TestFrameDeliveredClearsMissedCount(t *testing.T) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 60)

	// Accumulate misses.
	c.FrameMissed(1)
	c.FrameMissed(1)

	c.mu.Lock()
	countBefore := c.modules[1].missedCount
	c.mu.Unlock()

	if countBefore != 2 {
		t.Errorf("missedCount = %d before delivery, want 2", countBefore)
	}

	// Delivery resets missed count.
	c.FrameDelivered(1)

	c.mu.Lock()
	countAfter := c.modules[1].missedCount
	c.mu.Unlock()

	if countAfter != 0 {
		t.Errorf("missedCount = %d after delivery, want 0", countAfter)
	}
}

func TestFrameDeliveredUnknownModule(t *testing.T) {
	c := New()

	// Should not panic.
	c.FrameDelivered(999)
}

func TestFrameMissedIncrements(t *testing.T) {
	c := New()

	c.AddModule(1, 60)

	for i := 1; i <= 5; i++ {
		c.FrameMissed(1)

		c.mu.Lock()
		count := c.modules[1].missedCount
		c.mu.Unlock()

		if count != i {
			t.Errorf("missedCount after %d misses = %d, want %d", i, count, i)
		}
	}
}

func TestFrameMissedClearsPending(t *testing.T) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 60)

	c.FrameRequested(1)
	if c.PendingModules() != 1 {
		t.Fatal("PendingModules() != 1 after request")
	}

	c.FrameMissed(1)
	if c.PendingModules() != 0 {
		t.Errorf("PendingModules() = %d after miss, want 0", c.PendingModules())
	}
}

func TestFrameMissedUnknownModule(t *testing.T) {
	c := New()

	// Should not panic.
	c.FrameMissed(999)
}

func TestAdaptiveRateReduction(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 10) // 10 FPS = 100ms base interval

	// Request and deliver first frame to establish lastRequest.
	c.FrameRequested(1)
	c.FrameDelivered(1)

	// Accumulate 3 misses (threshold).
	c.FrameRequested(1)
	c.FrameMissed(1)
	c.FrameRequested(1)
	c.FrameMissed(1)
	c.FrameRequested(1)
	c.FrameMissed(1)

	// After 3 misses, effective interval should be 200ms (doubled).
	// Advance 100ms from last request: should NOT be ready.
	advance(100 * time.Millisecond)
	if c.ShouldRequest(1) {
		t.Error("ShouldRequest() = true at 100ms (base interval) after 3 misses, want false (doubled to 200ms)")
	}

	// Advance another 100ms (total 200ms): now should be ready.
	advance(100 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false at 200ms (doubled interval) after 3 misses, want true")
	}
}

func TestAdaptiveRateResetOnDelivery(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 10) // 10 FPS = 100ms base interval

	c.FrameRequested(1)
	c.FrameDelivered(1)

	// Accumulate 3 misses.
	c.FrameRequested(1)
	c.FrameMissed(1)
	c.FrameRequested(1)
	c.FrameMissed(1)
	c.FrameRequested(1)
	c.FrameMissed(1)

	// Now deliver a successful frame. This resets missedCount.
	c.FrameRequested(1)
	c.FrameDelivered(1)

	// After delivery, effective interval should be back to 100ms.
	advance(100 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false at 100ms after miss reset, want true (back to base interval)")
	}
}

func TestAdaptiveRateBelowThreshold(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 10) // 10 FPS = 100ms interval

	c.FrameRequested(1)
	c.FrameDelivered(1)

	// Only 2 misses (below threshold of 3).
	c.FrameRequested(1)
	c.FrameMissed(1)
	c.FrameRequested(1)
	c.FrameMissed(1)

	// Effective interval should still be 100ms (not doubled).
	advance(100 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false at 100ms with only 2 misses, want true (below threshold)")
	}
}

func TestPendingModules(t *testing.T) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 60)
	c.AddModule(2, 30)
	c.AddModule(3, 10)

	if c.PendingModules() != 0 {
		t.Errorf("PendingModules() = %d initially, want 0", c.PendingModules())
	}

	c.FrameRequested(1)
	c.FrameRequested(2)
	if c.PendingModules() != 2 {
		t.Errorf("PendingModules() = %d after 2 requests, want 2", c.PendingModules())
	}

	c.FrameDelivered(1)
	if c.PendingModules() != 1 {
		t.Errorf("PendingModules() = %d after 1 delivery, want 1", c.PendingModules())
	}

	c.FrameMissed(2)
	if c.PendingModules() != 0 {
		t.Errorf("PendingModules() = %d after miss, want 0", c.PendingModules())
	}
}

func TestModuleCount(t *testing.T) {
	c := New()

	tests := []struct {
		action string
		fn     func()
		want   int
	}{
		{"initial", func() {}, 0},
		{"add 1", func() { c.AddModule(1, 60) }, 1},
		{"add 2", func() { c.AddModule(2, 30) }, 2},
		{"add 3", func() { c.AddModule(3, 10) }, 3},
		{"remove 2", func() { c.RemoveModule(2) }, 2},
		{"remove unknown", func() { c.RemoveModule(999) }, 2},
		{"remove 1", func() { c.RemoveModule(1) }, 1},
		{"remove 3", func() { c.RemoveModule(3) }, 0},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			tt.fn()
			if got := c.ModuleCount(); got != tt.want {
				t.Errorf("ModuleCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHighFPSInterval(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 120) // 120 FPS ~ 8.33ms interval

	c.FrameRequested(1)
	c.FrameDelivered(1)

	// At 8ms: should not be ready yet.
	advance(8 * time.Millisecond)
	if c.ShouldRequest(1) {
		t.Error("ShouldRequest() = true at 8ms for 120 FPS, want false")
	}

	// At 9ms: should be ready (8.33ms interval).
	advance(1 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false at 9ms for 120 FPS, want true")
	}
}

func TestMultipleModulesIndependent(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 10) // 100ms interval
	c.AddModule(2, 5)  // 200ms interval

	// Both should allow first request.
	if !c.ShouldRequest(1) {
		t.Error("module 1: ShouldRequest() = false for first request")
	}
	if !c.ShouldRequest(2) {
		t.Error("module 2: ShouldRequest() = false for first request")
	}

	c.FrameRequested(1)
	c.FrameRequested(2)
	c.FrameDelivered(1)
	c.FrameDelivered(2)

	// At 100ms: module 1 ready, module 2 not.
	advance(100 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("module 1: ShouldRequest() = false at 100ms, want true")
	}
	if c.ShouldRequest(2) {
		t.Error("module 2: ShouldRequest() = true at 100ms (needs 200ms), want false")
	}

	// At 200ms: both ready.
	advance(100 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("module 1: ShouldRequest() = false at 200ms, want true")
	}
	if !c.ShouldRequest(2) {
		t.Error("module 2: ShouldRequest() = false at 200ms, want true")
	}
}

func TestEffectiveInterval(t *testing.T) {
	tests := []struct {
		name        string
		missedCount int
		baseMs      int
		wantMs      int
	}{
		{"no misses", 0, 100, 100},
		{"1 miss", 1, 100, 100},
		{"2 misses", 2, 100, 100},
		{"3 misses (threshold)", 3, 100, 200},
		{"4 misses", 4, 100, 200},
		{"10 misses", 10, 100, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &moduleState{
				interval:    time.Duration(tt.baseMs) * time.Millisecond,
				missedCount: tt.missedCount,
			}
			got := ms.effectiveInterval()
			want := time.Duration(tt.wantMs) * time.Millisecond
			if got != want {
				t.Errorf("effectiveInterval() = %v, want %v", got, want)
			}
		})
	}
}

func TestFullLifecycle(t *testing.T) {
	clock, advance := testClock()
	c := New(WithClock(clock))

	// Register.
	c.AddModule(1, 10) // 100ms interval
	if c.ModuleCount() != 1 {
		t.Fatal("ModuleCount() != 1 after add")
	}

	// First request cycle.
	if !c.ShouldRequest(1) {
		t.Fatal("ShouldRequest() = false for first request")
	}
	c.FrameRequested(1)
	if c.PendingModules() != 1 {
		t.Fatal("PendingModules() != 1 after request")
	}
	c.FrameDelivered(1)
	if c.PendingModules() != 0 {
		t.Fatal("PendingModules() != 0 after delivery")
	}

	// Second request after interval.
	advance(100 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Fatal("ShouldRequest() = false after interval")
	}
	c.FrameRequested(1)
	c.FrameDelivered(1)

	// Miss some frames.
	advance(100 * time.Millisecond)
	c.FrameRequested(1)
	c.FrameMissed(1)
	advance(100 * time.Millisecond)
	c.FrameRequested(1)
	c.FrameMissed(1)
	advance(100 * time.Millisecond)
	c.FrameRequested(1)
	c.FrameMissed(1)

	// Now at doubled interval. 100ms should not suffice.
	advance(100 * time.Millisecond)
	if c.ShouldRequest(1) {
		t.Error("ShouldRequest() = true at base interval after 3 misses, want false")
	}

	// 200ms should work.
	advance(100 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false at doubled interval, want true")
	}

	// Deliver to reset.
	c.FrameRequested(1)
	c.FrameDelivered(1)

	// Back to normal interval.
	advance(100 * time.Millisecond)
	if !c.ShouldRequest(1) {
		t.Error("ShouldRequest() = false after reset, want true")
	}

	// Unregister.
	c.RemoveModule(1)
	if c.ModuleCount() != 0 {
		t.Fatal("ModuleCount() != 0 after remove")
	}
	if c.ShouldRequest(1) {
		t.Error("ShouldRequest() = true for removed module, want false")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New()

	const numModules = 100
	const iterations = 1000

	// Add modules.
	for i := uint64(0); i < numModules; i++ {
		c.AddModule(i, uint16(10+i%50))
	}

	var wg sync.WaitGroup

	// Concurrent ShouldRequest calls.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			for i := uint64(0); i < numModules; i++ {
				c.ShouldRequest(i)
			}
		}
	}()

	// Concurrent FrameRequested/Delivered calls.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			for i := uint64(0); i < numModules; i++ {
				c.FrameRequested(i)
				c.FrameDelivered(i)
			}
		}
	}()

	// Concurrent FrameMissed calls.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			for i := uint64(0); i < numModules; i++ {
				c.FrameMissed(i)
			}
		}
	}()

	// Concurrent PendingModules/ModuleCount calls.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			c.PendingModules()
			c.ModuleCount()
		}
	}()

	// Concurrent Add/Remove.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			c.AddModule(numModules+1, 60)
			c.RemoveModule(numModules + 1)
		}
	}()

	wg.Wait()
}

func BenchmarkShouldRequest(b *testing.B) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 60)

	b.ResetTimer()
	for range b.N {
		c.ShouldRequest(1)
	}
}

func BenchmarkShouldRequestMultipleModules(b *testing.B) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	const numModules = 16
	for i := uint64(1); i <= numModules; i++ {
		c.AddModule(i, uint16(10*i))
	}

	b.ResetTimer()
	for range b.N {
		for i := uint64(1); i <= numModules; i++ {
			c.ShouldRequest(i)
		}
	}
}

func BenchmarkFrameRequestDeliverCycle(b *testing.B) {
	clock, _ := testClock()
	c := New(WithClock(clock))

	c.AddModule(1, 60)

	b.ResetTimer()
	for range b.N {
		c.FrameRequested(1)
		c.FrameDelivered(1)
	}
}
