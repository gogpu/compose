package conn

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	t.Run("positive capacity", func(t *testing.T) {
		r := NewRegistry(16)
		if r.Count() != 0 {
			t.Errorf("new registry count = %d, want 0", r.Count())
		}
	})

	t.Run("zero capacity clamped to 1", func(t *testing.T) {
		r := NewRegistry(0)
		_, err := r.Register("a", 100, 100, 30)
		if err != nil {
			t.Fatalf("first register failed: %v", err)
		}
		_, err = r.Register("b", 100, 100, 30)
		if !errors.Is(err, ErrMaxModules) {
			t.Errorf("second register err = %v, want ErrMaxModules", err)
		}
	})

	t.Run("negative capacity clamped to 1", func(t *testing.T) {
		r := NewRegistry(-5)
		_, err := r.Register("a", 100, 100, 30)
		if err != nil {
			t.Fatalf("first register failed: %v", err)
		}
		_, err = r.Register("b", 100, 100, 30)
		if !errors.Is(err, ErrMaxModules) {
			t.Errorf("second register err = %v, want ErrMaxModules", err)
		}
	})
}

func TestRegistry_Register(t *testing.T) {
	t.Run("basic registration", func(t *testing.T) {
		r := NewRegistry(16)
		id, err := r.Register("clock", 400, 120, 1)
		if err != nil {
			t.Fatalf("register failed: %v", err)
		}
		if id != 1 {
			t.Errorf("first ID = %d, want 1", id)
		}
		if r.Count() != 1 {
			t.Errorf("count = %d, want 1", r.Count())
		}
	})

	t.Run("monotonic IDs", func(t *testing.T) {
		r := NewRegistry(16)
		id1, _ := r.Register("a", 100, 100, 30)
		id2, _ := r.Register("b", 100, 100, 30)
		id3, _ := r.Register("c", 100, 100, 30)

		if id1 >= id2 || id2 >= id3 {
			t.Errorf("IDs not monotonic: %d, %d, %d", id1, id2, id3)
		}
	})

	t.Run("IDs never reused after unregister", func(t *testing.T) {
		r := NewRegistry(16)
		id1, _ := r.Register("a", 100, 100, 30)
		r.Unregister(id1)
		id2, _ := r.Register("b", 100, 100, 30)

		if id2 <= id1 {
			t.Errorf("ID reused: id1=%d, id2=%d", id1, id2)
		}
	})

	t.Run("max capacity", func(t *testing.T) {
		r := NewRegistry(3)
		for i := range 3 {
			_, err := r.Register(fmt.Sprintf("mod%d", i), 100, 100, 30)
			if err != nil {
				t.Fatalf("register %d failed: %v", i, err)
			}
		}

		_, err := r.Register("overflow", 100, 100, 30)
		if !errors.Is(err, ErrMaxModules) {
			t.Errorf("overflow err = %v, want ErrMaxModules", err)
		}
	})

	t.Run("name collision", func(t *testing.T) {
		r := NewRegistry(16)
		_, err := r.Register("clock", 400, 120, 1)
		if err != nil {
			t.Fatalf("first register failed: %v", err)
		}

		_, err = r.Register("clock", 400, 120, 1)
		if !errors.Is(err, ErrNameTaken) {
			t.Errorf("duplicate name err = %v, want ErrNameTaken", err)
		}
	})

	t.Run("name reusable after unregister", func(t *testing.T) {
		r := NewRegistry(16)
		id, _ := r.Register("clock", 400, 120, 1)
		r.Unregister(id)

		id2, err := r.Register("clock", 400, 120, 1)
		if err != nil {
			t.Fatalf("re-register failed: %v", err)
		}
		if id2 <= id {
			t.Errorf("reused ID: old=%d, new=%d", id, id2)
		}
	})

	t.Run("initial state is connecting", func(t *testing.T) {
		r := NewRegistry(16)
		id, _ := r.Register("mod", 100, 100, 30)
		mod, ok := r.Lookup(id)
		if !ok {
			t.Fatal("lookup failed")
		}
		if mod.State != StateConnecting {
			t.Errorf("initial state = %v, want StateConnecting", mod.State)
		}
	})

	t.Run("metadata stored correctly", func(t *testing.T) {
		r := NewRegistry(16)
		before := time.Now()
		id, _ := r.Register("weather", 320, 240, 60)
		after := time.Now()

		mod, ok := r.Lookup(id)
		if !ok {
			t.Fatal("lookup failed")
		}
		if mod.Name != "weather" {
			t.Errorf("name = %q, want %q", mod.Name, "weather")
		}
		if mod.Width != 320 {
			t.Errorf("width = %d, want 320", mod.Width)
		}
		if mod.Height != 240 {
			t.Errorf("height = %d, want 240", mod.Height)
		}
		if mod.FPS != 60 {
			t.Errorf("fps = %d, want 60", mod.FPS)
		}
		if mod.ConnectedAt.Before(before) || mod.ConnectedAt.After(after) {
			t.Errorf("ConnectedAt = %v, want between %v and %v", mod.ConnectedAt, before, after)
		}
	})
}

