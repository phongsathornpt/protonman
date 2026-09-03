package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
)

func TestNewConversationClearsProviderHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.messages = []model.Message{{Role: model.RoleUser, Content: "old context"}}
	m.appendUser("visible old context")
	m.queue = []string{"queued"}

	if command := m.executeCommand("/new"); command != nil {
		t.Fatalf("/new command = %v, want nil", command)
	}
	if len(m.messages) != 0 {
		t.Fatalf("provider history length = %d, want 0", len(m.messages))
	}
	if len(m.queue) != 0 || len(m.historyState.Cells()) != 0 {
		t.Fatalf("new conversation retained state: queue=%v cells=%v", m.queue, m.historyState.Cells())
	}
}

func TestClearTranscriptPreservesProviderHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.messages = []model.Message{{Role: model.RoleUser, Content: "keep context"}}
	m.appendUser("visible message")
	m.resetTranscript()
	if len(m.messages) != 1 {
		t.Fatalf("clear changed provider history length = %d, want 1", len(m.messages))
	}
}

func TestMultilinePromptUpMovesCursorInsteadOfRecallingHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	prompt := m.bottom.prompt()
	prompt.SetValue("first line\nsecond line")
	prompt.CursorEnd()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(*bubbleModel)
	if got := m.bottom.prompt().Value(); got != "first line\nsecond line" {
		t.Fatalf("up changed multiline draft to %q", got)
	}
	if line := m.bottom.prompt().Line(); line != 0 {
		t.Fatalf("up moved to logical line %d, want 0", line)
	}
}

func TestAssistantMarkdownAndWrapping(t *testing.T) {
	cell := AssistantCell{Text: "# Heading\n\n- item one\n\n```go\nfmt.Println(\"a long line that must wrap\")\n```"}
	lines := cell.RenderWidth(28)
	plain := sanitizeBubbleText(strings.Join(lines, "\n"))
	if strings.Contains(plain, "# Heading") {
		t.Fatalf("heading marker was not rendered: %q", plain)
	}
	for _, expected := range []string{"Heading", "item one", "code", "fmt.Println"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("markdown output missing %q: %q", expected, plain)
		}
	}
	for _, line := range lines {
		if width := ansi.StringWidth(line); width > 28 {
			t.Fatalf("rendered line width = %d, want <= 28: %q", width, line)
		}
	}
}

func TestWrapLinesPreservesLongTokens(t *testing.T) {
	path := "/workspace/project/very-long-dangerous-command-suffix"
	wrapped := strings.Join(wrapLines(path, 12), "")
	if wrapped != path {
		t.Fatalf("long token was dropped or changed: %q", wrapped)
	}
	if got := strings.Join(strings.Fields(wrapWords("alpha beta", 5)), " "); got != "alpha beta" {
		t.Fatalf("word wrapping changed text: %q", got)
	}
}

func TestLiveViewFitsNarrowTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(24, 12)
	m.appendAssistant("# Heading\n\nA very long response with a path /workspace/project/that/keeps/going")
	m.refreshViewport()
	for _, line := range strings.Split(m.View(), "\n") {
		if width := ansi.StringWidth(line); width > 24 {
			t.Fatalf("narrow view line width = %d, want <= 24: %q", width, line)
		}
	}
}

func TestSpinnerStopsSchedulingWhenIdle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	_, command := m.Update(spinnerTickMessage())
	if command != nil {
		t.Fatalf("idle spinner unexpectedly scheduled another tick: %v", command)
	}
}

// spinnerTickMessage keeps this regression test independent of spinner frame
// values while still exercising the Bubble Tea message path.
func spinnerTickMessage() tea.Msg { return spinner.TickMsg{} }
