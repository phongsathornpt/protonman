package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type AgentsSnapshot struct {
	Width            int
	Height           int
	Retained         []agent.AgentStatus
	SubagentsEnabled bool
	Activity         map[string]string
	Now              time.Time
}

func AgentRows(snapshot AgentsSnapshot) []string {
	retained := append([]agent.AgentStatus(nil), snapshot.Retained...)
	if len(retained) == 0 {
		rows := []string{tuistyle.BrandStyle.Render("Agents")}
		if !snapshot.SubagentsEnabled {
			rows = append(rows, tuistyle.WarningStyle.Render("Subagents disabled"), tuistyle.MutedStyle.Render("Universal handles work directly."))
		} else {
			rows = append(rows, tuistyle.MutedStyle.Render("No subagents in this session."))
		}
		return append(rows, tuistyle.MutedStyle.Render("esc close"))
	}
	sort.SliceStable(retained, func(i, j int) bool {
		return AgentDisplayPriority(retained[i].State) < AgentDisplayPriority(retained[j].State)
	})
	limit := 8
	if panecommon.ModeForHeight(snapshot.Height) == panecommon.LayoutCompact {
		limit = 4
	}
	visible := retained
	if len(visible) > limit {
		visible = visible[:limit]
	}
	rows := []string{tuistyle.BrandStyle.Render(fmt.Sprintf("Agents · %d retained", len(retained)))}
	if !snapshot.SubagentsEnabled {
		rows = append(rows, tuistyle.WarningStyle.Render("New delegation disabled · existing agents remain manageable"))
	}
	now := snapshot.Now
	if now.IsZero() {
		now = time.Now()
	}
	for _, st := range visible {
		header := AgentDisplayProfile(st) + "  " + textview.PadRight(string(st.State), 9) + " " + FormatElapsed(AgentDisplayDuration(st, now))
		rows = append(rows, tuistyle.CommandStyle.Render(strings.TrimSpace(header)))
		if task := strings.TrimSpace(st.Task); task != "" {
			rows = append(rows, "  "+textview.TruncateEllipsis(task, max(12, snapshot.Width-8)))
		}
		if label := AgentModelLabel(st); label != "" {
			rows = append(rows, tuistyle.MutedStyle.Render("  "+textview.TruncateEllipsis(label, max(12, snapshot.Width-8))))
		}
		if activity := strings.TrimSpace(snapshot.Activity[st.ID]); activity != "" && !st.State.Terminal() {
			rows = append(rows, tuistyle.MutedStyle.Render("  "+textview.TruncateEllipsis(activity, max(12, snapshot.Width-8))))
		} else if reason := strings.TrimSpace(st.Reason); reason != "" {
			rows = append(rows, tuistyle.ErrorStyle.Render("  "+textview.TruncateEllipsis(reason, max(12, snapshot.Width-8))))
		}
		rows = append(rows, tuistyle.MutedStyle.Render("  id: "+st.ID))
	}
	if hidden := len(retained) - len(visible); hidden > 0 {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("… %d more retained", hidden)))
	}
	return append(rows, tuistyle.MutedStyle.Render("esc close"))
}

func AgentDisplayProfile(st agent.AgentStatus) string {
	profile := st.Profile
	if !profile.Valid() {
		prefix := st.ID
		if idx := strings.IndexByte(prefix, '-'); idx >= 0 {
			prefix = prefix[:idx]
		}
		if parsed, err := agent.ParseProfile(prefix); err == nil {
			profile = parsed
		}
	}
	if !profile.Valid() {
		return "AGENT"
	}
	return profile.ShortLabel()
}

func AgentDisplayPriority(state agent.State) int {
	switch state {
	case agent.StateCanceling:
		return 0
	case agent.StateRunning:
		return 1
	case agent.StateQueued:
		return 2
	case agent.StateFailed, agent.StateCanceled:
		return 3
	case agent.StateCompleted:
		return 4
	default:
		return 5
	}
}

func AgentDisplayDuration(st agent.AgentStatus, now time.Time) time.Duration {
	start := st.StartTime
	if !st.StartedAt.IsZero() {
		start = st.StartedAt
	}
	if st.State.Terminal() && !st.FinishedAt.IsZero() {
		if st.StartedAt.IsZero() {
			return 0
		}
		return st.FinishedAt.Sub(st.StartedAt)
	}
	if start.IsZero() || now.Before(start) {
		return 0
	}
	return now.Sub(start)
}

func FormatElapsed(duration time.Duration) string {
	if duration < time.Second {
		return "0s"
	}
	return duration.Truncate(time.Second).String()
}

func AgentModelLabel(st agent.AgentStatus) string {
	provider := strings.TrimSpace(st.Provider)
	modelID := strings.TrimSpace(st.Model)
	if modelID == "" {
		return ""
	}
	if provider == "" {
		return modelID
	}
	return provider + " · " + modelID
}
