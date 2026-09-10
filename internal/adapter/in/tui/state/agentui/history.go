package agentui

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
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

func (t *Tracker) TouchOperation(name string, call tool.Call, state *history.HistoryState) {
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
