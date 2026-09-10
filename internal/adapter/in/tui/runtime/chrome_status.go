package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/agentui"
	agentpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/agent"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func (m bubbleModel) statusView() string {
	if m.hasPermissionView() {
		return warningStyle.Render(truncateWithEllipsis("action required · permission", maxInt(1, m.layout.width-2)))
	}
	if !m.busy {
		return ""
	}
	agentSnapshot := m.turnAgentSnapshot()
	activeAgents, _, _, _ := agentActivityCounts(agentSnapshot)
	activity := strings.TrimSpace(m.activity)
	if activeAgents > 0 {
		label := "agent"
		if activeAgents != 1 {
			label = "agents"
		}
		dota := dominantAgentActivity(agentSnapshot, m.agentActivity)
		if activity == "canceling" {
			dota = agentui.ActivityRetreating.Label()
		}
		activity = fmt.Sprintf("%s · %d %s", dota, activeAgents, label)
	}
	if activity == "" || activity == "ready" {
		activity = "analyzing"
	}
	meta := ""
	if activeAgents == 0 {
		if running, ok := m.ensureHistoryState().LastRunningTool(); ok && activity == "analyzing" {
			activity = tool.DisplayName(strings.TrimSpace(running.Name))
			if target := strings.TrimSpace(running.Target); target != "" {
				activity += " " + target
			}
		}
		if m.turnProgress.ToolCalls > 0 {
			label := "tool"
			if m.turnProgress.ToolCalls != 1 {
				label = "tools"
			}
			meta = fmt.Sprintf(" · %d %s", m.turnProgress.ToolCalls, label)
		}
	}
	indicator := brandMarkStyle.Render("◌")
	if spin := m.spinner.View(); spin != "" {
		indicator = spin
	}
	maxWidth := maxInt(1, m.layout.width-2)
	contentWidth := maxInt(1, maxWidth-2-len([]rune(meta)))
	activity = truncateWithEllipsis(activity, contentWidth)
	return indicator + " " + systemStyle.Render(activity) + mutedStyle.Render(meta)
}

func dominantAgentActivity(snapshot []agent.AgentStatus, activities map[string]AgentActivity) string {
	type ranked struct {
		label string
		rank  int
	}
	best := ranked{label: agentui.ActivityWaiting.Label(), rank: 0}
	for _, st := range snapshot {
		if st.State.Terminal() {
			continue
		}
		a := activities[st.ID]
		if a.String() == "" {
			a = agentui.ActivityForState(st.Profile, st.State)
		}
		rank := agentActivityRank(a.Intent)
		if rank > best.rank {
			best = ranked{label: a.Intent.Label(), rank: rank}
		}
	}
	if strings.TrimSpace(best.label) == "" {
		return agentui.ActivityRoaming.Label()
	}
	return best.label
}

func agentActivityRank(intent agentui.ActivityIntent) int {
	switch intent {
	case agentui.ActivityRetreating:
		return 90
	case agentui.ActivityCare:
		return 80
	case agentui.ActivityDefending:
		return 70
	case agentui.ActivityPushing:
		return 60
	case agentui.ActivityGanking:
		return 50
	case agentui.ActivitySticking:
		return 45
	case agentui.ActivitySkilling:
		return 40
	case agentui.ActivityFarming:
		return 30
	case agentui.ActivityRoaming:
		return 20
	case agentui.ActivityWaiting:
		return 10
	default:
		return 0
	}
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
			return paneKeyboardHelp(maxInt(1, m.layout.width-2), "tab", "Review", "y", "Once", "s", "Session", "n", "Deny")
		}
		return paneKeyboardHelp(maxInt(1, m.layout.width-2), "y", "Once", "s", "Session", "n", "Deny", "esc", "Review")
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
