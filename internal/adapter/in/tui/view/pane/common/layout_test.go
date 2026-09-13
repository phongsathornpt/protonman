package common

import "testing"

func TestModeForSize(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		want   LayoutMode
	}{
		{name: "wide", width: 80, height: 24, want: LayoutNormal},
		{name: "compact width", width: 32, height: 24, want: LayoutCompact},
		{name: "compact height", width: 80, height: 18, want: LayoutCompact},
		{name: "tiny width", width: 20, height: 24, want: LayoutTiny},
		{name: "tiny height", width: 80, height: 12, want: LayoutTiny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ModeForSize(tt.width, tt.height); got != tt.want {
				t.Fatalf("ModeForSize(%d, %d) = %v, want %v", tt.width, tt.height, got, tt.want)
			}
		})
	}
}

func TestModeForHeightPreservesPaneBreakpoints(t *testing.T) {
	if got := ModeForHeight(24); got != LayoutNormal {
		t.Fatalf("24 rows mode = %v", got)
	}
	if got := ModeForHeight(18); got != LayoutCompact {
		t.Fatalf("18 rows mode = %v", got)
	}
	if got := ModeForHeight(12); got != LayoutTiny {
		t.Fatalf("12 rows mode = %v", got)
	}
}

func TestResolveProfile(t *testing.T) {
	tests := []struct {
		name          string
		width         int
		height        int
		hasBottomView bool
		wantMode      LayoutMode
		wantHeader    bool
	}{
		{name: "wide", width: 80, height: 24, wantMode: LayoutNormal, wantHeader: true},
		{name: "compact", width: 32, height: 24, wantMode: LayoutCompact, wantHeader: true},
		{name: "tiny", width: 20, height: 24, wantMode: LayoutTiny, wantHeader: true},
		{name: "very short terminal", width: 80, height: 9, wantMode: LayoutTiny, wantHeader: false},
		{name: "bottom view on short terminal", width: 80, height: 17, hasBottomView: true, wantMode: LayoutCompact, wantHeader: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveProfile(tt.width, tt.height, tt.hasBottomView)
			if got.Mode != tt.wantMode {
				t.Fatalf("mode = %v, want %v", got.Mode, tt.wantMode)
			}
			if got.ShowHeader != tt.wantHeader {
				t.Fatalf("show header = %v, want %v", got.ShowHeader, tt.wantHeader)
			}
		})
	}
}

func TestProfileContentWidth(t *testing.T) {
	profile := ResolveProfile(80, 24, false)
	if got, want := profile.ContentWidth(80), 78; got != want {
		t.Fatalf("content width = %d, want %d", got, want)
	}
	if got := profile.ContentWidth(1); got != 1 {
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
		{name: "negative chrome", width: 80, height: 24, chromeHeight: -2, wantViewport: 24},
		{name: "invalid terminal", width: 0, height: 0, chromeHeight: 0, wantViewport: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveFrameGeometry(tt.width, tt.height, tt.chromeHeight)
			if got.ViewportHeight != tt.wantViewport {
				t.Fatalf("viewport height = %d, want %d", got.ViewportHeight, tt.wantViewport)
			}
			if got.TerminalWidth < 1 || got.TerminalHeight < 1 {
				t.Fatalf("invalid geometry: %+v", got)
			}
		})
	}
}