func TestRegistry_Unregister(t *testing.T) {
	t.Run("removes module", func(t *testing.T) {
		r := NewRegistry(16)
		id, _ := r.Register("mod", 100, 100, 30)
		r.Unregister(id)

		if r.Count() != 0 {
			t.Errorf("count after unregister = %d, want 0", r.Count())
		}
		_, ok := r.Lookup(id)
		if ok {
			t.Error("lookup succeeded after unregister, want not found")
		}
	})

	t.Run("nonexistent ID is no-op", func(t *testing.T) {
		r := NewRegistry(16)
		r.Unregister(999) // should not panic
	})

	t.Run("frees capacity slot", func(t *testing.T) {
		r := NewRegistry(2)
		id1, _ := r.Register("a", 100, 100, 30)
		_, _ = r.Register("b", 100, 100, 30)

		// At capacity.
		_, err := r.Register("c", 100, 100, 30)
		if !errors.Is(err, ErrMaxModules) {
			t.Fatalf("expected ErrMaxModules, got %v", err)
		}

		// Unregister one — should free a slot.
		r.Unregister(id1)
		_, err = r.Register("c", 100, 100, 30)
		if err != nil {
			t.Errorf("register after unregister failed: %v", err)
		}
	})
}

func TestRegistry_Lookup(t *testing.T) {
	t.Run("existing module", func(t *testing.T) {
		r := NewRegistry(16)
		id, _ := r.Register("mod", 100, 200, 30)
		mod, ok := r.Lookup(id)
		if !ok {
			t.Fatal("lookup failed")
		}
		if mod.ID != id || mod.Name != "mod" {
			t.Errorf("lookup returned wrong module: %+v", mod)
		}
	})

	t.Run("nonexistent module", func(t *testing.T) {
		r := NewRegistry(16)
		_, ok := r.Lookup(42)
		if ok {
			t.Error("lookup succeeded for nonexistent ID")
		}
	})

	t.Run("returns copy", func(t *testing.T) {
		r := NewRegistry(16)
		id, _ := r.Register("mod", 100, 100, 30)
		mod1, _ := r.Lookup(id)
		mod1.Name = "mutated"

		mod2, _ := r.Lookup(id)
		if mod2.Name == "mutated" {
			t.Error("Lookup returned a reference instead of a copy")
		}
	})
}

func TestRegistry_LookupByName(t *testing.T) {
	t.Run("existing module", func(t *testing.T) {
		r := NewRegistry(16)
		id, _ := r.Register("clock", 400, 120, 1)
		mod, ok := r.LookupByName("clock")
		if !ok {
			t.Fatal("lookup by name failed")
		}
		if mod.ID != id {
			t.Errorf("ID = %d, want %d", mod.ID, id)
		}
	})

	t.Run("nonexistent name", func(t *testing.T) {
		r := NewRegistry(16)
		_, ok := r.LookupByName("nonexistent")
		if ok {
			t.Error("lookup by name succeeded for nonexistent name")
		}
	})
}

func TestRegistry_SetState(t *testing.T) {
	t.Run("transitions state", func(t *testing.T) {
		r := NewRegistry(16)
		id, _ := r.Register("mod", 100, 100, 30)

		r.SetState(id, StateHandshaking)
		mod, _ := r.Lookup(id)
		if mod.State != StateHandshaking {
			t.Errorf("state = %v, want StateHandshaking", mod.State)
		}

		r.SetState(id, StateActive)
		mod, _ = r.Lookup(id)
		if mod.State != StateActive {
			t.Errorf("state = %v, want StateActive", mod.State)
		}

		r.SetState(id, StateDisconnected)
		mod, _ = r.Lookup(id)
		if mod.State != StateDisconnected {
			t.Errorf("state = %v, want StateDisconnected", mod.State)
		}
	})

	t.Run("nonexistent ID is no-op", func(t *testing.T) {
		r := NewRegistry(16)
		r.SetState(999, StateActive) // should not panic
	})
}

