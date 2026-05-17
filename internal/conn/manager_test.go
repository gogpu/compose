package conn

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager(16)
	if m.Registry() == nil {
		t.Fatal("Registry() returned nil")
	}
	if m.Registry().Count() != 0 {
		t.Errorf("initial count = %d, want 0", m.Registry().Count())
	}
}

func TestManager_HandleConnect(t *testing.T) {
	t.Run("basic connect", func(t *testing.T) {
		m := NewManager(16)
		id, err := m.HandleConnect("clock", 400, 120, 1)
		if err != nil {
			t.Fatalf("HandleConnect failed: %v", err)
		}
		if id == 0 {
			t.Error("HandleConnect returned zero ID")
		}

		mod, ok := m.Registry().Lookup(id)
		if !ok {
			t.Fatal("module not found after connect")
		}
		if mod.State != StateActive {
			t.Errorf("state = %v, want StateActive", mod.State)
		}
		if mod.Name != "clock" {
			t.Errorf("name = %q, want %q", mod.Name, "clock")
		}
		if mod.Width != 400 || mod.Height != 120 {
			t.Errorf("dimensions = %dx%d, want 400x120", mod.Width, mod.Height)
		}
		if mod.FPS != 1 {
			t.Errorf("fps = %d, want 1", mod.FPS)
		}
	})

	t.Run("fires OnConnect callback", func(t *testing.T) {
		m := NewManager(16)

		var callbackID uint64
		var callbackName string
		m.OnConnect(func(id uint64, name string) {
			callbackID = id
			callbackName = name
		})

		id, _ := m.HandleConnect("weather", 320, 240, 30)
		if callbackID != id {
			t.Errorf("callback ID = %d, want %d", callbackID, id)
		}
		if callbackName != "weather" {
			t.Errorf("callback name = %q, want %q", callbackName, "weather")
		}
	})

	t.Run("no callback when nil", func(t *testing.T) {
		m := NewManager(16)
		// No callback set — should not panic.
		_, err := m.HandleConnect("mod", 100, 100, 30)
		if err != nil {
			t.Fatalf("HandleConnect failed: %v", err)
		}
	})

	t.Run("max modules error", func(t *testing.T) {
		m := NewManager(2)
		_, _ = m.HandleConnect("a", 100, 100, 30)
		_, _ = m.HandleConnect("b", 100, 100, 30)

		_, err := m.HandleConnect("c", 100, 100, 30)
		if err != ErrMaxModules {
			t.Errorf("err = %v, want ErrMaxModules", err)
		}
	})

	t.Run("name taken error", func(t *testing.T) {
		m := NewManager(16)
		_, _ = m.HandleConnect("clock", 400, 120, 1)

		_, err := m.HandleConnect("clock", 400, 120, 1)
		if err != ErrNameTaken {
			t.Errorf("err = %v, want ErrNameTaken", err)
		}
	})

	t.Run("callback not fired on error", func(t *testing.T) {
		m := NewManager(1)
		_, _ = m.HandleConnect("a", 100, 100, 30)

		callbackFired := false
		m.OnConnect(func(_ uint64, _ string) {
			callbackFired = true
		})

		_, _ = m.HandleConnect("b", 100, 100, 30) // capacity exceeded
		if callbackFired {
			t.Error("OnConnect callback fired on error, should not")
		}
	})
}

func TestManager_HandleDisconnect(t *testing.T) {
	t.Run("basic disconnect", func(t *testing.T) {
		m := NewManager(16)
		id, _ := m.HandleConnect("clock", 400, 120, 1)
		m.HandleDisconnect(id)

		_, ok := m.Registry().Lookup(id)
		if ok {
			t.Error("module still found after disconnect")
		}
		if m.Registry().Count() != 0 {
			t.Errorf("count = %d, want 0", m.Registry().Count())
		}
	})

	t.Run("fires OnDisconnect callback", func(t *testing.T) {
		m := NewManager(16)
		id, _ := m.HandleConnect("weather", 320, 240, 30)

		var callbackID uint64
		var callbackName string
		m.OnDisconnect(func(id uint64, name string) {
			callbackID = id
			callbackName = name
		})

		m.HandleDisconnect(id)
		if callbackID != id {
			t.Errorf("callback ID = %d, want %d", callbackID, id)
		}
		if callbackName != "weather" {
			t.Errorf("callback name = %q, want %q", callbackName, "weather")
		}
	})

	t.Run("nonexistent ID is no-op", func(t *testing.T) {
		m := NewManager(16)
		callbackFired := false
		m.OnDisconnect(func(_ uint64, _ string) {
			callbackFired = true
		})

		m.HandleDisconnect(999) // should not panic
		if callbackFired {
			t.Error("OnDisconnect fired for nonexistent ID")
		}
	})

	t.Run("double disconnect is safe", func(t *testing.T) {
		m := NewManager(16)
		id, _ := m.HandleConnect("mod", 100, 100, 30)

		var count int
		m.OnDisconnect(func(_ uint64, _ string) {
			count++
		})

		m.HandleDisconnect(id)
		m.HandleDisconnect(id) // second call should be no-op

		if count != 1 {
			t.Errorf("OnDisconnect fired %d times, want 1", count)
		}
	})
}

