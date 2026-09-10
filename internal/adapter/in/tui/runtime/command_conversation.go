package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *bubbleModel) executeConversationCommand(name, argument string) tea.Cmd {
	switch name {
	case "transcript":
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			m.panes.showTranscript = true
			m.refreshTranscriptViewport(true)
		case "clear":
			m.resetTranscript()
		default:
			m.appendError("usage: /transcript [clear]")
		}
	case "todo":
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			m.toggleTodoPane()
		case "show":
			if !m.panes.bottom.has(todoInspectViewID) {
				m.panes.bottom.push(&todoPaneView{})
			}
			m.requestRelayout()
		case "hide":
			m.panes.bottom.remove(todoInspectViewID)
			m.requestRelayout()
		default:
			m.appendError("usage: /todo [show|hide]")
		}
	}
	return nil
}
