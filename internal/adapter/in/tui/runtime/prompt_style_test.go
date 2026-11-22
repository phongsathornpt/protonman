package runtime

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func composerTopRailLine(t *testing.T, m *bubbleModel) string {
	t.Helper()
	lines := strings.Split(m.promptView(), "\n")
	if len(lines) < 3 {
		t.Fatalf("prompt view has %d lines, want at least 3: %q", len(lines), m.promptView())
	}
	return lines[0]
}

func expectedComposerTopRail(m *bubbleModel, style lipgloss.Style) string {
	icons := tuistyle.OrUnicodeIcons(m.icons)
	return m.buildComposerTopRail(m.layout.width, style, icons)
}

func TestFocusedComposerCardTopRail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	line := composerTopRailLine(t, m)
	plain := ansi.Strip(line)
	if !strings.HasPrefix(plain, "╭─") || !strings.HasSuffix(plain, "─╮") {
		t.Fatalf("focused card top rail missing frame glyphs: %q", plain)
	}
	if !strings.Contains(plain, "universal") {
		t.Fatalf("focused card top rail missing profile badge: %q", plain)
	}
	if strings.Contains(plain, "ask") {
		t.Fatalf("focused card top rail should not carry the permission badge: %q", plain)
	}
	if got := ansi.StringWidth(line); got != m.layout.width {
		t.Fatalf("focused card top rail width = %d, want %d", got, m.layout.width)
	}
}

func TestComposerTopRailUsesFocusedStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	if got, want := composerTopRailLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderFocused); got != want {
		t.Fatalf("focused top rail = %q, want %q", got, want)
	}
}

func TestComposerTopRailPermissionOverridesFocus(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.openPermission(permissionRequest{
		Request:  permission.Request{ToolName: "edit", ToolKind: permission.ToolEdit, Detail: "main.go"},
		Response: make(chan permissionResponse, 1),
	})
	if got, want := composerTopRailLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderWarning); got != want {
		t.Fatalf("permission top rail = %q, want %q", got, want)
	}
}

func TestComposerTopRailDenyModeUsesErrorStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeDeny, emptyTodoItems())
	if got, want := composerTopRailLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderError); got != want {
		t.Fatalf("deny top rail = %q, want %q", got, want)
	}
}

func TestComposerTopRailBlurredComposerUsesIdleStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.panes.bottom.prompt().Blur()
	if got, want := composerTopRailLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderNormal); got != want {
		t.Fatalf("idle top rail = %q, want %q", got, want)
	}
}

func TestComposerTopRailBashModeUsesBashStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.setBashMode(true)
	if got, want := composerTopRailLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderBash); got != want {
		t.Fatalf("bash top rail = %q, want %q", got, want)
	}
}

func TestComposerTopRailPlanModeUsesPlanStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.setPlanEnabled(true)
	if got, want := composerTopRailLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderPlan); got != want {
		t.Fatalf("plan top rail = %q, want %q", got, want)
	}
}
