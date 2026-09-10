package runtime

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"strings"
)

func (m *bubbleModel) matchesGlobalShortcut(message tea.KeyPressMsg) bool {
	return key.Matches(message, m.keys.ToggleTodo) || key.Matches(message, m.keys.Transcript) || key.Matches(message, m.keys.CyclePermission) || key.Matches(message, m.keys.ToggleSkills) || key.Matches(message, m.keys.ToggleModel)
}

func (m *bubbleModel) handleInterruptKey() tea.Cmd {
	if m.panes.showTranscript {
		m.closeTranscriptOverlay()
		m.requestRelayout()
		return nil
	}
	if top := m.panes.bottom.top(); top != nil && top.ID() != permissionViewID && top.ID() != slashViewID {
		if provider, ok := top.(*providerPaneView); ok {
			provider.cancelFetch()
		}
		m.panes.bottom.remove(top.ID())
		m.requestRelayout()
		return nil
	}
	if m.busy && m.turnCancel != nil {
		m.cancelActiveTurn()
		m.queue = nil
		return nil
	}
	prompt := m.panes.bottom.prompt()
	if prompt.Value() != "" || m.panes.bottom.bashMode() {
		m.resetPrompt()
		m.setBashMode(false)
		m.syncSlashView()
		m.requestRelayout()
		return nil
	}
	return tea.Quit
}

func (m *bubbleModel) updateKey(message tea.KeyPressMsg) tea.Cmd {
	top := m.panes.bottom.top()
	if top != nil && top.PresentationMode() == paneBlocking {
		if handled, command := m.handlePaneKey(message); handled {
			return m.withSpinner(command)
		}
		return nil
	}
	if m.matchesGlobalShortcut(message) {
		if handled, command := m.handleGlobalKey(message); handled {
			return m.withSpinner(command)
		}
	}
	if handled, command := m.handlePaneKey(message); handled {
		return m.withSpinner(command)
	}
	if handled, command := m.handleGlobalKey(message); handled {
		return m.withSpinner(command)
	}
	return m.handlePromptKey(message)
}

func (m *bubbleModel) handlePaneKey(message tea.KeyPressMsg) (bool, tea.Cmd) {
	top := m.panes.bottom.top()
	if top == nil {
		return false, nil
	}
	if isolated, ok := top.(isolatedPaneKeyHandler); ok {
		result := isolated.HandlePaneKey(newPaneRenderContext(m), message)
		command := result.cmd
		if result.action.kind != paneActionNone {
			command = tea.Batch(command, m.applyPaneAction(result.action))
		}
		if result.handled {
			m.requestRelayout()
		}
		return result.handled, command
	}
	return false, nil
}

func (m *bubbleModel) handleGlobalKey(message tea.KeyPressMsg) (bool, tea.Cmd) {
	switch {
	case key.Matches(message, m.keys.CyclePermission):
		m.cyclePermission()
		return true, nil
	case key.Matches(message, m.keys.Transcript):
		m.openTranscriptOverlay()
		return true, nil
	case key.Matches(message, m.keys.ToggleSkills):
		return true, m.toggleSkillsPane()
	case key.Matches(message, m.keys.ToggleModel):
		return true, m.toggleModelSetupPane()
	case key.Matches(message, m.keys.ToggleTodo):
		return true, m.toggleTodoPane()
	case key.Matches(message, m.keys.PageUp):
		return true, m.updateConversationViewport(message)
	case key.Matches(message, m.keys.PageDown):
		return true, m.updateConversationViewport(message)
	default:
		return false, nil
	}
}

func (m *bubbleModel) handlePromptKey(message tea.KeyPressMsg) tea.Cmd {
	prompt := m.panes.bottom.prompt()
	if message.String() == "?" && prompt.Value() == "" && !m.panes.bottom.bashMode() {
		m.openShortcutsPane()
		return nil
	}
	if message.String() == "tab" && m.busy {
		return m.withSpinner(m.submit())
	}
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
	if strings.TrimSpace(prompt.Value()) == "" && prompt.Value() != "" {
		m.resetPrompt()
	}
	m.syncSlashView()
	m.requestRelayout()
	return command
}
