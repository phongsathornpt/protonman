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

func expectedComposerTopRail(m *bubbleModel, style lipgloss.Style) string {
	icons := tuistyle.OrUnicodeIcons(m.icons)
	return m.buildComposerTopRail(m.layout.width, style, icons, false)
}

func TestFocusedComposerCardTopRail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	line := promptDividerLine(t, m)
	plain := ansi.Strip(line)
	if !strings.HasPrefix(plain, "╭─") || !strings.HasSuffix(plain, "─╮") {
		t.Fatalf("focused card top rail missing frame glyphs: %q", plain)
	}
	if !strings.Contains(plain, "universal") || !strings.Contains(plain, "ask") {
		t.Fatalf("focused card top rail missing profile/mode badges: %q", plain)
	}
	if got := ansi.StringWidth(line); got != m.layout.width {
		t.Fatalf("focused card top rail width = %d, want %d", got, m.layout.width)
	}
}

func TestPromptDividerUsesFocusedStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	if got, want := promptDividerLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderFocused); got != want {
		t.Fatalf("focused top rail = %q, want %q", got, want)
	}
}

func TestPromptDividerPermissionOverridesFocus(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.openPermission(permissionRequest{
		Request:  permission.Request{ToolName: "edit", ToolKind: permission.ToolEdit, Detail: "main.go"},
		Response: make(chan permissionResponse, 1),
	})
	if got, want := promptDividerLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderWarning); got != want {
		t.Fatalf("permission top rail = %q, want %q", got, want)
	}
}

func TestPromptDividerDenyModeUsesErrorStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeDeny, emptyTodoItems())
	if got, want := promptDividerLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderError); got != want {
		t.Fatalf("deny top rail = %q, want %q", got, want)
	}
}

func TestPromptDividerBlurredComposerUsesIdleStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.panes.bottom.prompt().Blur()
	if got, want := promptDividerLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderNormal); got != want {
		t.Fatalf("idle top rail = %q, want %q", got, want)
	}
}

func TestPromptDividerBashModeUsesBashStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.setBashMode(true)
	if got, want := promptDividerLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderBash); got != want {
		t.Fatalf("bash top rail = %q, want %q", got, want)
	}
}

func TestPromptDividerPlanModeUsesPlanStyle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.setPlanEnabled(true)
	if got, want := promptDividerLine(t, m), expectedComposerTopRail(m, tuistyle.ComposerBorderPlan); got != want {
		t.Fatalf("plan top rail = %q, want %q", got, want)
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
