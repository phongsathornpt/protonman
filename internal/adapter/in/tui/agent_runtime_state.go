package tui

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// agentRuntimeState owns TUI-local agent controls across Bubble Tea program
// restarts. Persistent config seeds it once; runtime commands mutate the model,
// and the latest model snapshot is captured before a restart.
type agentRuntimeState struct {
	maxToolCalls     int
	profile          string
	subagentsEnabled bool
	reasoningEffort  sdk.ReasoningEffort
}

func newAgentRuntimeState(cfg config.AgentConfig, configured bool) agentRuntimeState {
	state := agentRuntimeState{
		maxToolCalls:     config.DefaultMaxToolCalls,
		subagentsEnabled: true,
	}
	if configured {
		state.maxToolCalls = cfg.MaxToolCalls
		state.profile = cfg.Profile
		state.subagentsEnabled = cfg.SubagentsEnabled
		state.reasoningEffort = cfg.ReasoningEffort
	}
	return state
}

func (s agentRuntimeState) apply(m *bubbleModel) {
	if m == nil {
		return
	}
	m.maxToolCalls = s.maxToolCalls
	m.agentProfile = s.profile
	m.subagentsEnabled = s.subagentsEnabled
	m.reasoningEffort = s.reasoningEffort
	m.agents.SetEnabled(s.subagentsEnabled)
}

func (s *agentRuntimeState) capture(m *bubbleModel) {
	if s == nil || m == nil {
		return
	}
	s.maxToolCalls = m.maxToolCalls
	s.profile = m.agentProfile
	s.subagentsEnabled = m.subagentsEnabled
	s.reasoningEffort = m.reasoningEffort
}
