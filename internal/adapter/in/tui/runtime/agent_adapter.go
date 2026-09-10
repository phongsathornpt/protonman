package runtime

import (
	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/agentui"
	agentpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/agent"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const agentsViewID = "agents"

type AgentActivity = agentui.Activity

type agentRuntimeState struct {
	state                          agentui.RuntimeState
	reasoningPreference            sdk.ReasoningEffort
	reasoningPreferenceSet         bool
	reasoningPreferenceSource      reasoningPreferenceSource
	reasoningCompatibilityFallback bool
}

func newAgentRuntimeState(cfg config.AgentConfig, configured bool) agentRuntimeState {
	state := agentRuntimeState{state: agentui.NewRuntimeState(cfg, configured)}
	if configured {
		state.reasoningPreference = cfg.ReasoningEffort
		state.reasoningPreferenceSet = true
	}
	return state
}

func (s agentRuntimeState) apply(m *bubbleModel) {
	if m == nil {
		return
	}
	m.maxToolCalls = s.state.MaxToolCalls
	m.agentProfile = s.state.Profile
	m.subagentsEnabled = s.state.SubagentsEnabled
	m.reasoningEffort = s.state.ReasoningEffort
	m.reasoningPreference = s.reasoningPreference
	m.reasoningPreferenceSet = s.reasoningPreferenceSet
	m.reasoningPreferenceSource = s.reasoningPreferenceSource
	m.reasoningCompatibilityFallback = s.reasoningCompatibilityFallback
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
	s.reasoningPreference = m.reasoningPreference
	s.reasoningPreferenceSet = m.reasoningPreferenceSet
	s.reasoningPreferenceSource = m.reasoningPreferenceSource
	s.reasoningCompatibilityFallback = m.reasoningCompatibilityFallback
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
func (*agentsPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch message.String() {
	case "esc", "enter":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: agentsViewID}}
	default:
		return paneKeyResult{}
	}
}
func (*agentsPaneView) Render(ctx paneRenderContext) string {
	return renderModalRows(ctx, promptBorder, agentInspectionRows(ctx))
}
func agentInspectionRows(ctx paneRenderContext) []string {
	return agentpane.AgentRows(agentpane.AgentsSnapshot{Width: ctx.width, Height: ctx.height, Retained: ctx.agentSnapshot, SubagentsEnabled: ctx.subagentsEnabled, Activity: ctx.agentActivity})
}
func (m *bubbleModel) openAgentsPane() tea.Cmd {
	if m.panes.bottom.has(agentsViewID) {
		m.panes.bottom.remove(agentsViewID)
	} else {
		if m.agents.Available() {
			m.syncAgentSnapshot()
		}
		m.panes.bottom.push(&agentsPaneView{})
	}
	m.requestRelayout()
	return nil
}
func agentModelLabel(st agent.AgentStatus) string { return agentpane.AgentModelLabel(st) }
