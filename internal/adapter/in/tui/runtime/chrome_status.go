package runtime

import (
	"fmt"
	"strings"
	"time"

	agentpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/agent"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func (m bubbleModel) statusView() string {
	if m.hasPermissionView() {
		return warningStyle.Render(truncateWithEllipsis("action required · permission", maxInt(1, m.width-2)))
	}
	if !m.busy {
		return ""
	}
	activeAgents, _, _, _ := agentActivityCounts(m.turnAgentSnapshot())
	activity := strings.TrimSpace(m.activity)
	if activeAgents > 0 {
		label := "agent"
		if activeAgents != 1 {
			label = "agents"
		}
		if activity == "canceling" {
			activity = fmt.Sprintf("stopping %d %s", activeAgents, label)
		} else {
			activity = fmt.Sprintf("%d %s working", activeAgents, label)
		}
	}
	if activity == "" || activity == "ready" {
		activity = "analyzing"
	}
	indicator := "● "
	if spin := m.spinner.View(); spin != "" {
		indicator = spin + " "
	}
	return statusStyle.Render(truncateWithEllipsis(indicator+activity, maxInt(1, m.width-2)))
}

func (m bubbleModel) turnAgentSnapshot() []agent.AgentStatus {
	if m.activeTurnOwner == "" {
		return m.agentSnapshot
	}
	out := make([]agent.AgentStatus, 0, len(m.agentSnapshot))
	for _, st := range m.agentSnapshot {
		if st.ParentID == m.activeTurnOwner {
			out = append(out, st)
		}
	}
	return out
}

func agentActivityCounts(snapshot []agent.AgentStatus) (active, running, queued, canceling int) {
	for _, st := range snapshot {
		switch st.State {
		case agent.StateRunning:
			running++
		case agent.StateQueued:
			queued++
		case agent.StateCanceling:
			canceling++
		}
	}
	active = running + queued + canceling
	return active, running, queued, canceling
}

func (m *bubbleModel) infoView() string {
	if view := m.permissionView(); view != nil {
		if view.parked {
			return mutedStyle.Render("tab review · y once · s session · n deny")
		}
		return mutedStyle.Render("y once · s session · n deny · esc review")
	}
	if m.planMode {
		return planStyle.Render("plan · read-only")
	}
	if m.service != nil {
		switch m.service.Mode() {
		case permission.ModeAlwaysApprove:
			return warningStyle.Render("auto")
		case permission.ModeDeny:
			return errorStyle.Render("deny")
		}
	}
	return ""
}

func formatElapsed(duration time.Duration) string {
	return agentpane.FormatElapsed(duration)
}
