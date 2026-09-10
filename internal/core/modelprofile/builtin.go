package modelprofile

import sdk "github.com/phongsathornpt/protonman/proton-sdk"

var builtinRegistry = mustRegistry(
	Profile{
		Name:  "gemini-3.8-flash",
		Match: Matcher{ExactIDs: []string{"gemini-3.8-flash"}, Prefixes: []string{"gemini-3.8-flash"}},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningHigh},
			Default: sdk.ReasoningMedium,
		},
		ContextWindow: 1_048_576,
		Compatibility: CompatibilityPolicy{ToolSchemaDialect: ToolSchemaGeminiSubset},
		AgentPolicy: AgentPolicy{PromptHints: []string{
			"Prefer provided structured capabilities over ad-hoc scripts when a capability directly represents the operation.",
			"Use tool and action names exactly as provided; do not invent namespaces, prefixes, or operation names.",
		}},
	},
	Profile{
		Name: "zai-glm-thinking",
		Match: Matcher{Prefixes: []string{
			"glm-4.5", "glm-4.6", "glm-4.7", "glm-5",
		}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningNone},
		},
	},
	Profile{
		Name:  "muse-spark-1.3-family",
		Match: Matcher{Prefixes: []string{"muse-spark-1.3"}},
		Capabilities: Capabilities{
			Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels: []sdk.ReasoningEffort{
				sdk.ReasoningMinimal, sdk.ReasoningLow, sdk.ReasoningMedium,
				sdk.ReasoningHigh, sdk.ReasoningXHigh,
			},
			Default: sdk.ReasoningMedium,
		},
	},
	Profile{
		Name:  "gpt-5.6-family",
		Match: Matcher{Prefixes: []string{"gpt-5.6"}},
		Capabilities: Capabilities{
			Tools: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels: []sdk.ReasoningEffort{
				sdk.ReasoningNone, sdk.ReasoningLow, sdk.ReasoningMedium,
				sdk.ReasoningHigh, sdk.ReasoningXHigh, sdk.ReasoningMax,
			},
			Default: sdk.ReasoningMedium,
		},
		ContextWindow: 1_050_000,
	},
	Profile{
		Name:  "grok-4.6",
		Match: Matcher{Prefixes: []string{"grok-4.6"}},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningHigh, sdk.ReasoningXHigh},
			Default: sdk.ReasoningHigh,
		},
		AgentPolicy: AgentPolicy{PromptHints: []string{
			"Verify tool-dependent claims with the relevant tool result before presenting them as facts.",
		}},
	},
	Profile{
		Name: "claude-adaptive-thinking",
		Match: Matcher{Prefixes: []string{
			"claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8", "claude-opus-5",
			"claude-sonnet-4-6", "claude-sonnet-5", "claude-fable-5", "claude-mythos-5",
		}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning:    Reasoning{Support: SupportYes},
	},
)

func ResolveBuiltin(provider, modelID string, catalog CatalogMetadata) Resolved {
	return builtinRegistry.Resolve(provider, modelID, catalog)
}

func mustRegistry(profiles ...Profile) *Registry {
	registry, err := NewRegistry(profiles...)
	if err != nil {
		panic(err)
	}
	return registry
}
