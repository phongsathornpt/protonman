package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *bubbleModel) executeConversationCommand(name, argument string) tea.Cmd {
	switch name {
	case "transcript":
		m.showTranscript = true
		m.refreshTranscriptViewport(true)
	case "todo":
		if layoutModeForHeight(m.height) != layoutNormal {
			verb := strings.ToLower(strings.TrimSpace(argument))
			if verb == "hide" {
				m.bottom.remove(todoInspectViewID)
				m.relayout()
				return nil
			}
			if verb == "" || verb == "show" {
				if !m.bottom.has(todoInspectViewID) {
					m.bottom.push(&todoPaneView{})
				}
				m.relayout()
				return nil
			}
		}
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			m.todoViewState.Expanded = !m.todoViewState.Expanded
			if m.todoViewState.Expanded {
				m.revealRetiredTodo()
			}
		case "show":
			m.todoViewState.Expanded = true
			m.revealRetiredTodo()
		case "hide":
			m.todoViewState.Expanded = false
		default:
			m.appendError("usage: /todo [show|hide]")
		}
		m.resize(m.width, m.height)
	case "clear":
		m.resetTranscript()
		m.refreshViewport()
	case "new":
		m.resetConversation()
		m.refreshViewport()
	}
	return nil
}
