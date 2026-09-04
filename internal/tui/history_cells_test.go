package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/model"
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

func TestHistoryStateRunningToolSpinner(t *testing.T) {
	state := NewHistoryState(100)
	state.SetSpinnerFrame("⠋")

	state.StartTool("read_file")
	lines := state.RenderLines()
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "⠋") {
		t.Fatalf("expected running tool to contain spinner frame ⠋, got: %v", lines)
	}

	state.SetSpinnerFrame("⠙")
	lines = state.RenderLines()
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "⠙") {
		t.Fatalf("expected running tool to contain updated spinner frame ⠙, got: %v", lines)
	}

	state.CompleteTool(ToolCell{Name: "read_file", Body: "done"})
	lines = state.RenderLines()
	if len(lines) == 0 || strings.Contains(lines[0], "⠙") || strings.Contains(lines[0], "…") {
		t.Fatalf("completed tool should not contain spinner, got: %v", lines)
	}
}

func TestHistoryStateThinkingCellLifecycle(t *testing.T) {
	t.Run("converts to assistant on first delta", func(t *testing.T) {
		state := NewHistoryState(100)
		state.SetSpinnerFrame("⠋")
		state.StartThinking()

		active := state.Active()
		if _, ok := active.(*ThinkingCell); !ok {
			t.Fatalf("active cell = %T, want *ThinkingCell", active)
		}
		rendered := state.RenderLines()
		if len(rendered) == 0 || !strings.Contains(rendered[0], "Thinking…") {
			t.Fatalf("expected thinking render, got: %v", rendered)
		}

		state.AppendAssistantDelta("Hello world")
		active = state.Active()
		assistant, ok := active.(*AssistantCell)
		if !ok {
			t.Fatalf("active cell = %T, want *AssistantCell", active)
		}
		if assistant.Text != "Hello world" {
			t.Fatalf("assistant text = %q, want Hello world", assistant.Text)
		}
	})

	t.Run("discarded on commit if no text", func(t *testing.T) {
		state := NewHistoryState(100)
		state.StartThinking()
		state.CommitActive()
		if state.Active() != nil {
			t.Fatalf("active cell = %T after commit, want nil", state.Active())
		}
		if len(state.Cells()) != 0 {
			t.Fatalf("cells length = %d, want 0 (thinking cell should not be committed)", len(state.Cells()))
		}
	})
}

func TestActivateSkillToolCellCompactRendering(t *testing.T) {
	xmlBody := `<skill_content name="golang-performance">
**Persona:** You are a Go performance engineer.
# Go Performance Optimization
1. Profile before optimizing...
</skill_content>`

	cell := ToolCell{
		Name: "activate_skill",
		Body: xmlBody,
	}

	raw := cell.RawLines()
	for _, line := range raw {
		if strings.Contains(line, "Profile before optimizing") {
			t.Fatalf("RawLines should not contain raw instruction markdown, got: %v", raw)
		}
	}

	rendered := cell.Render()
	joined := strings.Join(rendered, "\n")
	if !strings.Contains(joined, `Activated skill "golang-performance"`) {
		t.Fatalf("expected compact activation badge in render, got: %s", joined)
	}
	if strings.Contains(joined, "Profile before optimizing") {
		t.Fatalf("rendered output contains full skill instructions: %s", joined)
	}
}

func TestLoadInitialMessagesCompactsSkillDetail(t *testing.T) {
	bm := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "")
	bm.loadInitialMessages([]model.Message{
		{
			Role:     model.RoleTool,
			ToolName: "activate_skill",
			Content:  `<skill_content name="golang-code-style">\n# Full instructions...\n</skill_content>`,
		},
		{
			Role:    model.RoleUser,
			Content: "Activated skill pdf-tool [user]:\n# PDF Guide\nLong content here...",
		},
	})

	rendered := strings.Join(bm.historyState.RenderLines(), "\n")
	if strings.Contains(rendered, "Full instructions") {
		t.Fatalf("history rendered full skill instructions from tool message: %s", rendered)
	}
	if strings.Contains(rendered, "Long content here") {
		t.Fatalf("history rendered full skill instructions from user message: %s", rendered)
	}
	if !strings.Contains(rendered, `Activated skill "golang-code-style"`) {
		t.Fatalf("history missing compact badge: %s", rendered)
	}
}
