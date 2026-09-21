package permission

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
)

func permissionFixture(width, height int) PermissionSnapshot {
	return PermissionSnapshot{
		Width: width, Height: height, Index: 1,
		Title: "Permission required — modifies workspace",
		Tone:  panecommon.ToneError, ToolName: "Edit", ToolKind: "edit",
		Detail:       "internal/adapter/in/tui/runtime/presentation_runtime.go",
		Options:      []string{"Allow once", "Allow session", "Deny"},
		ShortcutHint: "y once · s session · n deny",
	}
}

func TestPermissionViewResponsiveModes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		height int
		mode   panecommon.LayoutMode
	}{
		{"normal", 80, 24, panecommon.LayoutNormal},
		{"compact", 32, 18, panecommon.LayoutCompact},
		{"tiny", 20, 12, panecommon.LayoutTiny},
		{"narrow-compact", 32, 24, panecommon.LayoutCompact},
		{"narrow-tiny", 20, 24, panecommon.LayoutTiny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := panecommon.ModeForSize(tc.width, tc.height); got != tc.mode {
				t.Fatalf("mode = %v, want %v", got, tc.mode)
			}
			rendered := PermissionView(permissionFixture(tc.width, tc.height))
			if rendered.Inline != "" || len(rendered.Rows) == 0 {
				t.Fatalf("unexpected render: inline=%q rows=%d", rendered.Inline, len(rendered.Rows))
			}
			plain := ansi.Strip(strings.Join(rendered.Rows, "\n"))
			if !strings.Contains(plain, "Allow session") || !strings.Contains(plain, "› Allow session") {
				t.Fatalf("selected option missing: %q", plain)
			}
			switch tc.mode {
			case panecommon.LayoutTiny:
				if len(rendered.Rows) != 3 {
					t.Fatalf("tiny permission rows = %d, want 3", len(rendered.Rows))
				}
			case panecommon.LayoutCompact:
				if !strings.Contains(plain, "more lines truncated") {
					t.Fatalf("compact permission did not truncate details: %q", plain)
				}
			}
			modal := panecommon.RenderToneModal(tc.width, tc.height, rendered.Tone, rendered.Rows)
			for _, line := range strings.Split(modal, "\n") {
				if width := ansi.StringWidth(line); width > tc.width {
					t.Fatalf("modal width=%d terminal=%d: %q", width, tc.width, ansi.Strip(line))
				}
			}
		})
	}
}

func TestPermissionViewParkedUsesInlinePresentation(t *testing.T) {
	snapshot := permissionFixture(32, 18)
	snapshot.Parked = true
	rendered := PermissionView(snapshot)
	if rendered.Inline == "" || len(rendered.Rows) != 0 {
		t.Fatalf("parked render = inline %q rows %d", rendered.Inline, len(rendered.Rows))
	}
	if width := ansi.StringWidth(rendered.Inline); width > snapshot.Width-2 {
		t.Fatalf("parked inline width = %d, want <= %d", width, snapshot.Width-2)
	}
}

func TestPermissionViewRendersDiffPreview(t *testing.T) {
	snapshot := permissionFixture(80, 24)
	snapshot.DiffPreview = []string{
		"@@ -1,3 +1,4 @@",
		"-old line",
		"+new line",
		"+extra line",
	}

	rendered := PermissionView(snapshot)
	plain := ansi.Strip(strings.Join(rendered.Rows, "\n"))

	if !strings.Contains(plain, "@@ -1,3 +1,4 @@") {
		t.Fatalf("expected hunk header in permission view, got:\n%s", plain)
	}
	if !strings.Contains(plain, "-old line") || !strings.Contains(plain, "+new line") {
		t.Fatalf("expected diff lines in permission view, got:\n%s", plain)
	}
}
