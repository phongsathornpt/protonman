package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/projectTHORN/proton/internal/permission"
)

func assertBubbleViewFits(t *testing.T, m *bubbleModel, width, height int) {
	t.Helper()
	m.resize(width, height)
	view := m.View()
	if got := lipgloss.Height(view); got > height {
		t.Fatalf("view height %d exceeds %d at %dx%d:\n%s", got, height, width, height, view)
	}
	for _, line := range strings.Split(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("line width %d exceeds %d at %dx%d: %q", got, width, width, height, line)
		}
	}
}

func TestResponsiveUXSurfacesFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 18}, {40, 14}, {24, 12}} {
		m := newTestSkillsModel(t, 12)
		m.workDir = "/workspace/a/very/long/path/for/responsive/testing"
		m.activeModel = "provider/a-very-long-model-identifier-for-layout-testing"
		m.activeProvider = "provider-with-a-long-name"
		assertBubbleViewFits(t, m, size[0], size[1])

		m.bottom.push(newModelSelectPaneView(m))
		assertBubbleViewFits(t, m, size[0], size[1])
		m.bottom.remove(modelSelectViewID)

		m.bottom.push(&skillsPaneView{})
		assertBubbleViewFits(t, m, size[0], size[1])
		m.bottom.remove(skillsViewID)
	}
}

func TestPermissionReviewFlowFitsNarrowTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.busy = true
	m.modal = &permissionRequest{
		request: permission.Request{
			ToolName:  "bash",
			ToolKind:  permission.ToolBash,
			Detail:    "git status --short --branch",
			Arguments: json.RawMessage(`{"command":"git status --short --branch"}`),
		},
		response: make(chan permissionResponse, 1),
	}
	assertBubbleViewFits(t, m, 24, 12)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*bubbleModel)
	assertBubbleViewFits(t, m, 24, 12)
	if !m.modalParked {
		t.Fatal("esc did not enter transcript review mode")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(*bubbleModel)
	assertBubbleViewFits(t, m, 24, 12)
	if m.modalParked {
		t.Fatal("tab did not return to permission review")
	}
}

func TestLongActivityStatusFitsTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.busy = true
	m.activity = "connecting to a provider with an absurdly long status description that should never wrap chrome"
	for _, width := range []int{80, 40, 24} {
		m.resize(width, 14)
		if got := ansi.StringWidth(m.statusView()); got > width {
			t.Fatalf("status width %d exceeds terminal width %d: %q", got, width, m.statusView())
		}
	}
}
