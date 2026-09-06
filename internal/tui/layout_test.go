package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/projectTHORN/proton/internal/permission"
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
