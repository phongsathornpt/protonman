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
	case tea.KeyPressMsg:
		if key.Matches(message, m.keys.Quit) {
			_, command := m.handleInterruptKey()
			return command, true
		}
		if m.panes.showTranscript {
			_, command := m.updateTranscriptKey(message)
			return command, true
		}
		_, command := m.updateKey(message)
		return command, true
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
		_, command := m.updateAgentLifecycle(message)
		return command, true
	case permissionRequestMsg:
		_, command := m.updatePermissionRequest(message)
		return command, true
	case permissionBridgeClosedMsg:
		return nil, true
	case toolResultMsg:
		_, command := m.updateToolResult(message)
		return command, true
	case modelsFetchedMsg:
		_, command := m.updateModelsFetched(message)
		return command, true
	case providerSavedMsg:
		_, command := m.updateProviderSaved(message)
		return command, true
	case modelSelectedMsg:
		_, command := m.updateModelSelected(message)
		return command, true
	case providerActiveSelectedMsg:
		_, command := m.updateProviderActiveSelected(message)
		return command, true
	case providerDeletedMsg:
		_, command := m.updateProviderDeleted(message)
		return command, true
	case projectInitializedMsg:
		_, command := m.updateProjectInitialized(message)
		return command, true
	case projectSettingSavedMsg:
		_, command := m.updateProjectSettingSaved(message)
		return command, true
	case userSettingSavedMsg:
		_, command := m.updateUserSettingSaved(message)
		return command, true
	case permissionRuleSavedMsg:
		_, command := m.updatePermissionRuleSaved(message)
		return command, true
	case projectLoadedMsg:
		_, command := m.updateProjectLoaded(message)
		return command, true
	case turnmsg.Delta:
		_, command := m.updateTurnDelta(message)
		return command, true
	case turnmsg.EventsClosed:
		_, command := m.updateTurnEventsClosed(message)
		return command, true
	case turnmsg.Done:
		_, command := m.updateTurnDone(message)
		return command, true
	default:
		return nil, false
	}
}
