//go:build desktop

package desktop

import "testing"

func TestContextDrawerWidthForWindow(t *testing.T) {
	tests := []struct {
		name        string
		windowWidth float32
		want        float32
	}{
		{name: "unknown window uses preferred width", windowWidth: 0, want: contextDrawerPreferredWidth},
		{name: "narrow window uses minimum", windowWidth: 600, want: contextDrawerMinWidth},
		{name: "medium window scales", windowWidth: 900, want: 270},
		{name: "wide window caps at preferred", windowWidth: 1600, want: contextDrawerPreferredWidth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contextDrawerWidthFor(tt.windowWidth); got != tt.want {
				t.Fatalf("contextDrawerWidthFor(%v) = %v, want %v", tt.windowWidth, got, tt.want)
			}
		})
	}
}
