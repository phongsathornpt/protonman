//go:build desktop

package desktop

import "testing"

func TestContextDrawerWidthForWindow(t *testing.T) {
	tests := []struct {
		name  string
		width float32
		want  float32
	}{
		{name: "unknown width", width: 0, want: contextDrawerPreferredWidth},
		{name: "narrow window", width: 700, want: contextDrawerMinWidth},
		{name: "medium window", width: 900, want: 270},
		{name: "wide window", width: 1600, want: contextDrawerPreferredWidth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contextDrawerWidthFor(tt.width); got != tt.want {
				t.Fatalf("context drawer width for %v = %v, want %v", tt.width, got, tt.want)
			}
		})
	}
}
