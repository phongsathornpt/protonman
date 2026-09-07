package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/core/permission"
)

func TestShortcutMatrixGlobalKeysSurviveModalRouting(t *testing.T) {
	t.Run("permission lets transcript shortcut bubble", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.bottom.push(&permissionPaneView{})

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
		m = updated.(*bubbleModel)
		if !m.showTranscript {
			t.Fatal("ctrl+t did not open transcript above permission pane")
		}
		if !m.bottom.has(permissionViewID) {
			t.Fatal("transcript shortcut removed pending permission pane")
		}
	})

	t.Run("provider lets transcript shortcut bubble", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.bottom.push(newProviderPaneView())

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
		m = updated.(*bubbleModel)
		if !m.showTranscript {
			t.Fatal("ctrl+t did not open transcript above provider pane")
		}
		if !m.bottom.has(providerViewID) {
			t.Fatal("transcript shortcut unexpectedly closed provider pane")
		}
	})

	t.Run("provider keeps shift-tab local", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		view := newProviderPaneView()
		view.focusIndex = 1
		view.syncInputFocus()
		m.bottom.push(view)

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		m = updated.(*bubbleModel)
		if view.focusIndex != 0 {
			t.Fatalf("shift+tab focus = %d, want previous provider field", view.focusIndex)
		}
		if m.planMode {
			t.Fatal("provider-local shift+tab leaked into global mode cycling")
		}
	})
}

func TestShortcutMatrixInterruptAndToggleSemantics(t *testing.T) {
	t.Run("ctrl-c closes modal before quitting", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.bottom.push(&skillsPaneView{})

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		m = updated.(*bubbleModel)
		if cmd != nil {
			t.Fatal("ctrl+c on modal returned quit command")
		}
		if m.bottom.has(skillsViewID) {
			t.Fatal("ctrl+c did not close skills pane")
		}
	})

	t.Run("ctrl-p closes model picker through binding", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.bottom.push(&modelSelectPaneView{})

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
		m = updated.(*bubbleModel)
		if m.bottom.has(modelSelectViewID) {
			t.Fatal("ctrl+p did not close model picker")
		}
	})

	t.Run("ctrl-c closes transcript overlay", func(t *testing.T) {
		m := newTestBubbleModel(t, permission.ModeAsk, nil)
		m.showTranscript = true

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		m = updated.(*bubbleModel)
		if cmd != nil {
			t.Fatal("ctrl+c on transcript returned quit command")
		}
		if m.showTranscript {
			t.Fatal("ctrl+c did not close transcript overlay")
		}
	})
}
