package agentui

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// RuntimeState owns TUI-local agent controls across Bubble Tea program restarts.
type RuntimeState struct {
	MaxToolCalls     int
	Profile          string
	SubagentsEnabled bool
	ReasoningEffort  sdk.ReasoningEffort
}

func NewRuntimeState(cfg config.AgentConfig, configured bool) RuntimeState {
	state := RuntimeState{MaxToolCalls: config.DefaultMaxToolCalls, SubagentsEnabled: true}
	if configured {
		state.MaxToolCalls = cfg.MaxToolCalls
		state.Profile = cfg.Profile
		state.SubagentsEnabled = cfg.SubagentsEnabled
		state.ReasoningEffort = cfg.ReasoningEffort
	}
	return state
}
