package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestComposerCardDimensionsAndCorners(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	view := m.promptView()
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("composer card has %d lines, want at least 3: %q", len(lines), view)
	}

	top := ansi.Strip(lines[0])
	bottom := ansi.Strip(lines[len(lines)-1])

	if !strings.HasPrefix(top, "╭─") || !strings.HasSuffix(top, "─╮") {
		t.Fatalf("top rail corners missing: %q", top)
	}
	if !strings.HasPrefix(bottom, "╰─") || !strings.HasSuffix(bottom, "─╯") {
		t.Fatalf("bottom rail corners missing: %q", bottom)
	}

	for i, line := range lines {
		if got := ansi.StringWidth(line); got != 80 {
			t.Fatalf("row %d width = %d, want 80: %q", i, got, line)
		}
		if i > 0 && i < len(lines)-1 {
			stripped := ansi.Strip(line)
			if !strings.HasPrefix(stripped, "│ ") || !strings.HasSuffix(stripped, " │") {
				t.Fatalf("middle line %d missing vertical borders: %q", i, stripped)
			}
		}
	}
}

func TestComposerCardAdaptiveModeStyles(t *testing.T) {
	t.Run("focused agent mode", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.resize(80, 24)
		view := m.promptView()
		top := strings.Split(view, "\n")[0]
		expected := m.buildComposerTopRail(80, tuistyle.ComposerBorderFocused, tuistyle.UnicodeIcons, false)
		if top != expected {
			t.Fatalf("focused agent mode top rail = %q, want %q", top, expected)
		}
		plain := ansi.Strip(top)
		if !strings.Contains(plain, "universal") || !strings.Contains(plain, "ask") {
			t.Fatalf("missing profile or mode badge: %q", plain)
		}
	})

	t.Run("bash mode", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.resize(80, 24)
		m.setBashMode(true)
		view := m.promptView()
		top := strings.Split(view, "\n")[0]
		expected := m.buildComposerTopRail(80, tuistyle.ComposerBorderBash, tuistyle.UnicodeIcons, false)
		if top != expected {
			t.Fatalf("bash mode top rail = %q, want %q", top, expected)
		}
		plain := ansi.Strip(top)
		if !strings.Contains(plain, "! bash direct") {
			t.Fatalf("bash mode badge missing: %q", plain)
		}
	})

	t.Run("plan mode", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.resize(80, 24)
		m.setPlanEnabled(true)
		view := m.promptView()
		top := strings.Split(view, "\n")[0]
		expected := m.buildComposerTopRail(80, tuistyle.ComposerBorderPlan, tuistyle.UnicodeIcons, false)
		if top != expected {
			t.Fatalf("plan mode top rail = %q, want %q", top, expected)
		}
		plain := ansi.Strip(top)
		if !strings.Contains(plain, "plan · read-only") {
			t.Fatalf("plan mode badge missing: %q", plain)
		}
	})

	t.Run("permission warning", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.resize(80, 24)
		m.openPermission(permissionRequest{
			Request:  permission.Request{ToolName: "edit", ToolKind: permission.ToolEdit, Detail: "main.go"},
			Response: make(chan permissionResponse, 1),
		})
		view := m.promptView()
		top := strings.Split(view, "\n")[0]
		expected := m.buildComposerTopRail(80, tuistyle.ComposerBorderWarning, tuistyle.UnicodeIcons, false)
		if top != expected {
			t.Fatalf("permission mode top rail = %q, want %q", top, expected)
		}
	})

	t.Run("deny error", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeDeny, emptyTodoItems())
		m.resize(80, 24)
		view := m.promptView()
		top := strings.Split(view, "\n")[0]
		expected := m.buildComposerTopRail(80, tuistyle.ComposerBorderError, tuistyle.UnicodeIcons, false)
		if top != expected {
			t.Fatalf("deny mode top rail = %q, want %q", top, expected)
		}
	})
}

