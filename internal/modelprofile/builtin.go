package modelprofile

import sdk "github.com/projectTHORN/proton/proton-sdk"

var builtinRegistry = mustRegistry(
	Profile{
		Name:  "gemini-3.8-flash",
		Match: Matcher{ExactIDs: []string{"gemini-3.8-flash"}},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningHigh},
			Default: sdk.ReasoningMedium,
		},
		ContextWindow: 1_048_576,
		PromptHints: []string{
			"Use tool names exactly as provided; do not invent namespaces or prefixes.",
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
