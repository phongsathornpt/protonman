package runtime

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m *bubbleModel) matchesGlobalShortcut(message tea.KeyPressMsg) bool {
	return key.Matches(message, m.keys.Clear) || key.Matches(message, m.keys.ToggleTodo) || key.Matches(message, m.keys.Transcript) || key.Matches(message, m.keys.CycleMode) || key.Matches(message, m.keys.ToggleSkills) || key.Matches(message, m.keys.ToggleModel)
}

func (m *bubbleModel) handleInterruptKey() (tea.Model, tea.Cmd) {
	if m.panes.showTranscript {
		m.closeTranscriptOverlay()
		m.requestRelayout()
		return m, nil
	}
	if top := m.panes.bottom.top(); top != nil && top.ID() != permissionViewID && top.ID() != slashViewID {
		if provider, ok := top.(*providerPaneView); ok {
			provider.cancelFetch()
		}
		m.panes.bottom.remove(top.ID())
		m.requestRelayout()
		return m, nil
	}
	if m.busy && m.turnCancel != nil {
		m.cancelActiveTurn()
		m.queue = nil
		return m, nil
	}
	prompt := m.panes.bottom.prompt()
	if prompt.Value() != "" || m.panes.bottom.bashMode() {
		m.resetPrompt()
		m.setBashMode(false)
		m.syncSlashView()
		m.requestRelayout()
		return m, nil
	}
	return m, tea.Quit
}

func (m *bubbleModel) updateKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if handled, command := m.handleModalKey(message); handled {
		return m, m.withSpinner(command)
	}
	if handled, command := m.handleGlobalKey(message); handled {
		return m, m.withSpinner(command)
	}
	return m, m.handlePromptKey(message)
}

func (m *bubbleModel) handleModalKey(message tea.KeyPressMsg) (bool, tea.Cmd) {
	top := m.panes.bottom.top()
	if top == nil {
		return false, nil
	}
	handled, command := top.HandleKey(m, message)
	if handled {
		m.requestRelayout()
	}
	return handled, command
}

func (m *bubbleModel) handleGlobalKey(message tea.KeyPressMsg) (bool, tea.Cmd) {
	switch {
	case key.Matches(message, m.keys.CycleMode):
		m.cycleMode()
		return true, nil
	case key.Matches(message, m.keys.Transcript):
		m.panes.showTranscript = true
		m.refreshTranscriptViewport(true)
		return true, nil
	case key.Matches(message, m.keys.ToggleSkills):
		if m.panes.bottom.has(skillsViewID) {
			m.panes.bottom.remove(skillsViewID)
			m.requestRelayout()
			return true, nil
		}
		if m.skills != nil && len(m.skills.List()) > 0 {
			m.panes.bottom.push(&skillsPaneView{})
			m.requestRelayout()
			return true, nil
		}
		m.executeCommand("/skills")
		return true, nil
	case key.Matches(message, m.keys.ToggleModel):
		if m.panes.bottom.has(modelSelectViewID) {
			m.panes.bottom.remove(modelSelectViewID)
			m.requestRelayout()
			return true, nil
		}
		return true, m.openModelSelectPane()
	case key.Matches(message, m.keys.Clear):
		m.resetTranscript()
		m.refreshViewport()
		return true, nil
	case key.Matches(message, m.keys.ToggleTodo):
		m.toggleTodoPane()
		return true, nil
	case key.Matches(message, m.keys.PageUp):
		return true, m.updateConversationViewport(message)
	case key.Matches(message, m.keys.PageDown):
		return true, m.updateConversationViewport(message)
	default:
		return false, nil
	}
}

func (m *bubbleModel) handlePromptKey(message tea.KeyPressMsg) tea.Cmd {
	if message.String() == "tab" && m.busy {
		return m.withSpinner(m.submit())
	}
	prompt := m.panes.bottom.prompt()
	if message.String() == "esc" {
		if m.panes.bottom.bashMode() {
			m.setBashMode(false)
		}
		m.resetPrompt()
		m.syncSlashView()
		m.requestRelayout()
		return nil
	}
	if message.String() == "enter" {
		return m.withSpinner(m.submit())
	}
	if !m.panes.bottom.bashMode() && prompt.Value() == "" && message.String() == "!" {
		m.setBashMode(true)
		return nil
	}
	if m.panes.bottom.bashMode() && prompt.Value() == "" {
		switch message.String() {
		case "backspace", "ctrl+h", "delete":
			m.setBashMode(false)
			return nil
		}
	}
	if message.String() == "up" {
		lineInfo := prompt.LineInfo()
		if prompt.LineCount() == 1 || (prompt.Line() == 0 && lineInfo.RowOffset == 0 && lineInfo.ColumnOffset == 0) {
			m.historyPrevious()
			m.syncSlashView()
			return nil
		}
	}
	if message.String() == "down" && m.panes.bottom.historyNavigating() {
		m.historyNext()
		m.syncSlashView()
		return nil
	}
	updated, command := prompt.Update(message)
	*prompt = updated
	m.syncSlashView()
	m.requestRelayout()
	return command
}
