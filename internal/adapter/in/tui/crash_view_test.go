package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBuildCrashReport(t *testing.T) {
	report := BuildCrashReport("nil pointer dereference", "goroutine 1 [running]:\nmain.go:123")
	if !strings.Contains(report, "Proton Crash Report") {
		t.Fatalf("expected report header, got: %s", report)
	}
	if !strings.Contains(report, "nil pointer dereference") {
		t.Fatalf("expected panic message in report, got: %s", report)
	}
	if !strings.Contains(report, "goroutine 1 [running]") {
		t.Fatalf("expected stack trace in report, got: %s", report)
	}
}

func TestCrashModelNavigation(t *testing.T) {
	stack := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10"
	m := NewCrashModel("test failure", []byte(stack))
	m.width = 80
	m.height = 24

	// Test View rendering
	rendered := m.View()
	if !strings.Contains(rendered, "Proton crashed") {
		t.Fatalf("expected headline in view, got: %s", rendered)
	}
	if !strings.Contains(rendered, "test failure") {
		t.Fatalf("expected error message in view, got: %s", rendered)
	}
	if !strings.Contains(rendered, "[c] Copy report") {
		t.Fatalf("expected copy report action in view, got: %s", rendered)
	}

	// Scroll down
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.scrollOffset != 1 {
		t.Fatalf("expected scrollOffset 1, got %d", m.scrollOffset)
	}

	// Scroll up
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.scrollOffset != 0 {
		t.Fatalf("expected scrollOffset 0, got %d", m.scrollOffset)
	}

	// Copy action
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !m.copied {
		t.Fatal("expected copied flag to be set")
	}

	// Restart action
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !m.restart {
		t.Fatal("expected restart flag to be set")
	}
	if cmd == nil {
		t.Fatal("expected Quit cmd on restart")
	}
}
