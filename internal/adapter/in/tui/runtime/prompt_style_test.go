package runtime

import (
	"strings"
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func promptDividerLine(t *testing.T, m *bubbleModel) string {
	t.Helper()
	lines := strings.Split(m.promptView(), "\n")
	if len(lines) < 3 {
		t.Fatalf("prompt view has %d lines, want at least 3: %q", len(lines), m.promptView())
	}
	return lines[0]
}

func expectedPromptDivider(m *bubbleModel, styleName string) string {
	line := strings.Repeat("─", composerUsableWidth(m.layout.width))
	switch styleName {
	case "warning":
		return tuistyle.PromptDividerWarning.Render(line)
	case "error":
		return tuistyle.PromptDividerError.Render(line)
	case "idle":
		return tuistyle.PromptDividerIdle.Render(line)
	default:
		return tuistyle.PromptDividerFocused.Render(line)
	}
}

func TestPromptDividerUsesFocusedStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	if got, want := promptDividerLine(t, m), expectedPromptDivider(m, "focused"); got != want {
		t.Fatalf("focused divider = %q, want %q", got, want)
	}
}

func TestPromptDividerPermissionOverridesFocus(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.openPermission(permissionRequest{
		Request:  permission.Request{ToolName: "edit", ToolKind: permission.ToolEdit, Detail: "main.go"},
		Response: make(chan permissionResponse, 1),
	})
	if got, want := promptDividerLine(t, m), expectedPromptDivider(m, "warning"); got != want {
		t.Fatalf("permission divider = %q, want %q", got, want)
	}
}

func TestPromptDividerDenyModeUsesErrorStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeDeny, emptyTodoItems())
	if got, want := promptDividerLine(t, m), expectedPromptDivider(m, "error"); got != want {
		t.Fatalf("deny divider = %q, want %q", got, want)
	}
}

func TestPromptDividerBlurredComposerUsesIdleStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.panes.bottom.prompt().Blur()
	if got, want := promptDividerLine(t, m), expectedPromptDivider(m, "idle"); got != want {
		t.Fatalf("idle divider = %q, want %q", got, want)
	}
}
