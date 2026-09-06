package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/projectTHORN/proton/internal/permission"
	tododomain "github.com/projectTHORN/proton/internal/todo"
)

func TestPickerVisibleRows(t *testing.T) {
	tests := []struct {
		height  int
		maximum int
		want    int
	}{
		{height: 12, maximum: 6, want: 2},
		{height: 14, maximum: 6, want: 3},
		{height: 18, maximum: 6, want: 4},
		{height: 24, maximum: 6, want: 6},
		{height: 24, maximum: 5, want: 5},
	}
	for _, tc := range tests {
		if got := pickerVisibleRows(tc.height, tc.maximum); got != tc.want {
			t.Fatalf("pickerVisibleRows(%d, %d) = %d, want %d", tc.height, tc.maximum, got, tc.want)
		}
	}
}

func TestLayoutModeBreakpoints(t *testing.T) {
	if got := layoutModeForHeight(24); got != layoutNormal {
		t.Fatalf("24 rows mode = %v", got)
	}
	if got := layoutModeForHeight(18); got != layoutCompact {
		t.Fatalf("18 rows mode = %v", got)
	}
	if got := layoutModeForHeight(12); got != layoutTiny {
		t.Fatalf("12 rows mode = %v", got)
	}
}

func TestPickersFitResponsiveTerminalHeights(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 18}, {40, 14}, {24, 12}} {
		m := newTestSkillsModel(t, 10)
		m.resize(size[0], size[1])

		modelView := newModelSelectPaneView(m).Render(m)
		if got := lipgloss.Height(modelView); got > size[1] {
			t.Fatalf("model picker height %d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if got := lipgloss.Width(modelView); got > size[0] {
			t.Fatalf("model picker width %d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}

		skillsView := (&skillsPaneView{}).Render(m)
		if got := lipgloss.Height(skillsView); got > size[1] {
			t.Fatalf("skills picker height %d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if got := lipgloss.Width(skillsView); got > size[0] {
			t.Fatalf("skills picker width %d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
	}
}

func TestCompactLayoutReducesChrome(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{{Text: "one"}, {Text: "two"}})
	m.activeModel = "provider/a-very-long-model-name"
	m.resize(60, 18)
	if got := m.todoView(); !strings.Contains(got, "Tasks 0/2") || strings.Contains(got, "one") {
		t.Fatalf("compact todo = %q, want summary only", got)
	}
	if got := m.infoView(); strings.Contains(got, "ctrl+t transcript") || !strings.Contains(got, "ctrl+p model") {
		t.Fatalf("compact info = %q", got)
	}

	m.resize(24, 12)
	if got := m.todoView(); got != "" {
		t.Fatalf("tiny todo = %q, want hidden", got)
	}
	if strings.Contains(m.promptView(), "╭") || strings.Contains(m.promptView(), "╰") {
		t.Fatalf("tiny prompt still renders box chrome: %q", m.promptView())
	}
	if got := lipgloss.Height(m.View()); got > 12 {
		t.Fatalf("tiny live view height = %d, want <= 12", got)
	}
}

func TestRunningToolUsesTranscriptAsProgressSurface(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	m.activity = "running read_file"
	m.historyState.StartTool("read_file")
	if got := m.statusView(); got != "" {
		t.Fatalf("running tool status duplicates transcript progress: %q", got)
	}

	m.historyState.CommitActive()
	m.historyState.StartThinking()
	if got := m.statusView(); got == "" {
		t.Fatal("thinking state should retain the global status row")
	}
}

func TestTodoDefaultsToSummaryAndCtrlOExpands(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{{Text: "first"}, {Text: "second"}})
	m.resize(80, 24)
	if got := m.todoView(); !strings.Contains(got, "Tasks 0/2") || strings.Contains(got, "first") {
		t.Fatalf("default todo = %q, want summary", got)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = updated.(*bubbleModel)
	if got := m.todoView(); !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("expanded todo missing details: %q", got)
	}
}

func TestWelcomeCardUsesVerticalHierarchyAndTruncates(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = "/a/very/long/workspace/path/that/does/not/fit/in/a/narrow/terminal"
	m.activeModel = "provider/a-very-long-model-name-that-does-not-fit"
	m.activeProvider = "provider-name"
	m.resize(32, 14)
	card := m.welcomeCard()
	if !strings.Contains(card, "Proton") || !strings.Contains(card, "provider-name") {
		t.Fatalf("welcome card missing hierarchy: %q", card)
	}
	for _, line := range strings.Split(card, "\n") {
		if got := lipgloss.Width(line); got > 30 {
			t.Fatalf("welcome line width = %d, want <= 30: %q", got, line)
		}
	}
}

func TestTodoAllCompletedStillShowsSummaryAndDetails(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{
		{ID: "one", Text: "one", Status: tododomain.StatusCompleted},
		{ID: "two", Text: "two", Status: tododomain.StatusCompleted},
	})
	m.resize(80, 24)
	if got := m.todoView(); !strings.Contains(got, "Tasks 2/2 ✓") {
		t.Fatalf("completed summary = %q", got)
	}
	m.todoExpanded = true
	if got := m.todoView(); !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatalf("completed details = %q", got)
	}
}

func TestTodoViewOrdersActivePendingCompletedAndFitsWidth(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{
		{ID: "done", Text: "completed task", Status: tododomain.StatusCompleted},
		{ID: "pending", Text: "pending task", Status: tododomain.StatusPending},
		{ID: "active", Text: "active task with a deliberately long description that should wrap safely on narrow terminals", Status: tododomain.StatusInProgress},
	})
	m.resize(32, 24)
	m.todoExpanded = true
	got := m.todoView()
	if !(strings.Index(got, "active task") < strings.Index(got, "pending task") && strings.Index(got, "pending task") < strings.Index(got, "completed task")) {
		t.Fatalf("todo order = %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if width := lipgloss.Width(line); width > 30 {
			t.Fatalf("todo line width = %d: %q", width, line)
		}
	}
}

func TestTodoVisibleRowsGrowWithTerminalHeight(t *testing.T) {
	if small, large := todoVisibleRows(20), todoVisibleRows(30); large <= small {
		t.Fatalf("rows did not grow: %d -> %d", small, large)
	}
}
