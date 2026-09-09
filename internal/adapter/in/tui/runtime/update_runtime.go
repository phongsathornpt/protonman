package runtime

import (
	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
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
	case tea.KeyPressMsg:
		if key.Matches(message, m.keys.Quit) {
			return m.handleInterruptKey()
		}
		if m.showTranscript {
			return m.updateTranscriptKey(message)
		}
		return m.updateKey(message)
	case tea.MouseMsg:
		mouse := message.Mouse()
		var command tea.Cmd
		if m.showTranscript {
			m.transcriptViewport, command = m.transcriptViewport.Update(message)
			return m, command
		}
		if m.bottom.has(skillsViewID) {
			if view, ok := m.bottom.find(skillsViewID).(*skillsPaneView); ok {
				switch mouse.Button {
				case tea.MouseWheelUp:
					view.HandleKey(m, tea.KeyPressMsg{Code: tea.KeyUp})
					m.relayout()
					return m, nil
				case tea.MouseWheelDown:
					view.HandleKey(m, tea.KeyPressMsg{Code: tea.KeyDown})
					m.relayout()
					return m, nil
				}
			}
		}
		if mouse.Y < 0 || mouse.Y >= m.viewport.Height() {
			return m, nil
		}
		if (m.viewportTailOnly || m.viewportStaleTail) && (mouse.Button == tea.MouseWheelUp || mouse.Button == tea.MouseWheelDown) {
			m.hydrateViewportForScroll()
		}
		beforeOffset := m.viewport.YOffset()
		m.viewport, command = m.viewport.Update(message)
		if m.viewport.YOffset() != beforeOffset {
			m.markViewportViewDirty()
		}
		m.followTail = m.viewport.AtBottom()
		return m, command
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		if !m.busy {
			return m, nil
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
	case projectSettingSavedMsg:
		return m.updateProjectSettingSaved(message)
	case userSettingSavedMsg:
		return m.updateUserSettingSaved(message)
	case permissionRuleSavedMsg:
		return m.updatePermissionRuleSaved(message)
	case projectLoadedMsg:
		return m.updateProjectLoaded(message)
	case turnmsg.Delta:
		return m.updateTurnDelta(message)
	case turnmsg.EventsClosed:
		return m.updateTurnEventsClosed(message)
	case turnmsg.Done:
		return m.updateTurnDone(message)
	}
	return m, nil
}
