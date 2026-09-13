package common

import "testing"

func TestModeForSizeExactBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		want   LayoutMode
	}{
		{name: "width below compact", width: 23, height: 24, want: LayoutTiny},
		{name: "width at compact", width: 24, height: 24, want: LayoutCompact},
		{name: "width below normal", width: 39, height: 24, want: LayoutCompact},
		{name: "width at normal", width: 40, height: 24, want: LayoutNormal},
		{name: "height below compact", width: 80, height: 13, want: LayoutTiny},
		{name: "height at compact", width: 80, height: 14, want: LayoutCompact},
		{name: "height below normal", width: 80, height: 19, want: LayoutCompact},
		{name: "height at normal", width: 80, height: 20, want: LayoutNormal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ModeForSize(tt.width, tt.height); got != tt.want {
				t.Fatalf("ModeForSize(%d, %d) = %v, want %v", tt.width, tt.height, got, tt.want)
			}
		})
	}
}

func TestResolveProfileHeaderVisibilityBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		height        int
		hasBottomView bool
		want          bool
	}{
		{name: "below base header threshold", height: 9, want: false},
		{name: "at base header threshold", height: 10, want: true},
		{name: "bottom view below overlay threshold", height: 17, hasBottomView: true, want: false},
		{name: "bottom view at overlay threshold", height: 18, hasBottomView: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := ResolveProfile(80, tt.height, tt.hasBottomView)
			if profile.ShowHeader != tt.want {
				t.Fatalf("ShowHeader at height %d bottom=%v = %v, want %v", tt.height, tt.hasBottomView, profile.ShowHeader, tt.want)
			}
		})
	}
}

func TestFrameGeometryCanonicalTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {72, 20}, {80, 24}, {100, 30}, {120, 32}, {160, 50}} {
		const chromeHeight = 7
		got := ResolveFrameGeometry(size[0], size[1], chromeHeight)
		if got.TerminalWidth != size[0] || got.TerminalHeight != size[1] {
			t.Fatalf("geometry terminal = %dx%d, want %dx%d", got.TerminalWidth, got.TerminalHeight, size[0], size[1])
		}
		if got.ChromeHeight != chromeHeight {
			t.Fatalf("geometry chrome = %d, want %d", got.ChromeHeight, chromeHeight)
		}
		if got.ViewportHeight != size[1]-chromeHeight {
			t.Fatalf("geometry viewport at %dx%d = %d, want %d", size[0], size[1], got.ViewportHeight, size[1]-chromeHeight)
		}
	}
}

func TestFrameGeometryAlwaysKeepsViewportAlive(t *testing.T) {
	for _, tc := range []struct {
		width  int
		height int
		chrome int
	}{{1, 1, 1}, {16, 8, 12}, {0, 0, 100}, {-10, -4, -1}} {
		got := ResolveFrameGeometry(tc.width, tc.height, tc.chrome)
		if got.TerminalWidth < 1 || got.TerminalHeight < 1 || got.ViewportHeight < 1 {
			t.Fatalf("invalid geometry for %+v: %+v", tc, got)
		}
	}
}
