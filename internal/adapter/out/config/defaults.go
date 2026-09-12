package config

import (
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
)

// DefaultSnapshot returns a fresh effective settings snapshot built only from
// canonical runtime policy. Callers may mutate the returned maps and slices.
func DefaultSnapshot() Snapshot {
	return Snapshot{
		Permission:     permission.Config{Rules: make([]permission.Rule, 0), Default: permission.ActionAsk},
		Mode:           permission.ModeAsk,
		ProtectedPaths: make([]string, 0),
		Sandbox:        sandbox.NameOff,
		Providers:      make(map[string]ProviderConfig),
		Agent: AgentConfig{
			SubagentsEnabled:     true,
			Subagents:            make(map[string]SubagentModelConfig),
			MaxToolCalls:         runtimepolicy.TurnMaxToolCalls,
			MaxLiveSubagents:     runtimepolicy.AgentMaxLive,
			MaxRetainedSubagents: runtimepolicy.AgentMaxRetained,
			SubagentMaxRuntime:   runtimepolicy.AgentMaxRuntime,
			SubagentWaitTimeout:  runtimepolicy.AgentWaitTimeout,
			SubagentQueueTimeout: runtimepolicy.AgentQueueTimeout,
			CompletedResultTTL:   runtimepolicy.AgentResultTTL,
		},
		Runtime:    DefaultRuntimeConfig(),
		Provenance: defaultProvenance(),
		Sources:    make([]string, 0, 2),
		Warnings:   make([]string, 0),
	}
}

func defaultProvenance() map[string]ValueSource {
	return map[string]ValueSource{
		FieldModelDefault:          SourceDefault,
		FieldModelProvider:         SourceDefault,
		FieldAgentProfile:          SourceDefault,
		FieldAgentSubagentsEnabled: SourceDefault,
		FieldAgentReasoningEffort:  SourceDefault,
		FieldAgentMaxToolCalls:     SourceDefault,
		FieldUIPermissionMode:      SourceDefault,
		FieldSkillsActive:          SourceDefault,
	}
}
