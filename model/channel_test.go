package model

import "testing"

func TestGetConcurrentTaskLimitSemantics(t *testing.T) {
	tests := []struct {
		name   string
		raw    *int64
		expect int64
	}{
		{"nil means unlimited-zero", nil, 0},
		{"explicit zero means unlimited", int64Ptr(0), 0},
		{"negative treated as unlimited", int64Ptr(-5), 0},
		{"positive passthrough", int64Ptr(7), 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &Channel{ConcurrentTaskLimit: tt.raw}
			if got := ch.GetConcurrentTaskLimit(); got != tt.expect {
				t.Fatalf("GetConcurrentTaskLimit() = %d want %d", got, tt.expect)
			}
		})
	}
}

func int64Ptr(v int64) *int64 { return &v }
