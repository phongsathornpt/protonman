package agentui

import (
	"encoding/json"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/agentprofile"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func RunFromCall(call tool.Call) PendingRun {
	var input struct {
		Profile string `json:"profile"`
		Task    string `json:"task"`
	}
	_ = json.Unmarshal(call.Arguments, &input)
	profile, _ := agentprofile.ParseSubagentProfile(input.Profile)
	return PendingRun{Profile: profile, Task: strings.TrimSpace(input.Task)}
}

// ParseToolResult parses the subagent structured-output envelope. The boolean
// is false only when a non-empty body exists but is not valid JSON, so callers
// can observe malformed envelopes instead of silently degrading to empty
// state. An empty body is the documented plain-text fallback, not a failure.
func ParseToolResult(body string) (ToolResult, bool) {
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
	if strings.TrimSpace(body) == "" {
		return ToolResult{}, true
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return ToolResult{}, false
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
	return out, true
}
