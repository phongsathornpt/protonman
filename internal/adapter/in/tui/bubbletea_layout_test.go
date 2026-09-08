// Code grouped by TUI behavior boundary; shared fixtures live in bubbletea_helpers_test.go.
package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/phongsathornpt/protonman/internal/core/permission"

	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"strings"
	"testing"
	"time"
)

func TestCompletedTodoPaneIsHidden(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{
		{ID: "done", Text: "done", Status: tododomain.StatusCompleted},
		{ID: "also-done", Text: "also done", Status: tododomain.StatusCompleted},
	})
	model.resize(80, 24)
	if strings.Contains(model.View(), "TODO") {
		t.Fatalf("completed TODO pane still visible: %s", model.View())
	}
}

func TestWelcomeSitsAtTopWithoutFloatingBox(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	view := model.View()
	plain := sanitizeBubbleText(view)
	if idx := strings.Index(plain, glyphBrand); idx < 0 || idx > 8 {
		t.Fatalf("welcome is not at the top of the view: %q", plain[:minInt(80, len(plain))])
	}
	if strings.Count(view, "╭") > 1 {
		t.Fatalf("idle view has extra boxes: %s", view)
	}
}

func TestTodoPaneShowsPendingBeforeCompleted(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{
		{ID: "already-done", Text: "already done", Status: tododomain.StatusCompleted},
		{ID: "still-open", Text: "still open", Status: tododomain.StatusPending},
		{ID: "also-done", Text: "also done", Status: tododomain.StatusCompleted},
	})
	model.resize(80, 24)
	model.todoViewState.Expanded = true
	view := model.View()
	if !strings.Contains(view, "still open") {
		t.Fatalf("todo pane hid the pending item: %s", view)
	}
	pendingAt := strings.Index(view, "still open")
	doneAt := strings.Index(view, "already done")
	if doneAt >= 0 && pendingAt > doneAt {
		t.Fatal("completed todo rendered before pending todo")
	}
}

func TestPromptIsSingleRow(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	if model.prompt.Height() != 1 {
		t.Fatalf("prompt height = %d, want 1", model.prompt.Height())
	}
	if strings.Count(model.promptView(), "›") != 1 {
		t.Fatalf("prompt chrome repeated:\n%s", model.promptView())
	}
}

func TestLiveViewFitsTerminal(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{{ID: "one", Text: "one", Status: tododomain.StatusPending}})
	model.resize(80, 24)
	height := lipgloss.Height(model.View())
	if height > 24 {
		t.Fatalf("view height = %d, want <= 24:\n%s", height, model.View())
	}
}

func TestBubbleModelAcceptsTypedRunes(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	if !model.prompt.Focused() {
		t.Fatal("prompt is not focused; textarea will drop every key")
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(*bubbleModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model = updated.(*bubbleModel)

	if got, want := model.prompt.Value(), "hi"; got != want {
		t.Fatalf("typed value = %q, want %q", got, want)
	}
}

func TestBubbleModelRendersComponentLayout(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		[]TodoItem{{ID: "ship", Text: "ship Bubble Tea", Status: tododomain.StatusPending}},
		nil,
		newPermissionBridge(),
		"/tmp/proton",
	)
	model.resize(80, 24)
	model.appendLine("assistant: ready")
	model.refreshViewport()

	view := model.View()
	for _, expected := range []string{
		glyphBrand,
		"█▀█",
		"assistant: ready",
		"Tasks 0/1",
		"ask",
		"›",
		"/help",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Bubble Tea view does not contain %q: %s", expected, view)
		}
	}
}

func TestBubbleModelHistoryUsesTextarea(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
	)
	model.prompt.SetValue(":help")
	if command := model.submit(); command != nil {
		t.Fatal("help submit command != nil")
	}
	model.historyPrevious()
	if got, want := model.prompt.Value(), ":help"; got != want {
		t.Fatalf("history value = %q, want %q", got, want)
	}
	model.historyNext()
	if got := model.prompt.Value(); got != "" {
		t.Fatalf("history next value = %q, want empty", got)
	}
}

func TestSanitizeBubbleTextRemovesControlCharacters(t *testing.T) {
	got := sanitizeBubbleText("hello\x1b[31m\nworld")
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("sanitized text contains escape: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("sanitized text contains newline: %q", got)
	}
}

func TestEmptyStateWithoutRunnerGuidesSlashCommands(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)

	view := model.View()
	for _, expected := range []string{
		"Type a message or /command",
		glyphBrand,
		"█▀█",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("empty state view does not contain %q: %s", expected, view)
		}
	}
	if got, want := model.prompt.Placeholder, "Type a message or /command…"; got != want {
		t.Fatalf("placeholder = %q, want %q", got, want)
	}
}

func TestRefreshViewportPreservesScrollWhenNotFollowing(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	for range 40 {
		model.appendLine("line")
	}
	model.refreshViewport()
	model.viewport.GotoTop()
	model.followTail = false

	model.appendLine("tail")
	model.refreshViewport()
	if model.viewport.AtBottom() {
		t.Fatal("refreshViewport followed the tail after the user scrolled up")
	}
	if model.followTail {
		t.Fatal("followTail was re-enabled after a mid-scroll append")
	}
}

func TestWelcomeCardReprintsAfterClear(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.appendLine("gone")
	model.prompt.SetValue("/clear")
	_ = model.submit()
	model.refreshViewport()
	view := model.View()
	if strings.Contains(plainTranscript(model), "gone") {
		t.Fatal("clear left transcript body")
	}
	if !strings.Contains(view, glyphBrand) || !strings.Contains(view, "█▀█") {
		t.Fatalf("clear did not reprint welcome: %s", view)
	}
}

func TestSpinnerLifecycle(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, nil)

	t.Run("tick returns single tick command without double-batching", func(t *testing.T) {
		model.busy = true
		_, cmd := model.Update(spinner.TickMsg{})
		if cmd == nil {
			t.Fatal("expected non-nil cmd, got nil")
		}
		msg := cmd()
		if _, isBatch := msg.(tea.BatchMsg); isBatch {
			t.Fatal("cmd returned BatchMsg, indicating exponential double-batching of ticks")
		}
	})
}

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		duration time.Duration
		want     string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "0s"},
		{999 * time.Millisecond, "0s"},
		{time.Second, "1s"},
		{5 * time.Second, "5s"},
		{60 * time.Second, "1m0s"},
		{61 * time.Second, "1m1s"},
	}
	for _, tc := range cases {
		got := formatElapsed(tc.duration)
		if got != tc.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tc.duration, got, tc.want)
		}
	}
}
