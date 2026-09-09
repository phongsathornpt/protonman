package agentui

import (
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func (t *Tracker) ApplyToolResult(name string, result tool.Result, body string, state *history.HistoryState) bool {
	publicName := strings.TrimSpace(name)
	parsed := ParseToolResult(body)
	action := strings.ToLower(strings.TrimSpace(parsed.Action))
	if action == "" {
		action = strings.ToLower(strings.TrimSpace(t.pendingActions[result.CallID]))
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

func (t *Tracker) ApplyToolFailure(name string, result tool.Result, err error, state *history.HistoryState) bool {
	action := strings.ToLower(strings.TrimSpace(t.pendingActions[result.CallID]))
	delete(t.pendingActions, result.CallID)
	delete(t.pendingRuns, result.CallID)
	if strings.TrimSpace(name) != tool.NameSubagent || action == "" {
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