func TestManager_Reconnection(t *testing.T) {
	t.Run("name reusable after disconnect", func(t *testing.T) {
		m := NewManager(16)
		id1, _ := m.HandleConnect("clock", 400, 120, 1)
		m.HandleDisconnect(id1)

		id2, err := m.HandleConnect("clock", 400, 120, 1)
		if err != nil {
			t.Fatalf("reconnect failed: %v", err)
		}
		if id2 <= id1 {
			t.Errorf("reconnect ID not monotonic: old=%d, new=%d", id1, id2)
		}
	})

	t.Run("reconnection fires callbacks", func(t *testing.T) {
		m := NewManager(16)

		var connectCount, disconnectCount int
		m.OnConnect(func(_ uint64, _ string) { connectCount++ })
		m.OnDisconnect(func(_ uint64, _ string) { disconnectCount++ })

		id, _ := m.HandleConnect("clock", 400, 120, 1)
		m.HandleDisconnect(id)
		_, _ = m.HandleConnect("clock", 400, 120, 1)

		if connectCount != 2 {
			t.Errorf("connect count = %d, want 2", connectCount)
		}
		if disconnectCount != 1 {
			t.Errorf("disconnect count = %d, want 1", disconnectCount)
		}
	})

	t.Run("slot freed for capacity", func(t *testing.T) {
		m := NewManager(2)
		id1, _ := m.HandleConnect("a", 100, 100, 30)
		_, _ = m.HandleConnect("b", 100, 100, 30)

		// At capacity.
		_, err := m.HandleConnect("c", 100, 100, 30)
		if err != ErrMaxModules {
			t.Fatalf("expected ErrMaxModules, got %v", err)
		}

		// Disconnect one — slot freed.
		m.HandleDisconnect(id1)
		_, err = m.HandleConnect("c", 100, 100, 30)
		if err != nil {
			t.Errorf("connect after disconnect failed: %v", err)
		}
	})
}

func TestManager_CallbackReplacement(t *testing.T) {
	m := NewManager(16)

	var firstCalled, secondCalled bool

	m.OnConnect(func(_ uint64, _ string) { firstCalled = true })
	m.OnConnect(func(_ uint64, _ string) { secondCalled = true })

	m.HandleConnect("mod", 100, 100, 30)

	if firstCalled {
		t.Error("first callback called after replacement")
	}
	if !secondCalled {
		t.Error("second callback not called")
	}
}

func TestManager_NilCallback(t *testing.T) {
	m := NewManager(16)

	m.OnConnect(func(_ uint64, _ string) {})
	m.OnConnect(nil) // remove callback

	// Should not panic.
	_, _ = m.HandleConnect("mod", 100, 100, 30)
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager(500)

	var connectCount atomic.Int64
	var disconnectCount atomic.Int64

	m.OnConnect(func(_ uint64, _ string) {
		connectCount.Add(1)
	})
	m.OnDisconnect(func(_ uint64, _ string) {
		disconnectCount.Add(1)
	})

	var wg sync.WaitGroup

	// Concurrent connects.
	ids := make([]uint64, 200)
	var idMu sync.Mutex
	for i := range 200 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("module-%d", idx)
			id, err := m.HandleConnect(name, 100, 100, 30)
			if err == nil {
				idMu.Lock()
				ids[idx] = id
				idMu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	// Concurrent disconnects.
	for i := range 200 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			idMu.Lock()
			id := ids[idx]
			idMu.Unlock()
			if id != 0 {
				m.HandleDisconnect(id)
			}
		}(i)
	}
	wg.Wait()

	// After all disconnects, registry should be empty.
	if m.Registry().Count() != 0 {
		t.Errorf("count after all disconnects = %d, want 0", m.Registry().Count())
	}

	// Verify callback counts match.
	connected := connectCount.Load()
	disconnected := disconnectCount.Load()
	if connected != disconnected {
		t.Errorf("connect count (%d) != disconnect count (%d)", connected, disconnected)
	}
}

func TestManager_Registry(t *testing.T) {
	m := NewManager(16)
	r := m.Registry()
	if r == nil {
		t.Fatal("Registry() returned nil")
	}

	// Verify it is the same instance.
	id, _ := m.HandleConnect("mod", 100, 100, 30)
	mod, ok := r.Lookup(id)
	if !ok {
		t.Fatal("registry lookup failed for module connected via manager")
	}
	if mod.Name != "mod" {
		t.Errorf("name = %q, want %q", mod.Name, "mod")
	}
}

func BenchmarkHandleConnect(b *testing.B) {
	m := NewManager(b.N + 1)
	b.ResetTimer()
	for i := range b.N {
		name := fmt.Sprintf("module-%d", i)
		_, _ = m.HandleConnect(name, 100, 100, 30)
	}
}

func BenchmarkHandleDisconnect(b *testing.B) {
	m := NewManager(b.N + 1)
	ids := make([]uint64, b.N)
	for i := range b.N {
		ids[i], _ = m.HandleConnect(fmt.Sprintf("module-%d", i), 100, 100, 30)
	}

	b.ResetTimer()
	for i := range b.N {
		m.HandleDisconnect(ids[i])
	}
}
