package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/appdirs"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.syncLegacyToComponents()
	defer m.syncComponentsToLegacy()

	switch message := msg.(type) {
	case agentLifecycleMsg:
		if m.agentActivity == nil {
			m.agentActivity = make(map[string]string)
		}
		if message.event.Kind == agent.EventAgentProgress {
			if activity := strings.TrimSpace(message.event.Message); activity != "" {
				m.agentActivity[message.event.AgentID] = activity
			}
			return m, m.nextAgentEvent()
		}
		if message.event.Kind == agent.EventAgentCompleted || message.event.Kind == agent.EventAgentFailed {
			delete(m.agentActivity, message.event.AgentID)
		}
		m.syncAgentSnapshot()
		m.relayout()
		return m, m.nextAgentEvent()
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
		if !m.busy {
			message.request.response <- permissionResponse{
				resolution: permission.Resolution{Action: permission.ActionDeny, Reason: "turn is no longer active"},
				err:        context.Canceled,
			}
			return m, m.bridge.Next()
		}
		m.openPermission(message.request)
		m.relayout()
		return m, m.bridge.Next()
	case permissionBridgeClosedMsg:
		return m, nil
	case toolResultMsg:
		slog.DebugContext(m.ctx, "tui direct tool completed",
			"call_id", message.call.ID,
			"tool_name", message.call.Name,
			"success", message.err == nil,
			"error_type", errorType(message.err),
		)
		m.busy = false
		m.busyStarted = time.Time{}
		m.turnProgress = turnProgress{}
		m.activity = "ready"
		m.turnCancel = nil
		m.appendToolResult(message.result, message.err)
		m.reloadTodoAfterExternalTool(message.call, message.result, message.err)
		m.syncTodoSnapshot()
		if message.call.ID != "" {
			m.appendModelToolResult(message.call, message.result)
		}
		m.relayout()
		return m, m.withSpinner(m.drainQueue())
	case modelsFetchedMsg:
		if pane := m.bottom.find(providerViewID); pane != nil {
			if pv, ok := pane.(*providerPaneView); ok {
				if pv.fetchRequestID != 0 && message.requestID != pv.fetchRequestID {
					return m, nil
				}
				pv.fetchCancel = nil
				if message.err == nil && len(message.models) > 0 {
					m.modelCatalogs.set(message.providerName, message.models)
				}
				if message.err != nil {
					pv.state = providerStateError
					pv.errorMessage = message.err.Error()
				} else {
					pv.state = providerStateSelectModel
					pv.setFetchedModels(message.models)
				}
				m.relayout()
			}
			return m, nil
		}
		if pane := m.bottom.find(modelSelectViewID); pane != nil {
			if mv, ok := pane.(*modelSelectPaneView); ok {
				currentProvider := mv.activeProviderName()
				if message.requestID != mv.fetchRequestID || !strings.EqualFold(message.providerName, currentProvider) {
					return m, nil
				}
				mv.fetchCancel = nil
				mv.loading = false
				mv.err = message.err
				if message.err == nil {
					m.modelCatalogs.set(message.providerName, message.models)
					mv.setModels(m.modelCatalogs.models(message.providerName), m.activeModel)
				}
				m.relayout()
			}
		}
		return m, nil
	case providerSavedMsg:
		if message.err != nil {
			if pane := m.bottom.find(providerViewID); pane != nil {
				if pv, ok := pane.(*providerPaneView); ok {
					pv.state = providerStateSaveError
					pv.errorMessage = message.err.Error()
					m.relayout()
					return m, nil
				}
			}
			m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to save provider: %v", message.err)))
		} else {
			providerName := strings.TrimSpace(message.providerName)
			providerKey := strings.ToLower(providerName)
			previousKey := strings.ToLower(strings.TrimSpace(message.previousName))
			if previousKey != "" && previousKey != providerKey {
				delete(m.providers, previousKey)
			}
			if m.providers == nil {
				m.providers = make(map[string]config.ProviderConfig)
			}
			m.providers[providerKey] = config.ProviderConfig{
				Name:    providerName,
				Type:    message.providerType,
				BaseURL: message.baseURL,
				APIKey:  message.apiKey,
			}
			if message.activated {
				m.activeModel = message.modelID
				m.activeProvider = providerName
				m.reconfigureRunner()
				m.appendLine(successStyle.Render(fmt.Sprintf("✓ Configured provider %s", providerName)))
			} else {
				m.appendLine(successStyle.Render(fmt.Sprintf("✓ Updated provider %s", providerName)))
				if m.activeProvider != "" {
					m.appendLine(mutedStyle.Render(fmt.Sprintf("  Active provider remains %s", m.activeProvider)))
				}
			}
			m.appendLine(mutedStyle.Render(fmt.Sprintf("  Endpoint: %s", message.baseURL)))
			if message.activated && message.modelID != "" {
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Default Model: %s", message.modelID)))
			}
			m.appendLine(mutedStyle.Render("  Saved to " + appdirs.UserConfigDisplay()))
		}
		m.bottom.remove(providerViewID)
		m.relayout()
		return m, nil
	case modelSelectedMsg:
		if message.err != nil {
			m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to set active model: %v", message.err)))
		} else {
			m.activeModel = message.modelID
			if message.providerName != "" {
				m.activeProvider = message.providerName
			}
			m.reconfigureRunner()
			m.appendLine(successStyle.Render(fmt.Sprintf("✓ Active model set to %s (%s)", message.modelID, m.activeProvider)))
			if message.unverified {
				m.appendLine(mutedStyle.Render("  Model ID was not present in the discovered catalog; using it as a custom model."))
			}
			m.appendLine(mutedStyle.Render("  Saved to " + appdirs.UserConfigDisplay()))
		}
		m.bottom.remove(modelSelectViewID)
		m.relayout()
		return m, nil
	case providerActiveSelectedMsg:
		if message.err != nil {
			m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to switch provider: %v", message.err)))
		} else {
			m.activeProvider = message.providerName
			m.reconfigureRunner()
			m.appendLine(successStyle.Render(fmt.Sprintf("✓ Switched active provider to %s", message.providerName)))
			if p, ok := m.providers[strings.ToLower(message.providerName)]; ok && p.BaseURL != "" {
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Endpoint: %s", p.BaseURL)))
			}
			if m.activeModel != "" {
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Active model: %s", m.activeModel)))
			} else {
				m.appendLine(mutedStyle.Render("  Use /model to choose a model for this provider"))
			}
			m.appendLine(mutedStyle.Render("  Saved to " + appdirs.UserConfigDisplay()))
		}
		m.bottom.remove(providerSelectViewID)
		m.relayout()
		return m, nil
	case providerDeletedMsg:
		if message.err != nil {
			m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to remove provider %s: %v", message.providerName, message.err)))
		} else {
			delete(m.providers, strings.ToLower(message.providerName))
			if strings.EqualFold(m.activeProvider, message.providerName) {
				m.activeProvider = ""
				for remaining := range m.providers {
					m.activeProvider = remaining
					break
				}
				m.reconfigureRunner()
			}
			m.appendLine(successStyle.Render(fmt.Sprintf("✓ Removed provider %s", message.providerName)))
			if m.activeProvider != "" {
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Active provider is now %s", m.activeProvider)))
			}
			m.appendLine(mutedStyle.Render("  Updated " + appdirs.UserConfigDisplay()))
		}
		m.bottom.remove(providerSelectViewID)
		m.relayout()
		return m, nil
	case turnDeltaMsg:
		batch := []applicationturn.Event{message.event}
		for {
			select {
			case next, ok := <-m.turnEvents:
				if !ok {
					m.applyTurnEvents(batch)
					slog.DebugContext(m.ctx, "tui turn event channel closed before terminal message",
						"busy", m.busy,
					)
					if m.busy && m.ctx.Err() == nil {
						return m.Update(turnEventsClosedMsg{})
					}
					m.refreshViewport()
					return m, nil
				}
				if delta, isDelta := next.(turnDeltaMsg); isDelta {
					batch = append(batch, delta.event)
					continue
				}
				m.applyTurnEvents(batch)
				m.refreshViewport()
				return m.Update(next)
			default:
			}
			break
		}
		m.applyTurnEvents(batch)
		m.refreshViewport()
		return m, m.withSpinner(waitTurnCh(m.turnEvents))
	case turnEventsClosedMsg:
		slog.DebugContext(m.ctx, "tui turn event channel closed unexpectedly",
			"busy", m.busy,
			"context_error", m.ctx.Err() != nil,
		)
		if !m.busy || m.ctx.Err() != nil {
			return m, nil
		}
		return m.Update(turnDoneMsg{err: errTurnEventsClosed})
	case turnDoneMsg:
		slog.DebugContext(m.ctx, "tui turn terminal message received",
			"success", message.err == nil,
			"error_type", errorType(message.err),
			"rounds", message.result.Rounds,
			"message_count", len(message.result.Messages),
			"assistant_bytes", len(message.result.Message.Content),
		)
		m.busy = false
		m.busyStarted = time.Time{}
		m.activity = "ready"
		m.turnCancel = nil
		m.turnEvents = nil
		m.activeTurnOwner = ""
		if message.err != nil {
			m.finalizeRunningTools(message.err)
		}
		m.historyState.CommitActive()
		m.syncLegacyBlocks()
		if message.err == nil {
			if len(message.result.Messages) > 0 {
				m.messages = append(m.messages, model.CloneMessages(message.result.Messages)...)
			} else if message.result.Message.Content != "" {
				// Keep compatibility with older/injected runners that only populate
				// Result.Message.
				m.messages = append(m.messages, message.result.Message)
			}
		} else if message.err != nil && len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == model.RoleUser {
			m.messages = m.messages[:len(m.messages)-1]
		}
		m.appendTurnFailure(message.err)
		m.relayout()
		if message.err != nil {
			m.queue = nil
			return m, nil
		}
		return m, m.withSpinner(m.drainQueue())
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
