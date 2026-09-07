package tui

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/feature/agent"
)

type pendingAgentRun struct {
	Profile agent.Profile
	Task    string
}

type agentToolResult struct {
	AgentID string
	State   agent.State
	Summary string
	Reason  string
}

func agentRunFromCall(call tool.Call) pendingAgentRun {
	var input struct {
		Profile agent.Profile `json:"profile"`
		Task    string        `json:"task"`
	}
	_ = json.Unmarshal(call.Arguments, &input)
	return pendingAgentRun{Profile: input.Profile, Task: strings.TrimSpace(input.Task)}
}
func parseAgentToolResult(body string) agentToolResult {
	var payload struct {
		AgentID string      `json:"agent_id"`
		Status  agent.State `json:"status"`
		State   agent.State `json:"state"`
		Reason  string      `json:"reason"`
		Agent   *struct {
			ID     string      `json:"id"`
			State  agent.State `json:"state"`
			Reason string      `json:"reason"`
		} `json:"agent"`
		Result *struct {
			Summary string `json:"summary"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return agentToolResult{}
	}
	out := agentToolResult{AgentID: payload.AgentID, State: payload.Status, Reason: strings.TrimSpace(payload.Reason)}
	if out.State == "" {
		out.State = payload.State
	}
	if payload.Agent != nil {
		if out.AgentID == "" {
			out.AgentID = payload.Agent.ID
		}
		if out.State == "" {
			out.State = payload.Agent.State
		}
		if out.Reason == "" {
			out.Reason = strings.TrimSpace(payload.Agent.Reason)
		}
	}
	if payload.Result != nil {
		out.Summary = strings.TrimSpace(payload.Result.Summary)
	}
	return out
}

func (m *bubbleModel) rememberAgentRun(call tool.Call) {
	if m.pendingAgentRuns == nil {
		m.pendingAgentRuns = make(map[string]pendingAgentRun)
	}
	m.pendingAgentRuns[call.ID] = agentRunFromCall(call)
}

func (m *bubbleModel) touchAgentOperation(name string, call tool.Call) {
	id := extractStringArg(call.Arguments, "agent_id")
	if m.pendingAgentOps == nil {
		m.pendingAgentOps = make(map[string]string)
	}
	if call.ID != "" && id != "" {
		m.pendingAgentOps[call.ID] = id
	}
	cell := m.ensureHistoryState().AgentRun(id)
	if cell == nil {
		return
	}
	switch name {
	case "wait_agent":
		cell.Activity = "waiting for completion"
	case "get_agent":
		cell.Activity = "checking status"
	case "cancel_agent":
		cell.State = agent.StateCanceling
		cell.Activity = "canceling"
	}
	m.ensureHistoryState().TouchAgentRun(id)
}
func (m *bubbleModel) applyAgentToolResult(name string, result tool.Result, body string) bool {
	if name != "delegate_task" && !isAgentLifecycleTool(name) {
		return false
	}
	parsed := parseAgentToolResult(body)
	state := m.ensureHistoryState()
	if parsed.AgentID == "" {
		parsed.AgentID = m.pendingAgentOps[result.CallID]
	}
	delete(m.pendingAgentOps, result.CallID)
	if name == "list_agents" {
		return true
	}
	if name == "delegate_task" {
		intent := m.pendingAgentRuns[result.CallID]
		delete(m.pendingAgentRuns, result.CallID)
		if parsed.AgentID == "" {
			return false
		}
		cell := &AgentRunCell{AgentID: parsed.AgentID, Profile: intent.Profile, Task: intent.Task, State: parsed.State, Summary: parsed.Summary, Reason: parsed.Reason, StartedAt: time.Now()}
		if cell.State == "" {
			cell.State = agent.StateQueued
		}
		state.CompleteToolCall(result.CallID, name, cell)
		return true
	}
	if parsed.AgentID == "" {
		return true
	}
	cell := state.AgentRun(parsed.AgentID)
	if cell == nil {
		cell = &AgentRunCell{AgentID: parsed.AgentID, State: parsed.State, StartedAt: time.Now()}
		state.Append(cell)
	}
	if parsed.State != "" {
		cell.State = parsed.State
	}
	if parsed.Summary != "" {
		cell.Summary = parsed.Summary
	}
	if parsed.Reason != "" {
		cell.Reason = parsed.Reason
	}
	if cell.State.Terminal() && cell.FinishedAt.IsZero() {
		cell.FinishedAt = time.Now()
	}
	cell.Activity = ""
	state.TouchAgentRun(parsed.AgentID)
	return true
}

func (m *bubbleModel) syncAgentRunSnapshot(agentID string) {
	if strings.TrimSpace(agentID) == "" {
		return
	}
	var status *agent.AgentStatus
	for i := range m.agentSnapshot {
		if m.agentSnapshot[i].ID == agentID {
			status = &m.agentSnapshot[i]
			break
		}
	}
	if status == nil {
		return
	}
	cell := m.ensureHistoryState().AgentRun(agentID)
	if cell == nil {
		return
	}
	cell.Profile = status.Profile
	if strings.TrimSpace(status.Task) != "" {
		cell.Task = status.Task
	}
	cell.State = status.State
	cell.Reason = status.Reason
	cell.StartedAt = status.StartedAt
	if cell.StartedAt.IsZero() {
		cell.StartedAt = status.StartTime
	}
	cell.FinishedAt = status.FinishedAt
	if status.State.Terminal() {
		cell.Activity = ""
	}
	m.ensureHistoryState().TouchAgentRun(agentID)
}

func (m *bubbleModel) applyAgentToolFailure(name string, result tool.Result, err error) bool {
	if !isAgentLifecycleTool(name) {
		return false
	}
	id := m.pendingAgentOps[result.CallID]
	delete(m.pendingAgentOps, result.CallID)
	if id == "" {
		return true
	}
	cell := m.ensureHistoryState().AgentRun(id)
	if cell == nil {
		return true
	}
	message := "status check failed"
	if name == "cancel_agent" {
		message = "cancel failed"
	}
	if result.Failure != nil && strings.TrimSpace(result.Failure.Message) != "" {
		message += ": " + strings.TrimSpace(result.Failure.Message)
	} else if err != nil && strings.TrimSpace(err.Error()) != "" {
		message += ": " + strings.TrimSpace(err.Error())
	}
	cell.Activity = message
	m.ensureHistoryState().TouchAgentRun(id)
	return true
}

type AgentActivity struct {
	ToolName string
	ToolKind tool.Kind
	Target   string
	Label    string
}

func agentActivityFromEvent(ev agent.Event) AgentActivity {
	if ev.Call == nil {
		return AgentActivity{Label: strings.TrimSpace(ev.Message)}
	}
	target, kind := extractToolTarget(ev.Call.Name, "", ev.Call.Arguments)
	activity := AgentActivity{ToolName: ev.Call.Name, ToolKind: kind, Target: target}
	activity.Label = tool.DisplayName(ev.Call.Name)
	if strings.TrimSpace(target) != "" {
		activity.Label += " " + strings.TrimSpace(target)
	}
	return activity
}

func (a AgentActivity) String() string { return strings.TrimSpace(a.Label) }
