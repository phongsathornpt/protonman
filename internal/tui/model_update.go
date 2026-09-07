package tui

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.syncLegacyToComponents()
	defer m.syncComponentsToLegacy()

	switch message := msg.(type) {
	case agentLifecycleMsg:
		return m.updateAgentLifecycle(message)
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return m, nil
	case tea.KeyMsg:
		if key.Matches(message, m.keys.Quit) {
			return m.handleInterruptKey()
		}
		if m.showTranscript {
			return m.updateTranscriptKey(message)
		}
		return m.updateKey(message)
	case tea.MouseMsg:
		var command tea.Cmd
		if m.showTranscript {
			m.transcriptViewport, command = m.transcriptViewport.Update(message)
			return m, command
		}
		if m.bottom.has(skillsViewID) {
			if view, ok := m.bottom.find(skillsViewID).(*skillsPaneView); ok {
				switch message.Button {
				case tea.MouseButtonWheelUp:
					view.HandleKey(m, tea.KeyMsg{Type: tea.KeyUp})
					m.relayout()
					return m, nil
				case tea.MouseButtonWheelDown:
					view.HandleKey(m, tea.KeyMsg{Type: tea.KeyDown})
					m.relayout()
					return m, nil
				}
			}
		}
		if m.viewportTailOnly && message.Button == tea.MouseButtonWheelUp {
			m.hydrateViewportForScroll()
		}
		m.viewport, command = m.viewport.Update(message)
		m.followTail = m.viewport.AtBottom()
		return m, command
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		if !m.busy {
			return m, nil
		}
		if m.historyState.SetSpinnerFrame(m.spinner.View()) {
			m.refreshViewport()
		}
		return m, command
	case cursor.BlinkMsg:
		prompt := m.bottom.prompt()
		updated, command := prompt.Update(message)
		*prompt = updated
		return m, command

	case permissionRequestMsg:
		return m.updatePermissionRequest(message)
	case permissionBridgeClosedMsg:
		return m, nil

	case toolResultMsg:
		return m.updateToolResult(message)

	case modelsFetchedMsg:
		return m.updateModelsFetched(message)

	case providerSavedMsg:
		return m.updateProviderSaved(message)

	case modelSelectedMsg:
		return m.updateModelSelected(message)

	case providerActiveSelectedMsg:
		return m.updateProviderActiveSelected(message)

	case providerDeletedMsg:
		return m.updateProviderDeleted(message)

	case projectInitializedMsg:
		return m.updateProjectInitialized(message)

	case projectLoadedMsg:
		return m.updateProjectLoaded(message)

	case turnDeltaMsg:
		return m.updateTurnDelta(message)

	case turnEventsClosedMsg:
		return m.updateTurnEventsClosed(message)

	case turnDoneMsg:
		return m.updateTurnDone(message)
	}
	return m, nil
}

func (m *bubbleModel) matchesGlobalShortcut(message tea.KeyMsg) bool {
	return key.Matches(message, m.keys.Clear) ||
		key.Matches(message, m.keys.ToggleTodo) ||
		key.Matches(message, m.keys.Transcript) ||
		key.Matches(message, m.keys.CycleMode) ||
		key.Matches(message, m.keys.ToggleSkills) ||
		key.Matches(message, m.keys.ToggleModel)
}

func (m *bubbleModel) handleInterruptKey() (tea.Model, tea.Cmd) {
	if m.showTranscript {
		m.showTranscript = false
		m.relayout()
		return m, nil
	}
	if top := m.bottom.top(); top != nil && top.ID() != permissionViewID && top.ID() != slashViewID {
		if provider, ok := top.(*providerPaneView); ok {
			provider.cancelFetch()
		}
		m.bottom.remove(top.ID())
		m.relayout()
		return m, nil
	}
	if m.busy && m.turnCancel != nil {
		m.cancelActiveTurn()
		m.queue = nil
		return m, nil
	}
	prompt := m.bottom.prompt()
	if prompt.Value() != "" || m.bottom.bashMode() {
		prompt.Reset()
		m.setBashMode(false)
		m.syncSlashView()
		m.relayout()
		return m, nil
	}
	return m, tea.Quit
}

