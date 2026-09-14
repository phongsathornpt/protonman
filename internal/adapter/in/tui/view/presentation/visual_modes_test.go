package presentation

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
)

func TestSessionHeaderVisualModes(t *testing.T) {
	cases := []struct {
		name       string
		width      int
		height     int
		wantMode   panecommon.LayoutMode
		wantLines  int
		wantMarker string
	}{
		{name: "normal", width: 80, height: 28, wantMode: panecommon.LayoutNormal, wantLines: 4, wantMarker: "protonMAN"},
		{name: "compact", width: 32, height: 18, wantMode: panecommon.LayoutCompact, wantLines: 2, wantMarker: "◆ protonMAN"},
		{name: "tiny", width: 18, height: 10, wantMode: panecommon.LayoutTiny, wantLines: 1, wantMarker: "◆ protonMAN"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			profile := panecommon.ResolveProfile(tt.width, tt.height, false)
			if profile.Mode != tt.wantMode {
				t.Fatalf("mode = %v, want %v", profile.Mode, tt.wantMode)
			}
			got := RenderSessionHeader(SessionHeaderModel{
				Width: profile.ContentWidth(tt.width), Model: "qwen3.8-27b",
				LowConcurrency: true, GoalActive: true, Branch: "feat/tui-style-v2",
				Compact: profile.CompactHeader(), Minimal: profile.MinimalHeader(),
			})
			lines := strings.Split(ansi.Strip(got), "\n")
			if len(lines) != tt.wantLines {
				t.Fatalf("lines = %d, want %d: %q", len(lines), tt.wantLines, lines)
			}
			if !strings.Contains(lines[0], tt.wantMarker) {
				t.Fatalf("first line = %q, want marker %q", lines[0], tt.wantMarker)
			}
			for _, line := range strings.Split(got, "\n") {
				if width := ansi.StringWidth(line); width > profile.ContentWidth(tt.width) {
					t.Fatalf("line width = %d exceeds content width %d: %q", width, profile.ContentWidth(tt.width), ansi.Strip(line))
				}
			}
		})
	}
}
