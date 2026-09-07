package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/app"
	"github.com/projectTHORN/proton/internal/app/appdirs"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
)

func (m *bubbleModel) updateAgentLifecycle(message agentLifecycleMsg) (tea.Model, tea.Cmd) {
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
}

func (m *bubbleModel) updatePermissionRequest(message permissionRequestMsg) (tea.Model, tea.Cmd) {
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
}

func (m *bubbleModel) updateToolResult(message toolResultMsg) (tea.Model, tea.Cmd) {
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
}

func (m *bubbleModel) updateModelsFetched(message modelsFetchedMsg) (tea.Model, tea.Cmd) {
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
}

func (m *bubbleModel) updateProviderSaved(message providerSavedMsg) (tea.Model, tea.Cmd) {
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
}

func (m *bubbleModel) updateModelSelected(message modelSelectedMsg) (tea.Model, tea.Cmd) {
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
}

func (m *bubbleModel) updateProviderActiveSelected(message providerActiveSelectedMsg) (tea.Model, tea.Cmd) {
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
}

func (m *bubbleModel) updateProviderDeleted(message providerDeletedMsg) (tea.Model, tea.Cmd) {
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
}

func (m *bubbleModel) updateTurnDelta(message turnDeltaMsg) (tea.Model, tea.Cmd) {
	batch := []app.Event{message.event}
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
}

func (m *bubbleModel) updateTurnEventsClosed(message turnEventsClosedMsg) (tea.Model, tea.Cmd) {
	slog.DebugContext(m.ctx, "tui turn event channel closed unexpectedly",
		"busy", m.busy,
		"context_error", m.ctx.Err() != nil,
	)
	if !m.busy || m.ctx.Err() != nil {
		return m, nil
	}
	return m.Update(turnDoneMsg{err: errTurnEventsClosed})
}

func (m *bubbleModel) updateTurnDone(message turnDoneMsg) (tea.Model, tea.Cmd) {
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
