package turn

import "testing"

func TestShouldWarnSoftToolBudget(t *testing.T) {
	tests := []struct {
		name         string
		used, max    int
		warned, want bool
	}{
		{"below", 5, 10, false, false},
		{"threshold", 6, 10, false, true},
		{"above", 9, 10, false, true},
		{"already warned", 9, 10, true, false},
		{"unbounded", 100, 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldWarnSoftToolBudget(tt.used, tt.max, tt.warned); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
