package tui

import (
	"strings"
	"testing"
)

func TestHistoryStateStreamsAssistantIntoActiveCell(t *testing.T) {
	state := NewHistoryState(100)
	state.Append(&UserCell{Text: "hello"})
	state.AppendAssistantDelta("hel")
	state.AppendAssistantDelta("lo")

	active, ok := state.Active().(*AssistantCell)
	if !ok {
		t.Fatalf("active cell = %T, want *AssistantCell", state.Active())
	}
	if active.Text != "hello" {
		t.Fatalf("active text = %q, want hello", active.Text)
	}
	if got := state.Raw(); got != "hello\nhello" {
		t.Fatalf("raw transcript = %q", got)
	}

	state.CommitActive()
	if state.Active() != nil {
		t.Fatalf("active cell = %T after commit, want nil", state.Active())
	}
	if got := len(state.Cells()); got != 2 {
		t.Fatalf("cell count = %d, want 2", got)
	}
}

func TestHistoryStateToolRunningToCompleted(t *testing.T) {
	state := NewHistoryState(100)
	state.StartTool("bash")

	active, ok := state.Active().(*ToolCell)
	if !ok || !active.Running {
		t.Fatalf("active tool = %#v, want running ToolCell", state.Active())
	}

	exitCode := 0
	state.CompleteTool(ToolCell{Name: "bash", Body: "ok", ExitCode: &exitCode})
	if state.Active() != nil {
		t.Fatal("completed tool should be committed")
	}
	cells := state.Cells()
	if len(cells) != 1 {
		t.Fatalf("cell count = %d, want 1", len(cells))
	}
	toolCell, ok := cells[0].(*ToolCell)
	if !ok {
		t.Fatalf("cell = %T, want *ToolCell", cells[0])
	}
	if toolCell.Running {
		t.Fatal("completed tool is still marked running")
	}
	if got := state.Raw(); !strings.Contains(got, "bash\nok\nexit 0") {
		t.Fatalf("raw transcript missing tool result: %q", got)
	}
}

func TestHistoryStateCompletesPreviouslyCommittedParallelTool(t *testing.T) {
	state := NewHistoryState(100)
	state.StartTool("read_file")
	state.StartTool("grep")

	state.CompleteTool(ToolCell{Name: "read_file", Body: "contents"})
	cells := state.Cells()
	if len(cells) != 2 {
		t.Fatalf("cell count = %d, want 2", len(cells))
	}
	first, ok := cells[0].(*ToolCell)
	if !ok || first.Running || first.Body != "contents" {
		t.Fatalf("first tool = %#v, want completed read_file", cells[0])
	}
	second, ok := cells[1].(*ToolCell)
	if !ok || !second.Running || second.Name != "grep" {
		t.Fatalf("second tool = %#v, want active grep", cells[1])
	}
}

func TestHistoryStateUsesCallIDForSameNameParallelTools(t *testing.T) {
	state := NewHistoryState(100)
	state.StartToolCall("read-1", "read_file")
	state.StartToolCall("read-2", "read_file")

	state.CompleteTool(ToolCell{CallID: "read-1", Name: "read_file", Body: "first"})
	cells := state.Cells()
	if len(cells) != 2 {
		t.Fatalf("cell count = %d, want 2", len(cells))
	}
	first, ok := cells[0].(*ToolCell)
	if !ok || first.CallID != "read-1" || first.Running || first.Body != "first" {
		t.Fatalf("first tool = %#v, want completed read-1", cells[0])
	}
	second, ok := cells[1].(*ToolCell)
	if !ok || second.CallID != "read-2" || !second.Running {
		t.Fatalf("second tool = %#v, want active read-2", cells[1])
	}

	state.CompleteTool(ToolCell{CallID: "read-2", Name: "read_file", Body: "second"})
	cells = state.Cells()
	second = cells[1].(*ToolCell)
	if second.Running || second.Body != "second" {
		t.Fatalf("second tool = %#v, want completed read-2", second)
	}
}

func TestHistoryStateBoundsCommittedScrollback(t *testing.T) {
	state := NewHistoryState(3)
	state.Append(&SystemCell{Text: "one"})
	state.Append(&SystemCell{Text: "two"})
	state.Append(&SystemCell{Text: "three"})
	state.Append(&SystemCell{Text: "four"})

	if got := state.Raw(); got != "two\nthree\nfour" {
		t.Fatalf("trimmed transcript = %q", got)
	}
}

func TestHistoryCellKindEnum(t *testing.T) {
	tests := []struct {
		kind HistoryCellKind
		want string
	}{
		{HistoryCellUnknown, "unknown"},
		{HistoryCellUser, "user"},
		{HistoryCellAssistant, "assistant"},
		{HistoryCellTool, "tool"},
		{HistoryCellSystem, "system"},
		{HistoryCellError, "error"},
		{HistoryCellKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("HistoryCellKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
