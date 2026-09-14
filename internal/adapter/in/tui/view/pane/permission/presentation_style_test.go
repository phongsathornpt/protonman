package permission

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestPermissionSelectionUsesSemanticSelectionStyle(t *testing.T) {
	view := PermissionView(PermissionSnapshot{
		Width: 60, Height: 24, Tone: panecommon.ToneWarning,
		Title: "Permission required", ToolName: "edit", ToolKind: "write",
		Detail: "internal/tui/theme.go", Options: []string{"Allow once", "Deny"}, Index: 0,
	})
	selected := ""
	for _, row := range view.Rows {
		if strings.Contains(ansi.Strip(row), "Allow once") {
			selected = row
			break
		}
	}
	if selected == "" {
		t.Fatal("selected permission row not rendered")
	}
	want := tuistyle.SelectionStyle.Render(tuistyle.GlyphPrompt + "Allow once")
	if selected != want {
		t.Fatalf("selected row = %q, want semantic selection render %q", selected, want)
	}
}

func TestPermissionTinySelectionKeepsSemanticFocus(t *testing.T) {
	view := PermissionView(PermissionSnapshot{
		Width: 24, Height: 10, Tone: panecommon.ToneWarning,
		Title: "Permission required", ToolName: "bash", ToolKind: "exec",
		Detail: "go test ./...", Options: []string{"Allow", "Deny"}, Index: 1,
	})
	if len(view.Rows) != 3 {
		t.Fatalf("tiny permission rows = %d, want 3", len(view.Rows))
	}
	want := tuistyle.SelectionStyle.Render(tuistyle.GlyphPrompt + "Deny")
	if view.Rows[2] != want {
		t.Fatalf("tiny selected row = %q, want %q", view.Rows[2], want)
	}
}
