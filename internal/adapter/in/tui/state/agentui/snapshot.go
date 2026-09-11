package agentui

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

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
	previousState := cell.State
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
	} else if previousState != status.State || strings.TrimSpace(cell.Activity) == "" {
		cell.Activity = ActivityForState(status.Profile, status.State).String()
	}
	state.TouchAgentRun(agentID)
}
