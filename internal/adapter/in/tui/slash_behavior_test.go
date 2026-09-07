package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/core/permission"
	"testing"
)

func TestSlashEscapePreservesComposerDraft(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.bottom.prompt().SetValue("/he")
	m.syncSlashView()
	if !m.slashOpen() {
		t.Fatal("slash view did not open")
	}

	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*bubbleModel)
	if command != nil {
		t.Fatalf("escape command = %v, want nil", command)
	}
	if got := m.bottom.prompt().Value(); got != "/he" {
		t.Fatalf("draft = %q, want /he", got)
	}
	if m.bottom.has(slashViewID) {
		t.Fatal("slash view remained on stack after escape")
	}
}
