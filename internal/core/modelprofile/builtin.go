package modelprofile

import "github.com/phongsathornpt/protonman/proton-sdk/domain"

var builtinRegistry = mustRegistry(
	Profile{
		Name:  "gemini-3.8-flash",
		Match: Matcher{ExactIDs: []string{"gemini-3.8-flash"}, Prefixes: []string{"gemini-3.8-flash"}},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningMedium, domain.ReasoningHigh},
			Default: domain.ReasoningMedium,
		},
		ContextWindow: 1_048_576,
		Compatibility: CompatibilityPolicy{ToolSchemaDialect: ToolSchemaGeminiSubset},
		VisionPolicy: VisionPolicy{
			MaxDimension: 6000, MaxPatches: 10000, PatchSize: 32,
			MaxOutputBytes: 10 * 1024 * 1024, TokenScheme: VisionTokenGeminiTiles, FallbackTokens: 2064,
		},
	},
	Profile{
		Name:         "zai-glm-5.3-family",
		Match:        Matcher{Prefixes: []string{"glm-5.3"}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningHigh, domain.ReasoningMax},
			Default: domain.ReasoningMax,
		},
	},
	Profile{
		Name: "zai-glm-5.3-flash",
		Match: Matcher{
			ExactIDs: []string{"glm-5.3-flash"},
			Prefixes: []string{"glm-5.3-flash"},
		},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningHigh, domain.ReasoningMax},
			Default: domain.ReasoningMax,
		},
		ContextWindow:   1_048_576,
		MaxOutputTokens: 131_072,
	},
	Profile{
		Name: "zai-glm-thinking",
		Match: Matcher{Prefixes: []string{
			"glm-4.5", "glm-4.6", "glm-4.7", "glm-5",
		}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []domain.ReasoningEffort{domain.ReasoningNone},
		},
	},
	Profile{
		Name:  "qwen3.8-flash-family",
		Match: Matcher{Prefixes: []string{"qwen3.8-flash"}},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []domain.ReasoningEffort{domain.ReasoningNone, domain.ReasoningLow, domain.ReasoningMedium, domain.ReasoningXHigh},
			Default: domain.ReasoningXHigh,
		},
	},
	Profile{
		Name:  "qwen3.8-max-family",
		Match: Matcher{Prefixes: []string{"qwen3.8-max"}},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels: []domain.ReasoningEffort{
				domain.ReasoningLow, domain.ReasoningMedium, domain.ReasoningXHigh,
			},
			Default: domain.ReasoningMedium,
		},
	},
	Profile{
		Name: "qwen3-hybrid-thinking",
		Match: Matcher{Prefixes: []string{
			"qwen3.5-plus", "qwen3.6-plus", "qwen3.6-flash",
			"qwen3.7-plus", "qwen3.7-max",
		}},
		Capabilities: Capabilities{Tools: SupportYes, Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []domain.ReasoningEffort{domain.ReasoningNone},
		},
	},
	Profile{
		Name:         "minimax-m3-family",
		Match:        Matcher{Prefixes: []string{"minimax-m3"}},
		Capabilities: Capabilities{Reasoning: SupportYes},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []domain.ReasoningEffort{domain.ReasoningNone},
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
			Levels: []domain.ReasoningEffort{
				domain.ReasoningNone, domain.ReasoningLow, domain.ReasoningHigh, domain.ReasoningMax,
			},
			Default: domain.ReasoningHigh,
		},
	},
	Profile{
		Name: "deepseek-v4.1-flash-family",
		Match: Matcher{
			ExactIDs: []string{"deepseek-flash"},
			Prefixes: []string{"deepseek-v4.1-flash", "deepseek-v4-flash"},
		},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels: []domain.ReasoningEffort{
				domain.ReasoningNone, domain.ReasoningLow, domain.ReasoningHigh, domain.ReasoningMax,
			},
			Default: domain.ReasoningHigh,
		},
	},
	Profile{
		Name: "muse-spark-1.3-family",
		Match: Matcher{
			ExactIDs: []string{
				"muse-spark-1.3",
				"muse-spark-1.3-contributor",
				"muse-spark-1.3-contributor-free",
			},
			Prefixes: []string{"muse-spark-1.3"},
		},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels: []domain.ReasoningEffort{
				domain.ReasoningMinimal, domain.ReasoningLow, domain.ReasoningMedium,
				domain.ReasoningHigh, domain.ReasoningXHigh, domain.ReasoningMax,
			},
			Default: domain.ReasoningHigh,
		},
		VisionPolicy: DefaultVisionPolicy(),
	},
	gpt56Profile(
		"gpt-5.6-sol",
		[]string{"gpt-5.6-sol", "gpt-5.6"},
	),
	gpt56Profile("gpt-5.6-terra", []string{"gpt-5.6-terra"}),
	gpt56Profile("gpt-5.6-luna", []string{"gpt-5.6-luna"}),
	Profile{
		Name:  "grok-4.6",
		Match: Matcher{Prefixes: []string{"grok-4.6"}},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels:  []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningMedium, domain.ReasoningHigh, domain.ReasoningXHigh},
			Default: domain.ReasoningHigh,
		},
		VisionPolicy: DefaultVisionPolicy(),
	},
	Profile{
		Name: "claude-adaptive-thinking",
		Match: Matcher{Prefixes: []string{
			"claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8", "claude-opus-5",
			"claude-sonnet-4-6", "claude-sonnet-5", "claude-fable-5", "claude-mythos-5",
		}},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{Support: SupportYes},
		VisionPolicy: VisionPolicy{
			MaxDimension: 1568, MaxPatches: 1160, PatchSize: 32,
			MaxOutputBytes: 10 * 1024 * 1024, TokenScheme: VisionTokenAnthropicPixels, FallbackTokens: 1600,
		},
		Compatibility: CompatibilityPolicy{ThinkingMode: ThinkingModeAdaptive},
	},
)

func ResolveBuiltin(provider, modelID string, catalog CatalogMetadata) Resolved {
	return builtinRegistry.Resolve(provider, modelID, catalog)
}

func mustRegistry(profiles ...Profile) *Registry {
	// Built-in profiles are a compile-time constant exercised by the
	// TestResolveBuiltin* suite; a construction failure here is a programmer
	// error, not a runtime condition, so fail fast at package init.
	registry, err := NewRegistry(profiles...)
	if err != nil {
		panic(err)
	}
	return registry
}

func gpt56Profile(name string, exactIDs []string) Profile {
	return Profile{
		Name: name,
		Match: Matcher{
			ExactIDs: exactIDs,
			Prefixes: []string{name},
		},
		Capabilities: Capabilities{
			Tools: SupportYes, Vision: SupportYes, Reasoning: SupportYes,
		},
		Reasoning: Reasoning{
			Support: SupportYes,
			Levels: []domain.ReasoningEffort{
				domain.ReasoningNone, domain.ReasoningLow, domain.ReasoningMedium,
				domain.ReasoningHigh, domain.ReasoningXHigh, domain.ReasoningMax,
			},
			Default: domain.ReasoningMedium,
		},
		ContextWindow:   1_050_000,
		MaxOutputTokens: 131_072,
		VisionPolicy:    DefaultVisionPolicy(),
	}
}
