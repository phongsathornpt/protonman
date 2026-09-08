package agentui

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type PendingRun struct {
	Profile agent.Profile
	Task    string
}

type ToolResult struct {
	Action      string
	AgentID     string
	ResumedFrom string
	Profile     agent.Profile
	State       agent.State
	Summary     string
	Reason      string
}

type Tracker struct {
	pendingRuns    map[string]PendingRun
	pendingOps     map[string]string
	pendingActions map[string]string
}

func (t *Tracker) RememberRun(call tool.Call) {
	if t.pendingRuns == nil {
		t.pendingRuns = make(map[string]PendingRun)
	}
	if t.pendingActions == nil {
		t.pendingActions = make(map[string]string)
	}
	t.pendingRuns[call.ID] = RunFromCall(call)
	t.pendingActions[call.ID] = "spawn"
}

func RunFromCall(call tool.Call) PendingRun {
	var input struct {
		Profile string `json:"profile"`
		Task    string `json:"task"`
	}
	_ = json.Unmarshal(call.Arguments, &input)
	profile, _ := agent.ParseSubagentProfile(input.Profile)
	return PendingRun{Profile: profile, Task: strings.TrimSpace(input.Task)}
}

func ParseToolResult(body string) ToolResult {
	var payload struct {
		Action      string        `json:"action"`
		AgentID     string        `json:"agent_id"`
		ResumedFrom string        `json:"resumed_from"`
		Profile     agent.Profile `json:"profile"`
		Status      agent.State   `json:"status"`
		State       agent.State   `json:"state"`
		Reason      string        `json:"reason"`
		Agent       *struct {
			ID     string      `json:"id"`
			State  agent.State `json:"state"`
			Reason string      `json:"reason"`
		} `json:"agent"`
		Result *struct {
			Summary string `json:"summary"`
		} `json:"result"`
		Event *struct {
			AgentID string `json:"agent_id"`
			Message string `json:"message"`
		} `json:"event"`
		Agents []agent.AgentStatus `json:"agents"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return ToolResult{}
	}
	out := ToolResult{Action: strings.TrimSpace(payload.Action), AgentID: payload.AgentID, ResumedFrom: strings.TrimSpace(payload.ResumedFrom), Profile: payload.Profile, State: payload.Status, Reason: strings.TrimSpace(payload.Reason)}
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
	if payload.Event != nil {
		if out.AgentID == "" {
			out.AgentID = strings.TrimSpace(payload.Event.AgentID)
		}
		if out.Summary == "" {
			out.Summary = strings.TrimSpace(payload.Event.Message)
		}
	}
	if out.AgentID != "" {
		for _, status := range payload.Agents {
			if status.ID != out.AgentID {
				continue
			}
			if out.State == "" {
				out.State = status.State
			}
			if out.Reason == "" {
				out.Reason = strings.TrimSpace(status.Reason)
			}
			break
		}
	}
	return out
}

func (t *Tracker) TouchOperation(name string, call tool.Call, state *history.HistoryState) {
	call = tool.NormalizeLegacyCall(call)
	if call.Name != "subagent" {
		return
	}
	action := extractStringArg(call.Arguments, "action")
	if t.pendingActions == nil {
		t.pendingActions = make(map[string]string)
	}
	t.pendingActions[call.ID] = action
	if action == "wait" {
		return
	}
	id := extractStringArg(call.Arguments, "agent_id")
	if t.pendingOps == nil {
		t.pendingOps = make(map[string]string)
	}
	if call.ID != "" && id != "" {
		t.pendingOps[call.ID] = id
	}
	cell := state.AgentRun(id)
	if cell == nil {
		return
	}
	switch action {
	case "get":
		cell.Activity = "checking status"
	case "cancel":
		cell.State, cell.Activity = agent.StateCanceling, "canceling"
	case "resume":
		cell.Activity = "resuming"
	}
	state.TouchAgentRun(id)
}

func (t *Tracker) ApplyToolResult(name string, result tool.Result, body string, state *history.HistoryState) bool {
	publicName := tool.CanonicalName(name)
	parsed := ParseToolResult(body)
	action := strings.ToLower(strings.TrimSpace(parsed.Action))
	if action == "" {
		action = strings.ToLower(strings.TrimSpace(t.pendingActions[result.CallID]))
	}
	if action == "" {
		action = legacySubagentAction(name)
	}
	delete(t.pendingActions, result.CallID)
	if publicName != "subagent" || action == "" {
		return false
	}
	if parsed.AgentID == "" {
		parsed.AgentID = t.pendingOps[result.CallID]
	}
	if action == "list" {
		delete(t.pendingOps, result.CallID)
		return true
	}
	if action == "resume" {
		fromID := parsed.ResumedFrom
		if fromID == "" {
			fromID = t.pendingOps[result.CallID]
		}
		delete(t.pendingOps, result.CallID)
		if parsed.AgentID == "" {
			return true
		}
		resumed := &history.AgentRunCell{AgentID: parsed.AgentID, Profile: parsed.Profile, State: parsed.State, StartedAt: time.Now()}
		if prior := state.AgentRun(fromID); prior != nil {
			if !resumed.Profile.Valid() {
				resumed.Profile = prior.Profile
			}
			resumed.Task = prior.Task
			prior.Activity = ""
			state.TouchAgentRun(fromID)
		}
		if resumed.State == "" {
			resumed.State = agent.StateQueued
		}
		state.DiscardToolCall(result.CallID, publicName)
		state.Append(resumed)
		state.TouchAgentRun(parsed.AgentID)
		return true
	}
	delete(t.pendingOps, result.CallID)
	if action == "spawn" {
		intent := t.pendingRuns[result.CallID]
		delete(t.pendingRuns, result.CallID)
		if parsed.AgentID == "" {
			return false
		}
		if existing := state.AgentRun(parsed.AgentID); existing != nil {
			if intent.Profile.Valid() {
				existing.Profile = intent.Profile
			}
			if intent.Task != "" {
				existing.Task = intent.Task
			}
			if parsed.State != "" {
				existing.State = parsed.State
			}
			if parsed.Summary != "" {
				existing.Summary = parsed.Summary
			}
			if parsed.Reason != "" {
				existing.Reason = parsed.Reason
			}
			state.DiscardToolCall(result.CallID, publicName)
			state.TouchAgentRun(parsed.AgentID)
			return true
		}
		cell := &history.AgentRunCell{AgentID: parsed.AgentID, Profile: intent.Profile, Task: intent.Task, State: parsed.State, Summary: parsed.Summary, Reason: parsed.Reason, StartedAt: time.Now()}
		if cell.State == "" {
			cell.State = agent.StateQueued
		}
		state.DiscardToolCall(result.CallID, publicName)
		state.Append(cell)
		state.TouchAgentRun(parsed.AgentID)
		return true
	}
	if parsed.AgentID == "" {
		return true
	}
	cell := state.AgentRun(parsed.AgentID)
	if cell == nil {
		cell = &history.AgentRunCell{AgentID: parsed.AgentID, State: parsed.State, StartedAt: time.Now()}
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

func (t *Tracker) SyncSnapshot(agentID string, snapshot []agent.AgentStatus, state *history.HistoryState) {
	if strings.TrimSpace(agentID) == "" {
		return
	}
	var status *agent.AgentStatus
	for i := range snapshot {
		if snapshot[i].ID == agentID {
			status = &snapshot[i]
			break
		}
	}
	if status == nil {
		return
	}
	cell := state.AgentRun(agentID)
	if cell == nil {
		return
	}
	cell.Profile = status.Profile
	if strings.TrimSpace(status.Task) != "" {
		cell.Task = status.Task
	}
	cell.State, cell.Reason = status.State, status.Reason
	startedAt := status.StartedAt
	if startedAt.IsZero() {
		startedAt = status.StartTime
	}
	if !startedAt.IsZero() {
		cell.StartedAt = startedAt
	}
	if !status.FinishedAt.IsZero() {
		cell.FinishedAt = status.FinishedAt
	}
	if status.State.Terminal() {
		cell.Activity = ""
	}
	state.TouchAgentRun(agentID)
}

func (t *Tracker) ApplyToolFailure(name string, result tool.Result, err error, state *history.HistoryState) bool {
	action := strings.ToLower(strings.TrimSpace(t.pendingActions[result.CallID]))
	if action == "" {
		action = legacySubagentAction(name)
	}
	delete(t.pendingActions, result.CallID)
	if tool.CanonicalName(name) != "subagent" || action == "" {
		return false
	}
	id := t.pendingOps[result.CallID]
	delete(t.pendingOps, result.CallID)
	if id == "" {
		return true
	}
	cell := state.AgentRun(id)
	if cell == nil {
		return true
	}
	message := "status check failed"
	if action == "cancel" {
		message = "cancel failed"
	} else if action == "resume" {
		message = "resume failed"
	}
	if result.Failure != nil && strings.TrimSpace(result.Failure.Message) != "" {
		message += ": " + strings.TrimSpace(result.Failure.Message)
	} else if err != nil && strings.TrimSpace(err.Error()) != "" {
		message += ": " + strings.TrimSpace(err.Error())
	}
	cell.Activity = message
	state.TouchAgentRun(id)
	return true
}

func legacySubagentAction(name string) string {
	call := tool.NormalizeLegacyCall(tool.Call{Name: name, Arguments: json.RawMessage(`{}`)})
	if call.Name != "subagent" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(extractStringArg(call.Arguments, "action")))
}

type Activity struct {
	ToolName string
	ToolKind tool.Kind
	Target   string
	Label    string
}

func ActivityFromEvent(ev agent.Event) Activity {
	if ev.Call == nil {
		return Activity{Label: strings.TrimSpace(ev.Message)}
	}
	target, kind := toolview.ExtractTarget(ev.Call.Name, "", ev.Call.Arguments)
	activity := Activity{ToolName: ev.Call.Name, ToolKind: kind, Target: target, Label: tool.DisplayName(ev.Call.Name)}
	if strings.TrimSpace(target) != "" {
		activity.Label += " " + strings.TrimSpace(target)
	}
	return activity
}
func (a Activity) String() string { return strings.TrimSpace(a.Label) }

func extractStringArg(args json.RawMessage, key string) string {
	var values map[string]any
	if json.Unmarshal(args, &values) != nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
