package compose

import (
	"math"
	"testing"
)

func TestSaturateUint32(t *testing.T) {
	maxUint32 := uint64(math.MaxUint32)
	maxInt := int(^uint(0) >> 1)
	wantMaxInt := uint32(maxInt)
	if uint64(maxInt) > maxUint32 {
		wantMaxInt = math.MaxUint32
	}

	tests := []struct {
		name  string
		input int
		want  uint32
	}{
		{name: "negative", input: -1, want: 0},
		{name: "zero", input: 0, want: 0},
		{name: "one", input: 1, want: 1},
		{name: "max int", input: maxInt, want: wantMaxInt},
	}

	// math.MaxUint32 is wider than int on 32-bit systems, so these cases
	// are added only where the boundary is representable as an int.
	if uint64(maxInt) >= maxUint32 {
		tests = append(tests, struct {
			name  string
			input int
			want  uint32
		}{name: "max uint32", input: int(maxUint32), want: math.MaxUint32})
	}
	if uint64(maxInt) > maxUint32 {
		tests = append(tests, struct {
			name  string
			input int
			want  uint32
		}{name: "above max uint32", input: int(maxUint32 + 1), want: math.MaxUint32})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := saturateUint32(tt.input); got != tt.want {
				t.Fatalf("saturateUint32(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