func (m *bubbleModel) updateKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handled, command := m.handleModalKey(message); handled {
		return m, m.withSpinner(command)
	}
	if handled, command := m.handleGlobalKey(message); handled {
		return m, m.withSpinner(command)
	}
	return m, m.handlePromptKey(message)
}

func (m *bubbleModel) handleModalKey(message tea.KeyMsg) (bool, tea.Cmd) {
	top := m.bottom.top()
	if top == nil {
		return false, nil
	}
	handled, command := top.HandleKey(m, message)
	if handled {
		m.relayout()
	}
	return handled, command
}

func (m *bubbleModel) handleGlobalKey(message tea.KeyMsg) (bool, tea.Cmd) {
	switch {
	case key.Matches(message, m.keys.CycleMode):
		m.cycleMode()
		return true, nil
	case key.Matches(message, m.keys.Transcript):
		m.showTranscript = true
		m.refreshTranscriptViewport(true)
		return true, nil
	case key.Matches(message, m.keys.ToggleSkills):
		if m.bottom.has(skillsViewID) {
			m.bottom.remove(skillsViewID)
			m.relayout()
			return true, nil
		}
		if m.skills != nil && len(m.skills.List()) > 0 {
			m.bottom.push(&skillsPaneView{})
			m.relayout()
			return true, nil
		}
		m.executeCommand("/skills")
		return true, nil
	case key.Matches(message, m.keys.ToggleModel):
		if m.bottom.has(modelSelectViewID) {
			m.bottom.remove(modelSelectViewID)
			m.relayout()
			return true, nil
		}
		return true, m.openModelSelectPane()
	case key.Matches(message, m.keys.Clear):
		m.resetTranscript()
		m.refreshViewport()
		return true, nil
	case key.Matches(message, m.keys.ToggleTodo):
		m.todoExpanded = !m.todoExpanded
		m.relayout()
		return true, nil
	case key.Matches(message, m.keys.PageUp):
		m.hydrateViewportForScroll()
		m.viewport.PageUp()
		m.followTail = m.viewport.AtBottom()
		return true, nil
	case key.Matches(message, m.keys.PageDown):
		m.viewport.PageDown()
		m.followTail = m.viewport.AtBottom()
		return true, nil
	default:
		return false, nil
	}
}

func (m *bubbleModel) handlePromptKey(message tea.KeyMsg) tea.Cmd {
	if message.String() == "tab" && m.busy {
		return m.withSpinner(m.submit())
	}
	prompt := m.bottom.prompt()
	if message.String() == "esc" {
		if m.bottom.bashMode() {
			m.setBashMode(false)
		}
		prompt.Reset()
		m.syncSlashView()
		m.relayout()
		return nil
	}
	if message.String() == "enter" {
		return m.withSpinner(m.submit())
	}
	if !m.bottom.bashMode() && prompt.Value() == "" && message.String() == "!" {
		m.setBashMode(true)
		return nil
	}
	if m.bottom.bashMode() && prompt.Value() == "" {
		switch message.Type {
		case tea.KeyBackspace, tea.KeyCtrlH, tea.KeyDelete:
			m.setBashMode(false)
			return nil
		}
	}
	if message.String() == "up" {
		// Keep the textarea's vertical navigation intact for multiline prompts.
		// History recall is only unambiguous on a single-line draft or at the
		// very top-left of a multiline draft.
		lineInfo := prompt.LineInfo()
		if prompt.LineCount() == 1 || (prompt.Line() == 0 && lineInfo.RowOffset == 0 && lineInfo.ColumnOffset == 0) {
			m.historyPrevious()
			m.syncSlashView()
			return nil
		}
	}
	if message.String() == "down" && m.bottom.historyNavigating() {
		m.historyNext()
		m.syncSlashView()
		return nil
	}

	updated, command := prompt.Update(message)
	*prompt = updated
	m.syncSlashView()
	m.relayout()
	return command
}
