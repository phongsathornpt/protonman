package tui

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestAgentRuntimeStateSurvivesBubbleModelRestart(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	state := newAgentRuntimeState(config.AgentConfig{
		MaxToolCalls:     17,
		Profile:          "dex",
		SubagentsEnabled: true,
		ReasoningEffort:  sdk.ReasoningHigh,
	}, true)

	first := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "")
	first.agents = app.NewAgents(coord)
	state.apply(first)
	first.maxToolCalls = 23
	first.agentProfile = "strength"
	first.subagentsEnabled = false
	first.reasoningEffort = sdk.ReasoningLow
	coord.SetEnabled(false)
	state.capture(first)

	restarted := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "")
	restarted.agents = app.NewAgents(coord)
	state.apply(restarted)
	if restarted.maxToolCalls != 23 || restarted.agentProfile != "strength" || restarted.subagentsEnabled || restarted.reasoningEffort != sdk.ReasoningLow {
		t.Fatalf("restart state = tool_calls=%d profile=%q subagents=%v reasoning=%q", restarted.maxToolCalls, restarted.agentProfile, restarted.subagentsEnabled, restarted.reasoningEffort)
	}
	if coord.Enabled() {
		t.Fatal("restart re-enabled coordinator")
	}
}
