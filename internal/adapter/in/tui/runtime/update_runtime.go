package runtime

import (
	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
)

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer m.reconcileLayout()
	if command, handled := m.updateTerminalEvent(msg); handled {
		return m, command
	}
	if command, handled := m.updateAnimationEvent(msg); handled {
		return m, command
	}
	if command, handled := m.updateRuntimeEvent(msg); handled {
		return m, command
	}
	return m, nil
}

func (m *bubbleModel) updateTerminalEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return nil, true
	case tea.PasteMsg:
		if m.panes.showTranscript || m.panes.bottom == nil || !m.panes.bottom.composerVisible() {
			return nil, true
		}
		prompt := m.panes.bottom.prompt()
		if prompt == nil {
			return nil, true
		}
		updated, command := prompt.Update(message)
		*prompt = updated
		m.syncSlashView()
		m.requestRelayout()
		return command, true
	case tea.KeyPressMsg:
		if key.Matches(message, m.keys.Quit) {
			return m.handleInterruptKey(), true
		}
		if m.panes.showTranscript {
			return m.updateTranscriptKey(message), true
		}
		return m.updateKey(message), true
	case tea.MouseMsg:
		return m.updateMouseEvent(message), true
	default:
		return nil, false
	}
}

func (m *bubbleModel) updateMouseEvent(message tea.MouseMsg) tea.Cmd {
	mouse := message.Mouse()
	if m.panes.showTranscript {
		var command tea.Cmd
		m.panes.transcript, command = m.panes.transcript.Update(message)
		return command
	}
	if m.panes.bottom.has(skillsViewID) {
		if view, ok := m.panes.bottom.find(skillsViewID).(*skillsPaneView); ok {
			switch mouse.Button {
			case tea.MouseWheelUp:
				result := view.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: tea.KeyUp})
				m.requestRelayout()
				return result.cmd
			case tea.MouseWheelDown:
				result := view.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: tea.KeyDown})
				m.requestRelayout()
				return result.cmd
			}
		}
	}
	if mouse.Y < 0 || mouse.Y >= m.viewport.Height() {
		return nil
	}
	return m.updateConversationViewport(message)
}

func (m *bubbleModel) updateAnimationEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch message := msg.(type) {
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		if !m.busy {
			return nil, true
		}
		return command, true
	case cursor.BlinkMsg:
		prompt := m.panes.bottom.prompt()
		updated, command := prompt.Update(message)
		*prompt = updated
		return command, true
	default:
		return nil, false
	}
}

func (m *bubbleModel) updateRuntimeEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch message := msg.(type) {
	case agentLifecycleMsg:
		return m.updateAgentLifecycle(message), true
	case permissionRequestMsg:
		return m.updatePermissionRequest(message), true
	case permissionBridgeClosedMsg:
		return nil, true
	case toolResultMsg:
		return m.updateToolResult(message), true
	case todoReloadedMsg:
		return m.updateTodoReloaded(message), true
	case modelsFetchedMsg:
		return m.updateModelsFetched(message), true
	case providerSavedMsg:
		return m.updateProviderSaved(message), true
	case modelSetupAppliedMsg:
		return m.updateModelSetupApplied(message), true
	case providerActiveSelectedMsg:
		return m.updateProviderActiveSelected(message), true
	case providerDeletedMsg:
		return m.updateProviderDeleted(message), true
	case permissionRuleSavedMsg:
		return m.updatePermissionRuleSaved(message), true
	case turnmsg.Delta:
		return m.updateTurnDelta(message), true
	case turnmsg.EventsClosed:
		return m.updateTurnEventsClosed(message), true
	case turnmsg.Done:
		return m.updateTurnDone(message), true
	default:
		return nil, false
	}
}
