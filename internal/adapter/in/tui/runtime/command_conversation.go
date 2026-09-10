package runtime

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/app"

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
		if err := m.setActiveGoal(""); err != nil {
			m.appendError("failed to clear goal: " + err.Error())
			return
		}
		m.appendMuted("goal cleared")
	default:
		if err := m.setActiveGoal(goal); err != nil {
			m.appendError("failed to set goal: " + err.Error())
			return
		}
		m.appendMuted("goal · " + goal)
	}
}

func (m *bubbleModel) setActiveGoal(goal string) error {
	goal = strings.TrimSpace(goal)
	if m.runner != nil {
		runner, err := app.CloneConversationWithGoal(m.runner, goal)
		if err != nil {
			return err
		}
		m.runner = runner
	}
	m.activeGoal = goal
	return nil
}

func (m *bubbleModel) clearConversation() {
	m.conversationModelState.resetConversationData()
	m.ensureHistoryState().Reset()
	m.showWelcome = true
	m.panes.showTranscript = false
	m.refreshTranscriptViewport(true)
	m.refreshViewport()
	m.appendMuted("conversation cleared")
}
