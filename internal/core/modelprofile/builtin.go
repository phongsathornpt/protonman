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
	},
	Profile{
		Name:         "zai-glm-5.3-family",
		Match:        Matcher{Prefixes: []string{"glm-5.3"}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningHigh, sdk.ReasoningMax},
			Default: sdk.ReasoningMax,
		},
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
		Name:         "qwen3.8-flash-family",
		Match:        Matcher{Prefixes: []string{"qwen3.8-flash"}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningNone, sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningXHigh},
			Default: sdk.ReasoningXHigh,
		},
	},
	Profile{
		Name:  "qwen3.8-max-family",
		Match: Matcher{Prefixes: []string{"qwen3.8-max"}},
		Capabilities: Capabilities{
			Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels: []sdk.ReasoningEffort{
				sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningXHigh,
			},
			Default: sdk.ReasoningMedium,
		},
	},
	Profile{
		Name: "qwen3-hybrid-thinking",
		Match: Matcher{Prefixes: []string{
			"qwen3.5-plus", "qwen3.6-plus", "qwen3.6-flash",
			"qwen3.7-plus", "qwen3.7-max",
		}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningNone},
		},
	},
	Profile{
		Name:         "minimax-m3-family",
		Match:        Matcher{Prefixes: []string{"minimax-m3"}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningNone},
		},
	},
	Profile{
		Name:  "deepseek-v4-family",
		Match: Matcher{Prefixes: []string{"deepseek-v4"}},
		Capabilities: Capabilities{
			Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels: []sdk.ReasoningEffort{
				sdk.ReasoningNone, sdk.ReasoningLow, sdk.ReasoningHigh, sdk.ReasoningMax,
			},
			Default: sdk.ReasoningHigh,
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
				sdk.ReasoningHigh, sdk.ReasoningXHigh, sdk.ReasoningMax,
			},
			Default: sdk.ReasoningHigh,
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
