package common

import "testing"

func TestPaneContentWidth(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  int
	}{
		{name: "zero clamps to one", width: 0, want: 1},
		{name: "below inset clamps to one", width: 7, want: 1},
		{name: "equal to inset clamps to one", width: 8, want: 1},
		{name: "one past inset", width: 9, want: 1},
		{name: "two past inset", width: 10, want: 2},
		{name: "three past inset", width: 11, want: 3},
		{name: "typical pane", width: 80, want: 72},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PaneContentWidth(tt.width); got != tt.want {
				t.Fatalf("PaneContentWidth(%d) = %d, want %d", tt.width, got, tt.want)
			}
		})
	}
}

func TestPaneHelpWidth(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  int
	}{
		{name: "zero clamps to one", width: 0, want: 1},
		{name: "below inset clamps to one", width: 5, want: 1},
		{name: "equal to inset clamps to one", width: 6, want: 1},
		{name: "one past inset", width: 7, want: 1},
		{name: "two past inset", width: 8, want: 2},
		{name: "three past inset", width: 9, want: 3},
		{name: "four past inset", width: 10, want: 4},
		{name: "typical pane", width: 80, want: 74},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PaneHelpWidth(tt.width); got != tt.want {
				t.Fatalf("PaneHelpWidth(%d) = %d, want %d", tt.width, got, tt.want)
			}
		})
	}
}