func TestComposerCardMultilineExpansionAndLineCounter(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	// 1 line
	view1 := m.promptView()
	lines1 := strings.Split(view1, "\n")
	if len(lines1) != 3 {
		t.Fatalf("1 line prompt card should have 3 lines, got %d", len(lines1))
	}
	if strings.Contains(lines1[0], "lines") {
		t.Fatalf("single line card should not have line count indicator: %q", lines1[0])
	}

	// 3 lines
	m.panes.bottom.prompt().SetValue("line 1\nline 2\nline 3")
	m.requestRelayout()
	m.reconcileLayout()

	view3 := m.promptView()
	lines3 := strings.Split(view3, "\n")
	if len(lines3) != 5 { // top + 3 middle + bottom = 5
		t.Fatalf("3 line prompt card should have 5 lines, got %d", len(lines3))
	}
	if !strings.Contains(lines3[0], "3 lines") {
		t.Fatalf("3 line prompt card missing line count indicator: %q", lines3[0])
	}

	// 6 lines
	m.panes.bottom.prompt().SetValue("1\n2\n3\n4\n5\n6")
	m.requestRelayout()
	m.reconcileLayout()

	view6 := m.promptView()
	lines6 := strings.Split(view6, "\n")
	if len(lines6) != 8 { // top + 6 prompt rows + bottom = 8
		t.Fatalf("6 line prompt card should show 6 prompt rows, got %d total rows", len(lines6))
	}
	if !strings.Contains(lines6[0], "6 lines") {
		t.Fatalf("6 line prompt card missing line count indicator: %q", lines6[0])
	}

	for i, line := range lines6 {
		if got := ansi.StringWidth(line); got != 80 {
			t.Fatalf("multiline row %d width = %d, want 80", i, got)
		}
	}
}

func TestComposerCardAttachmentChipStripAndBackspaceRemoval(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	// Create temp image
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "screenshot.png")
	if err := os.WriteFile(imgPath, []byte("fake png bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	m.panes.bottom.attachImage(imgPath)

	view := m.promptView()
	lines := strings.Split(view, "\n")
	if len(lines) != 4 { // top + attachments + prompt + bottom = 4
		t.Fatalf("card with attachment should have 4 lines, got %d:\n%s", len(lines), view)
	}

	chipsLine := ansi.Strip(lines[1])
	if !strings.Contains(chipsLine, "screenshot.png") || !strings.Contains(chipsLine, "✕") {
		t.Fatalf("attachment chips line missing filename or remove glyph: %q", chipsLine)
	}

	// Backspace in empty prompt removes the attachment
	cmd := m.handlePromptKey(testKey(tea.KeyBackspace))
	if cmd != nil {
		t.Fatalf("expected nil cmd on attachment removal, got %v", cmd)
	}

	if len(m.panes.bottom.composer.attachments.localImages) != 0 {
		t.Fatalf("attachment was not removed on backspace in empty prompt")
	}

	viewAfter := m.promptView()
	linesAfter := strings.Split(viewAfter, "\n")
	if len(linesAfter) != 3 {
		t.Fatalf("card after attachment removal should return to 3 lines, got %d", len(linesAfter))
	}
}

func TestComposerCardResponsiveBreakpoints(t *testing.T) {
	for _, width := range []int{20, 24, 32, 40, 60, 80, 120} {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.resize(width, 24)
		view := m.promptView()

		if got := lipgloss.Width(view); got > width {
			t.Fatalf("prompt view width=%d exceeds terminal width=%d: %q", got, width, view)
		}

		plain := ansi.Strip(view)
		if width < 24 {
			// LayoutTiny: minimal borderless prompt fallback
			if strings.Contains(plain, "╭") || strings.Contains(plain, "╰") {
				t.Fatalf("tiny terminal %d should not have card corners: %q", width, plain)
			}
		} else {
			// Enclosed card
			if !strings.Contains(plain, "╭") || !strings.Contains(plain, "╰") {
				t.Fatalf("terminal width %d missing card corners: %q", width, plain)
			}
			lines := strings.Split(view, "\n")
			for i, line := range lines {
				if got := ansi.StringWidth(line); got != width {
					t.Fatalf("width %d line %d actual width=%d: %q", width, i, got, line)
				}
			}
		}
	}
}

func TestComposerCardContextualPlaceholders(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	// Agent mode placeholder
	if got, want := m.panes.bottom.prompt().Placeholder, "Message or /command…"; got != want {
		t.Fatalf("agent idle placeholder = %q, want %q", got, want)
	}

	// Bash mode placeholder
	m.setBashMode(true)
	if got, want := m.panes.bottom.prompt().Placeholder, "Run workspace shell command…"; got != want {
		t.Fatalf("bash mode placeholder = %q, want %q", got, want)
	}

	// Return to agent mode
	m.setBashMode(false)
	if got, want := m.panes.bottom.prompt().Placeholder, "Ask universal to build, test, or type / for commands…"; got != want {
		t.Fatalf("agent restored placeholder = %q, want %q", got, want)
	}
}
