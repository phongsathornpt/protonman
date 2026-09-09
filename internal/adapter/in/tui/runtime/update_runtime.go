package runtime

import (
	"context"
	"log/slog"
	"time"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
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
	case turnDeltaMsg:
		return m.updateTurnDelta(message)
	case turnEventsClosedMsg:
		return m.updateTurnEventsClosed(message)
	case turnDoneMsg:
		return m.updateTurnDone(message)
	}
	return m, nil
}

func (m *bubbleModel) matchesGlobalShortcut(message tea.KeyPressMsg) bool {
	return key.Matches(message, m.keys.Clear) || key.Matches(message, m.keys.ToggleTodo) || key.Matches(message, m.keys.Transcript) || key.Matches(message, m.keys.CycleMode) || key.Matches(message, m.keys.ToggleSkills) || key.Matches(message, m.keys.ToggleModel)
}

func (m *bubbleModel) handleInterruptKey() (tea.Model, tea.Cmd) {
	if m.showTranscript {
		m.closeTranscriptOverlay()
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
		m.resetPrompt()
		m.setBashMode(false)
		m.syncSlashView()
		m.relayout()
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

func (m *bubbleModel) handleGlobalKey(message tea.KeyPressMsg) (bool, tea.Cmd) {
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
		m.toggleTodoPane()
		return true, nil
	case key.Matches(message, m.keys.PageUp):
		m.hydrateViewportForScroll()
		before := m.viewport.YOffset()
		m.viewport.PageUp()
		if m.viewport.YOffset() != before {
			m.markViewportViewDirty()
		}
		m.followTail = m.viewport.AtBottom()
		return true, nil
	case key.Matches(message, m.keys.PageDown):
		m.hydrateViewportForScroll()
		before := m.viewport.YOffset()
		m.viewport.PageDown()
		if m.viewport.YOffset() != before {
			m.markViewportViewDirty()
		}
		m.followTail = m.viewport.AtBottom()
		return true, nil
	default:
		return false, nil
	}
}

func (m *bubbleModel) handlePromptKey(message tea.KeyPressMsg) tea.Cmd {
	if message.String() == "tab" && m.busy {
		return m.withSpinner(m.submit())
	}
	prompt := m.bottom.prompt()
	if message.String() == "esc" {
		if m.bottom.bashMode() {
			m.setBashMode(false)
		}
		m.resetPrompt()
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

func (m *bubbleModel) updateAgentLifecycle(message agentLifecycleMsg) (tea.Model, tea.Cmd) {
	if m.agentActivity == nil {
		m.agentActivity = make(map[string]AgentActivity)
	}
	if message.event.Kind == agent.EventAgentProgress {
		activity := agentActivityFromEvent(message.event)
		if activity.String() != "" {
			m.agentActivity[message.event.AgentID] = activity
			if run := m.ensureHistoryState().AgentRun(message.event.AgentID); run != nil {
				run.Activity = activity.String()
				m.ensureHistoryState().TouchAgentRun(message.event.AgentID)
			}
			m.relayout()
		}
		return m, m.nextAgentEvent()
	}
	if message.event.Kind == agent.EventAgentCompleted || message.event.Kind == agent.EventAgentFailed {
		delete(m.agentActivity, message.event.AgentID)
	}
	m.syncAgentSnapshot()
	m.syncAgentRunSnapshot(message.event.AgentID)
	m.relayout()
	return m, m.nextAgentEvent()
}

func (m *bubbleModel) updatePermissionRequest(message permissionRequestMsg) (tea.Model, tea.Cmd) {
	if !m.busy {
		message.request.response <- permissionResponse{resolution: permission.Resolution{Action: permission.ActionDeny, Reason: "turn is no longer active"}, err: context.Canceled}
		return m, m.bridge.Next()
	}
	m.openPermission(message.request)
	m.relayout()
	return m, m.bridge.Next()
}

func (m *bubbleModel) updateToolResult(message toolResultMsg) (tea.Model, tea.Cmd) {
	slog.DebugContext(m.ctx, "tui direct tool completed", "call_id", message.call.ID, "tool_name", message.call.Name, "success", message.err == nil, "error_type", errorType(message.err))
	m.busy = false
	m.busyStarted = time.Time{}
	m.turnProgress = turnProgress{}
	m.activity = "ready"
	m.turnCancel = nil
	m.appendToolResult(message.result, message.err)
	m.syncTodoSnapshot()
	if message.call.ID != "" {
		m.appendModelToolResult(message.call, message.result)
	}
	m.relayout()
	return m, m.withSpinner(m.drainQueue())
}
