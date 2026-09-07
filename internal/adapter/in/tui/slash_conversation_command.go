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
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			m.todoViewState.Expanded = !m.todoViewState.Expanded
		case "show":
			m.todoViewState.Expanded = true
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
