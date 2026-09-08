package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/pane"
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
	activity := make(map[string]string, len(m.agentActivity))
	for id, state := range m.agentActivity {
		activity[id] = state.String()
	}
	return pane.AgentRows(pane.AgentsSnapshot{
		Width:            m.width,
		Height:           m.height,
		Retained:         m.agentSnapshot,
		SubagentsEnabled: m.subagentsEnabled,
		Activity:         activity,
	})
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

func agentModelLabel(st agent.AgentStatus) string { return pane.AgentModelLabel(st) }