func TestRegistry_UpdateLastFrame(t *testing.T) {
	t.Run("updates timestamp", func(t *testing.T) {
		r := NewRegistry(16)
		id, _ := r.Register("mod", 100, 100, 30)

		ts := time.Now().Add(5 * time.Second)
		r.UpdateLastFrame(id, ts)

		mod, _ := r.Lookup(id)
		if !mod.LastFrameAt.Equal(ts) {
			t.Errorf("LastFrameAt = %v, want %v", mod.LastFrameAt, ts)
		}
	})

	t.Run("nonexistent ID is no-op", func(t *testing.T) {
		r := NewRegistry(16)
		r.UpdateLastFrame(999, time.Now()) // should not panic
	})
}

func TestRegistry_Count(t *testing.T) {
	r := NewRegistry(16)
	if r.Count() != 0 {
		t.Fatalf("initial count = %d, want 0", r.Count())
	}

	r.Register("a", 100, 100, 30)
	r.Register("b", 100, 100, 30)
	if r.Count() != 2 {
		t.Errorf("count = %d, want 2", r.Count())
	}

	id, _ := r.Register("c", 100, 100, 30)
	r.Unregister(id)
	if r.Count() != 2 {
		t.Errorf("count after unregister = %d, want 2", r.Count())
	}
}

func TestRegistry_All(t *testing.T) {
	t.Run("empty registry", func(t *testing.T) {
		r := NewRegistry(16)
		all := r.All()
		if len(all) != 0 {
			t.Errorf("All() len = %d, want 0", len(all))
		}
	})

	t.Run("returns all modules", func(t *testing.T) {
		r := NewRegistry(16)
		r.Register("a", 100, 100, 30)
		r.Register("b", 200, 200, 60)
		r.Register("c", 300, 300, 1)

		all := r.All()
		if len(all) != 3 {
			t.Fatalf("All() len = %d, want 3", len(all))
		}

		names := make(map[string]bool)
		for _, m := range all {
			names[m.Name] = true
		}
		for _, want := range []string{"a", "b", "c"} {
			if !names[want] {
				t.Errorf("missing module %q in All() result", want)
			}
		}
	})

	t.Run("returns copies", func(t *testing.T) {
		r := NewRegistry(16)
		r.Register("mod", 100, 100, 30)

		all := r.All()
		all[0].Name = "mutated"

		mod, _ := r.LookupByName("mod")
		if mod == nil {
			t.Error("original module name was mutated via All() result")
		}
	})
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry(1000)
	var wg sync.WaitGroup

	// Concurrent writers (register).
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("module-%d", idx)
			_, _ = r.Register(name, 100, 100, 30)
		}(i)
	}

	// Concurrent readers (lookup, count, all).
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Count()
			_ = r.All()
			_, _ = r.Lookup(1)
			_, _ = r.LookupByName("module-0")
		}()
	}

	// Concurrent state updates.
	for i := range 50 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r.SetState(uint64(idx+1), StateActive)
			r.UpdateLastFrame(uint64(idx+1), time.Now())
		}(i)
	}

	wg.Wait()

	// Verify consistency: count matches actual registered modules.
	count := r.Count()
	all := r.All()
	if count != len(all) {
		t.Errorf("count=%d != len(All())=%d", count, len(all))
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateConnecting, "connecting"},
		{StateHandshaking, "handshaking"},
		{StateActive, "active"},
		{StateDisconnected, "disconnected"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func BenchmarkRegister(b *testing.B) {
	r := NewRegistry(b.N + 1)
	b.ResetTimer()
	for i := range b.N {
		name := fmt.Sprintf("module-%d", i)
		_, _ = r.Register(name, 100, 100, 30)
	}
}

func BenchmarkLookup(b *testing.B) {
	r := NewRegistry(1000)
	for i := range 1000 {
		r.Register(fmt.Sprintf("module-%d", i), 100, 100, 30)
	}

	b.ResetTimer()
	for i := range b.N {
		id := uint64(i%1000) + 1
		_, _ = r.Lookup(id)
	}
}

func BenchmarkLookupByName(b *testing.B) {
	r := NewRegistry(1000)
	names := make([]string, 1000)
	for i := range 1000 {
		names[i] = fmt.Sprintf("module-%d", i)
		r.Register(names[i], 100, 100, 30)
	}

	b.ResetTimer()
	for i := range b.N {
		_, _ = r.LookupByName(names[i%1000])
	}
}
