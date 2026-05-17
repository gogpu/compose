package codec

import "testing"

func TestRegisterAndGet(t *testing.T) {
	// Reset registry for isolated test.
	resetRegistry()
	defer func() {
		// Re-register defaults after test.
		resetRegistry()
		Register(Raw())
		Register(LZ4())
	}()

	raw := Raw()
	Register(raw)

	got := Get(IDRaw)
	if got == nil {
		t.Fatal("Get(IDRaw) returned nil after Register")
	}
	if got.ID() != IDRaw {
		t.Errorf("Get(IDRaw).ID() = %d, want %d", got.ID(), IDRaw)
	}
}

func TestGetUnknownID(t *testing.T) {
	got := Get(0xFF)
	if got != nil {
		t.Errorf("Get(0xFF) = %v, want nil", got)
	}
}

func TestGetRegisteredCodecs(t *testing.T) {
	tests := []struct {
		name string
		id   byte
	}{
		{"Raw", IDRaw},
		{"LZ4", IDLZ4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Get(tt.id)
			if c == nil {
				t.Fatalf("Get(0x%02x) returned nil", tt.id)
			}
			if c.ID() != tt.id {
				t.Errorf("ID() = 0x%02x, want 0x%02x", c.ID(), tt.id)
			}
		})
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	resetRegistry()
	defer func() {
		resetRegistry()
		Register(Raw())
		Register(LZ4())
	}()

	Register(Raw())

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration, got none")
		}
	}()

	// Second registration with same ID should panic.
	Register(Raw())
}

func TestCodecConstants(t *testing.T) {
	if IDRaw != 0x00 {
		t.Errorf("IDRaw = 0x%02x, want 0x00", IDRaw)
	}
	if IDLZ4 != 0x01 {
		t.Errorf("IDLZ4 = 0x%02x, want 0x01", IDLZ4)
	}
}
