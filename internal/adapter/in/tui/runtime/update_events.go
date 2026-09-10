package runtime

import (
	"context"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func (m *bubbleModel) updateAgentLifecycle(message agentLifecycleMsg) tea.Cmd {
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
			m.requestRelayout()
		}
		return m.nextAgentEvent()
	}
	if message.event.Kind == agent.EventAgentCompleted || message.event.Kind == agent.EventAgentFailed {
		delete(m.agentActivity, message.event.AgentID)
	}
	m.syncAgentSnapshot()
	m.syncAgentRunSnapshot(message.event.AgentID)
	m.requestRelayout()
	return m.nextAgentEvent()
}

func (m *bubbleModel) updatePermissionRequest(message permissionRequestMsg) tea.Cmd {
	if !m.busy {
		message.request.response <- permissionResponse{resolution: permission.Resolution{Action: permission.ActionDeny, Reason: "turn is no longer active"}, err: context.Canceled}
		return m.bridge.Next()
	}
	m.openPermission(message.request)
	m.requestRelayout()
	return m.bridge.Next()
}

func (m *bubbleModel) updateToolResult(message toolResultMsg) tea.Cmd {
	slog.DebugContext(m.ctx, "tui direct tool completed", "call_id", message.call.ID, "tool_name", message.call.Name, "success", message.err == nil, "error_type", errorType(message.err))
	m.turnModelState.finishTool()
	m.appendToolResult(message.result, message.err)
	m.syncTodoSnapshot()
	if message.call.ID != "" {
		m.appendModelToolResult(message.call, message.result)
	}
	m.requestRelayout()
	return m.withSpinner(m.drainQueue())
}
