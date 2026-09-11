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
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func (m bubbleModel) statusView() string {
	if m.hasPermissionView() {
		return warningStyle.Render(truncateWithEllipsis("action required · permission", maxInt(1, m.layout.width-2)))
	}
	if !m.busy {
		return m.goalStatusView()
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
	meta := ""
	if activeAgents == 0 {
		// Only an explicit label (a named call, cancellation, or background
		// operation) outranks the derived signals. Everything else resolves
		// deterministically: retry countdown, then the running tool, then the
		// active profile's intent. No generic assistant busy word is invented.
		explicit := activity != "" && activity != "ready"
		if retryActivity, retryMeta, ok := modelRetryStatus(m.turnProgress.Retry, time.Now()); ok {
			activity, meta = retryActivity, retryMeta
		} else if !explicit {
			if running, ok := m.ensureHistoryState().LastRunningTool(); ok {
				activity = tool.DisplayName(strings.TrimSpace(running.Name))
				if target := strings.TrimSpace(running.Target); target != "" {
					activity += " " + target
				}
			} else {
				activity = m.rootActivityLabel()
			}
		}
		if meta == "" && m.turnProgress.ToolCalls > 0 {
			label := "tool"
			if m.turnProgress.ToolCalls != 1 {
				label = "tools"
			}
			meta = fmt.Sprintf(" · %d %s", m.turnProgress.ToolCalls, label)
		}
	}
	indicator := brandMarkStyle.Render("◌")
	if spin := m.spinnerIndicator(); spin != "" {
		indicator = spin
	}
	maxWidth := maxInt(1, m.layout.width-2)
	contentWidth := maxInt(1, maxWidth-2-len([]rune(meta)))
	activity = truncateWithEllipsis(activity, contentWidth)
	busyLine := indicator + " " + systemStyle.Render(activity) + mutedStyle.Render(meta)
	if goal := m.goalStatusView(); goal != "" {
		return goal + "\n" + busyLine
	}
	return busyLine
}

func (m bubbleModel) goalStatusView() string {
	goal := strings.TrimSpace(m.activeGoal)
	if goal == "" {
		return ""
	}
	maxWidth := maxInt(1, m.layout.width-2)
	prefix := "Goal  "
	goalWidth := maxInt(1, maxWidth-len(prefix))
	return mutedStyle.Render(prefix) + systemStyle.Render(truncateWithEllipsis(goal, goalWidth))
}

// rootActivityLabel is the deterministic busy label for the primary agent when
// no tool, retry, or explicit activity is available. It comes from the active
// profile's activity vocabulary rather than a generic assistant word.
func (m bubbleModel) rootActivityLabel() string {
	profile, err := agent.ParseProfile(strings.TrimSpace(m.agentProfile))
	if err != nil || !profile.Valid() {
		profile = agent.ProfileUniversal
	}
	return agentui.ActivityForState(profile, agent.StateRunning).String()
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
	case agentui.ActivitySticking, agentui.ActivityIntegrated:
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

func modelRetryStatus(retry sdk.RetryEvent, now time.Time) (string, string, bool) {
	if retry.Attempt <= 0 || retry.RetryAt.IsZero() {
		return "", "", false
	}
	remaining := retry.RetryAt.Sub(now)
	wait := "now"
	if remaining > 0 {
		if remaining < time.Second {
			wait = "<1s"
		} else {
			seconds := int((remaining + time.Second - 1) / time.Second)
			wait = fmt.Sprintf("%ds", seconds)
		}
	}
	activity := "retrying " + wait
	if retry.Phase == sdk.RetryPhaseCooldown {
		if wait == "now" {
			activity = "cooldown complete"
		} else {
			activity = "cooling down " + wait
		}
	} else if wait != "now" {
		activity = "retrying in " + wait
	}
	meta := fmt.Sprintf(" · retry %d", retry.Attempt)
	if retry.MaxRetries > 0 {
		meta += fmt.Sprintf("/%d", retry.MaxRetries)
	}
	if reason := retryReasonLabel(retry.Reason); reason != "" {
		meta += " · " + reason
	}
	return activity, meta, true
}

func retryReasonLabel(reason string) string {
	switch strings.TrimSpace(reason) {
	case "incomplete_stream":
		return "stream interrupted"
	case "first_event_timeout":
		return "provider slow"
	case "idle_event_timeout":
		return "stream stalled"
	case "max_stream_duration":
		return "stream limit"
	case "rate_limit":
		return "rate limited"
	case "overloaded":
		return "provider busy"
	case "transport":
		return "connection interrupted"
	}
	return strings.ReplaceAll(strings.TrimSpace(reason), "_", " ")
}
