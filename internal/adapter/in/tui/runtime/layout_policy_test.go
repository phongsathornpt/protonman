package runtime

import "testing"

func TestResolveLayoutProfile(t *testing.T) {
	tests := []struct {
		name          string
		width         int
		height        int
		hasBottomView bool
		wantMode      layoutMode
		wantHeader    bool
	}{
		{name: "wide", width: 120, height: 32, wantMode: layoutModeWide, wantHeader: true},
		{name: "compact width", width: 80, height: 32, wantMode: layoutModeCompact, wantHeader: true},
		{name: "compact height", width: 120, height: 18, wantMode: layoutModeCompact, wantHeader: true},
		{name: "minimal width", width: 48, height: 32, wantMode: layoutModeMinimal, wantHeader: true},
		{name: "minimal height", width: 120, height: 12, wantMode: layoutModeMinimal, wantHeader: true},
		{name: "hide header on very short terminal", width: 120, height: 9, wantMode: layoutModeMinimal, wantHeader: false},
		{name: "hide header behind bottom overlay on short terminal", width: 120, height: 17, hasBottomView: true, wantMode: layoutModeCompact, wantHeader: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveLayoutProfile(tt.width, tt.height, tt.hasBottomView)
			if got.mode != tt.wantMode {
				t.Fatalf("mode = %v, want %v", got.mode, tt.wantMode)
			}
			if got.showHeader != tt.wantHeader {
				t.Fatalf("showHeader = %v, want %v", got.showHeader, tt.wantHeader)
			}
		})
	}
}

func TestLayoutProfileContentWidth(t *testing.T) {
	profile := resolveLayoutProfile(80, 24, false)
	if got, want := profile.contentWidth(80), 78; got != want {
		t.Fatalf("content width = %d, want %d", got, want)
	}
	if got := profile.contentWidth(1); got != 1 {
		t.Fatalf("tiny content width = %d, want 1", got)
	}
}

func TestResolveFrameGeometry(t *testing.T) {
	tests := []struct {
		name         string
		width        int
		height       int
		chromeHeight int
		wantViewport int
	}{
		{name: "normal", width: 80, height: 24, chromeHeight: 9, wantViewport: 15},
		{name: "chrome consumes terminal", width: 80, height: 8, chromeHeight: 12, wantViewport: 1},
		{name: "negative chrome is clamped", width: 80, height: 24, chromeHeight: -2, wantViewport: 24},
		{name: "invalid terminal height is clamped", width: 0, height: 0, chromeHeight: 0, wantViewport: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveFrameGeometry(tt.width, tt.height, tt.chromeHeight)
			if got.viewportHeight != tt.wantViewport {
				t.Fatalf("viewport height = %d, want %d", got.viewportHeight, tt.wantViewport)
			}
			if got.terminalWidth < 1 || got.terminalHeight < 1 {
				t.Fatalf("invalid geometry: %+v", got)
			}
		})
	}
}
