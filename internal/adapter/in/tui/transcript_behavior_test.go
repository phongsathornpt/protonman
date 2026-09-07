package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/adapter/out/model"
	"github.com/projectTHORN/proton/internal/core/permission"
	"strings"
	"testing"
)

func TestTranscriptOverlayIncludesLiveAssistantTail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.appendAssistantDelta("streaming now")
	m.showTranscript = true
	m.refreshTranscriptViewport(true)

	if !strings.Contains(m.transcriptOverlayView(), "streaming now") {
		t.Fatalf("transcript overlay omitted active cell: %s", m.transcriptOverlayView())
	}
	updated, _ := m.updateTranscriptKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(*bubbleModel)
	if !m.rawTranscript {
		t.Fatal("r did not toggle raw transcript mode")
	}
}
func TestInitialMessagesRestoreIntoHistoryAndNextTurn(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	messages := []model.Message{
		{Role: model.RoleUser, Content: "previous question"},
		{Role: model.RoleAssistant, Content: "previous answer"},
	}
	m := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
		messages,
	)

	plain := plainTranscript(m)
	if !strings.Contains(plain, "previous question") || !strings.Contains(plain, "previous answer") {
		t.Fatalf("restored transcript = %q", plain)
	}
	if len(m.messages) != 2 {
		t.Fatalf("provider history length = %d, want 2", len(m.messages))
	}
}
