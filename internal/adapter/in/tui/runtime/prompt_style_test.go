package runtime

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
		return renderPromptDivider(tuistyle.PromptDividerFocused, len([]rune(line)), true)
	}
}

func TestFocusedPromptDividerUsesAccentRail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	line := promptDividerLine(t, m)
	plain := ansi.Strip(line)
	if !strings.HasPrefix(plain, strings.Repeat("─", 8)) {
		t.Fatalf("focused divider missing accent rail: %q", plain)
	}
	if got := lipgloss.Width(line); got != m.layout.width {
		t.Fatalf("focused divider width = %d, want %d", got, m.layout.width)
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

func TestAnimatedPromptDividerMovesAccentWithoutChangingWidth(t *testing.T) {
	const width = 28
	first := renderAnimatedPromptDivider(width, 0)
	second := renderAnimatedPromptDivider(width, 1)
	if first == second {
		t.Fatal("animated divider did not move its accent")
	}
	for phase, line := range []string{
		first,
		second,
		renderAnimatedPromptDivider(width, width),
		renderAnimatedPromptDivider(width, width-5),
	} {
		if got := lipgloss.Width(line); got != width {
			t.Errorf("phase %d width = %d, want %d", phase, got, width)
		}
	}
}

func TestBusyPromptDividerAnimatesUnlessReducedMotion(t *testing.T) {
	for _, reducedMotion := range []bool{false, true} {
		name := "motion enabled"
		if reducedMotion {
			name = "reduced motion"
		}
		t.Run(name, func(t *testing.T) {
			m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
			m.resize(40, 16)
			m.busy = true
			m.reducedMotion = reducedMotion
			before := promptDividerLine(t, m)
			updated, _ := m.Update(spinnerTickMessage())
			m = updated.(*bubbleModel)
			after := promptDividerLine(t, m)
			if reducedMotion && before != after {
				t.Fatalf("reduced-motion divider changed: %q -> %q", before, after)
			}
			if !reducedMotion && before == after {
				t.Fatal("busy divider did not animate")
			}
		})
	}
}
