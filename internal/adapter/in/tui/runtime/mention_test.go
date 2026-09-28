package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestMentionPopupTrigger(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	// Initially closed
	if m.mentionOpen() {
		t.Fatalf("mentionOpen() = true initially, want false")
	}

	// Type @
	m.panes.bottom.prompt().SetValue("@")
	m.panes.bottom.prompt().CursorEnd()
	m.syncSlashView()

	if !m.mentionOpen() {
		t.Fatalf("mentionOpen() = false after typing @, want true")
	}

	state := m.mentionState()
	if state == nil {
		t.Fatalf("mentionState() is nil")
	}

	matches := m.mentionMatches()
	if len(matches) < 4 {
		t.Fatalf("expected at least 4 agent matches for @, got %d", len(matches))
	}

	// Pinned agents
	if matches[0].Name != "strength" {
		t.Errorf("matches[0] = %q, want strength", matches[0].Name)
	}
}

func TestMentionPopupFiltering(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	m.panes.bottom.prompt().SetValue("@str")
	m.panes.bottom.prompt().CursorEnd()
	m.syncSlashView()

	if !m.mentionOpen() {
		t.Fatalf("mentionOpen() = false for @str, want true")
	}

	matches := m.mentionMatches()
	if len(matches) == 0 {
		t.Fatalf("no matches for @str")
	}
	if matches[0].Name != "strength" {
		t.Errorf("top match = %q, want strength", matches[0].Name)
	}
}

func TestMentionPopupKeyNavigationAndAccept(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	m.panes.bottom.prompt().SetValue("@")
	m.panes.bottom.prompt().CursorEnd()
	m.syncSlashView()

	state := m.mentionState()
	if state == nil {
		t.Fatalf("mentionState() is nil")
	}

	// Initial selection is 0 (strength)
	if state.picker.Index() != 0 {
		t.Errorf("initial index = %d, want 0", state.picker.Index())
	}

	// Press Down Arrow to navigate to agility (index 1)
	res := state.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: tea.KeyDown})
	if !res.handled {
		t.Errorf("expected KeyDown to be handled")
	}
	if state.picker.Index() != 1 {
		t.Errorf("after KeyDown index = %d, want 1", state.picker.Index())
	}

	// Press Enter to accept
	resEnter := state.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: tea.KeyEnter})
	if !resEnter.handled || resEnter.action.kind != paneActionAcceptMention {
		t.Fatalf("expected KeyEnter to yield paneActionAcceptMention, got: %#v", resEnter)
	}

	cmd := m.applyPaneAction(resEnter.action)
	_ = cmd

	// Mention popup should be closed
	if m.mentionOpen() {
		t.Errorf("mentionOpen() = true after accept, want false")
	}

	// Composer value should be "@agility "
	gotVal := m.panes.bottom.prompt().Value()
	if gotVal != "@agility " {
		t.Errorf("prompt.Value() = %q, want '@agility '", gotVal)
	}
}

func TestMentionDirectoryDrillDown(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "components")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(subDir, "button.go"), []byte("package components"), 0o600)

	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = tempDir
	m.resize(80, 24)

	m.panes.bottom.prompt().SetValue("@comp")
	m.panes.bottom.prompt().CursorEnd()
	m.syncSlashView()

	state := m.mentionState()
	if state == nil {
		t.Fatalf("mentionState() is nil")
	}

	// Find the components dir match index
	matches := m.mentionMatches()
	compIdx := -1
	for i, match := range matches {
		if match.Name == "components" {
			compIdx = i
			break
		}
	}
	if compIdx < 0 {
		t.Fatalf("components directory not found in matches: %v", matches)
	}
	state.picker.Select(compIdx)

	// Accept directory
	res := state.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: tea.KeyEnter})
	m.applyPaneAction(res.action)

	// Value should be "@components/" with trailing slash
	gotVal := m.panes.bottom.prompt().Value()
	if gotVal != "@components/" {
		t.Errorf("prompt.Value() = %q, want '@components/'", gotVal)
	}
}

func TestMentionSubmitWithValidFile(t *testing.T) {
	tempDir := t.TempDir()
	sampleFile := filepath.Join(tempDir, "app.go")
	if err := os.WriteFile(sampleFile, []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = tempDir
	m.busy = true
	m.resize(80, 24)

	m.panes.bottom.prompt().SetValue("please examine @app.go")
	m.panes.bottom.prompt().CursorEnd()

	// Submit prompt
	cmd := m.submit()
	_ = cmd

	// Check queued inputs
	queued := m.conversation.QueuedInputs()
	if len(queued) == 0 {
		t.Fatalf("no queued inputs recorded")
	}

	input := queued[len(queued)-1]
	if !strings.Contains(input.Text, "<file path=\"app.go\">") {
		t.Errorf("turn prompt missing file block:\n%s", input.Text)
	}
	if !strings.Contains(input.DisplayText, "@app.go (3 lines)") {
		t.Errorf("display text = %q, want '@app.go (3 lines)'", input.DisplayText)
	}
}

func TestMentionSubmitWithNonexistentFileFailsFast(t *testing.T) {
	tempDir := t.TempDir()

	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = tempDir
	m.resize(80, 24)

	m.panes.bottom.prompt().SetValue("look at @missing_file.go please")
	m.panes.bottom.prompt().CursorEnd()

	// Submit prompt
	cmd := m.submit()
	if cmd != nil {
		t.Errorf("expected submit to return nil on nonexistent file error, got cmd")
	}

	// Composer value should be PRESERVED
	gotPrompt := m.panes.bottom.prompt().Value()
	if gotPrompt != "look at @missing_file.go please" {
		t.Errorf("prompt value = %q, want preserved", gotPrompt)
	}

	// No message should be appended to conversation
	if len(m.conversation.SnapshotMessages()) > 0 {
		t.Errorf("unexpected message appended to conversation on failed submission")
	}
}

func TestMentionEscapeDismisses(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	m.panes.bottom.prompt().SetValue("check @")
	m.panes.bottom.prompt().CursorEnd()
	m.syncSlashView()

	if !m.mentionOpen() {
		t.Fatalf("mentionOpen() = false, want true")
	}

	state := m.mentionState()
	res := state.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: tea.KeyEsc})
	if !res.handled || res.action.kind != paneActionClose {
		t.Fatalf("expected KeyEsc to yield paneActionClose, got: %#v", res)
	}

	m.applyPaneAction(res.action)

	if m.mentionOpen() {
		t.Errorf("mentionOpen() = true after Esc, want false")
	}

	if val := m.panes.bottom.prompt().Value(); val != "check @" {
		t.Errorf("prompt.Value() = %q, want 'check @'", val)
	}
}
