package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/core/permission"
)

func TestCommandHistoryIsBounded(t *testing.T) {
	pane := newBottomPane(true)
	for i := 0; i < maxCommandHistory+25; i++ {
		pane.recordHistory(fmt.Sprintf("command-%d", i))
	}
	if got := len(pane.composer.history); got != maxCommandHistory {
		t.Fatalf("history len = %d, want %d", got, maxCommandHistory)
	}
	if got := pane.composer.history[0]; got != "command-25" {
		t.Fatalf("oldest retained history = %q, want command-25", got)
	}
}

func TestQueueFullPreservesDraft(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.busy = true
	for i := 0; i < maxQueuedPrompts; i++ {
		m.queue = append(m.queue, fmt.Sprintf("queued-%d", i))
	}
	m.prompt.SetValue("keep this draft")
	if cmd := m.submit(); cmd != nil {
		t.Fatalf("submit() command = %v, want nil", cmd)
	}
	if got := m.prompt.Value(); got != "keep this draft" {
		t.Fatalf("draft = %q, want preserved input", got)
	}
	if got := len(m.queue); got != maxQueuedPrompts {
		t.Fatalf("queue len = %d, want %d", got, maxQueuedPrompts)
	}
}

func TestQueueEchoTruncatesLongPrompt(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.busy = true
	long := strings.Repeat("x", maxQueuePreviewRunes+200)
	m.prompt.SetValue(long)
	_ = m.submit()
	plain := plainTranscript(m)
	if strings.Contains(plain, long) {
		t.Fatal("queued transcript echoed full long prompt")
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("queued transcript missing truncation marker: %q", plain)
	}
}

func TestRenderProviderInputMissingViewIsSafe(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	if got := renderProviderInput(m); got != "" {
		t.Fatalf("renderProviderInput() = %q, want empty without provider pane", got)
	}
}
