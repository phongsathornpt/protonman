package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *bubbleModel) executeConversationCommand(name, argument string) tea.Cmd {
	switch name {
	case "goal":
		m.handleGoalCommand(argument)
	case "clear":
		if strings.TrimSpace(argument) != "" {
			m.appendError("usage: /clear")
			break
		}
		m.clearConversation()
	case "transcript":
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			m.openTranscriptOverlay()
		case "clear":
			m.resetTranscript()
		default:
			m.appendError("usage: /transcript [clear]")
		}
	case "todo":
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			return m.toggleTodoPane()
		case "show":
			return m.openTodoPane()
		case "hide":
			m.panes.bottom.remove(todoInspectViewID)
			m.requestRelayout()
		default:
			m.appendError("usage: /todo [show|hide]")
		}
	}
	return nil
}

func (m *bubbleModel) handleGoalCommand(argument string) {
	goal := strings.TrimSpace(argument)
	switch strings.ToLower(goal) {
	case "":
		if m.activeGoal == "" {
			m.appendMuted("no active goal")
			return
		}
		m.appendMuted("goal · " + m.activeGoal)
	case "clear":
		m.activeGoal = ""
		m.reconfigureRunner()
		m.appendMuted("goal cleared")
	default:
		m.activeGoal = goal
		m.reconfigureRunner()
		m.appendMuted("goal · " + goal)
	}
}

func (m *bubbleModel) clearConversation() {
	m.messages = nil
	m.queue = nil
	m.resetTranscript()
	m.appendMuted("conversation cleared")
	m.refreshViewport()
}
