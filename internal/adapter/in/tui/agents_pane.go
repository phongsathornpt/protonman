package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

const agentsViewID = "agents"

type agentsPaneView struct{}

func (*agentsPaneView) ID() string             { return agentsViewID }
func (*agentsPaneView) ReplacesComposer() bool { return false }
func (*agentsPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "esc", "enter":
		m.bottom.remove(agentsViewID)
		return true, nil
	default:
		return false, nil
	}
}

func (*agentsPaneView) Render(m *bubbleModel) string {
	return renderModalRows(m, promptBorder, agentInspectionRows(m))
}

func agentInspectionRows(m *bubbleModel) []string {
	snapshot := append([]agent.AgentStatus(nil), m.agentSnapshot...)
	if len(snapshot) == 0 {
		rows := []string{brandStyle.Render("Agents")}
		if !m.subagentsEnabled {
			rows = append(rows, warningStyle.Render("Subagents disabled"), mutedStyle.Render("Universal handles work directly."))
		} else {
			rows = append(rows, mutedStyle.Render("No subagents in this session."))
		}
		return append(rows, mutedStyle.Render("esc close"))
	}
	sort.SliceStable(snapshot, func(i, j int) bool {
		return agentDisplayPriority(snapshot[i].State) < agentDisplayPriority(snapshot[j].State)
	})
	limit := 8
	if layoutModeForHeight(m.height) == layoutCompact {
		limit = 4
	}
	if len(snapshot) > limit {
		snapshot = snapshot[:limit]
	}
	rows := []string{brandStyle.Render(fmt.Sprintf("Agents · %d retained", len(m.agentSnapshot)))}
	if !m.subagentsEnabled {
		rows = append(rows, warningStyle.Render("New delegation disabled · existing agents remain manageable"))
	}
	now := time.Now()
	for _, st := range snapshot {
		identity := agentDisplayProfile(st)
		header := fmt.Sprintf("%s  %-9s %s", identity, string(st.State), formatElapsed(agentDisplayDuration(st, now)))
		rows = append(rows, commandStyle.Render(strings.TrimSpace(header)))
		if task := strings.TrimSpace(st.Task); task != "" {
			rows = append(rows, "  "+truncateWithEllipsis(task, maxInt(12, m.width-8)))
		}
		if modelLabel := agentModelLabel(st); modelLabel != "" {
			rows = append(rows, mutedStyle.Render("  "+truncateWithEllipsis(modelLabel, maxInt(12, m.width-8))))
		}
		if activity := m.agentActivity[st.ID].String(); activity != "" && !st.State.Terminal() {
			rows = append(rows, mutedStyle.Render("  "+truncateWithEllipsis(activity, maxInt(12, m.width-8))))
		} else if reason := strings.TrimSpace(st.Reason); reason != "" {
			rows = append(rows, errorStyle.Render("  "+truncateWithEllipsis(reason, maxInt(12, m.width-8))))
		}
		rows = append(rows, mutedStyle.Render("  id: "+st.ID))
	}
	if hidden := len(m.agentSnapshot) - len(snapshot); hidden > 0 {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("… %d more retained", hidden)))
	}
	rows = append(rows, mutedStyle.Render("esc close"))
	return rows
}

func (m *bubbleModel) openAgentsPane() tea.Cmd {
	if m.bottom.has(agentsViewID) {
		m.bottom.remove(agentsViewID)
	} else {
		if m.agents.Available() {
			m.syncAgentSnapshot()
		}
		m.bottom.push(&agentsPaneView{})
	}
	m.relayout()
	return nil
}

func agentModelLabel(st agent.AgentStatus) string {
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
