package runtime

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/agentui"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/pane"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

const agentsViewID = "agents"

type AgentActivity = agentui.Activity

type agentRuntimeState struct{ state agentui.RuntimeState }

func newAgentRuntimeState(cfg config.AgentConfig, configured bool) agentRuntimeState {
	return agentRuntimeState{state: agentui.NewRuntimeState(cfg, configured)}
}

func (s agentRuntimeState) apply(m *bubbleModel) {
	if m == nil {
		return
	}
	m.maxToolCalls = s.state.MaxToolCalls
	m.agentProfile = s.state.Profile
	m.subagentsEnabled = s.state.SubagentsEnabled
	m.reasoningEffort = s.state.ReasoningEffort
	m.agents.SetEnabled(s.state.SubagentsEnabled)
}

func (s *agentRuntimeState) capture(m *bubbleModel) {
	if s == nil || m == nil {
		return
	}
	s.state.MaxToolCalls = m.maxToolCalls
	s.state.Profile = m.agentProfile
	s.state.SubagentsEnabled = m.subagentsEnabled
	s.state.ReasoningEffort = m.reasoningEffort
}

func agentActivityFromEvent(ev agent.Event) AgentActivity { return agentui.ActivityFromEvent(ev) }

func (m *bubbleModel) rememberAgentRun(call tool.Call) {
	m.agentHistory.RememberRun(call)
}

func (m *bubbleModel) touchAgentOperation(name string, call tool.Call) {
	m.agentHistory.TouchOperation(name, call, m.ensureHistoryState())
}

func (m *bubbleModel) applyAgentToolResult(name string, result tool.Result, body string) bool {
	return m.agentHistory.ApplyToolResult(name, result, body, m.ensureHistoryState())
}

func (m *bubbleModel) syncAgentRunSnapshot(agentID string) {
	m.agentHistory.SyncSnapshot(agentID, m.agentSnapshot, m.ensureHistoryState())
}

func (m *bubbleModel) applyAgentToolFailure(name string, result tool.Result, err error) bool {
	return m.agentHistory.ApplyToolFailure(name, result, err, m.ensureHistoryState())
}

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
	return pane.AgentRows(pane.AgentsSnapshot{Width: m.width, Height: m.height, Retained: m.agentSnapshot, SubagentsEnabled: m.subagentsEnabled, Activity: activity})
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
