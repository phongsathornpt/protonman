package modelprofile

import (
	"testing"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func TestResolveBuiltinKnownFamilies(t *testing.T) {
	tests := []struct {
		model       string
		profile     string
		wantDefault sdk.ReasoningEffort
		wantLevels  int
		wantContext int
	}{
		{model: "gemini-3.8-flash", profile: "gemini-3.8-flash", wantDefault: sdk.ReasoningMedium, wantLevels: 3, wantContext: 1_048_576},
		{model: "gpt-5.6-sol", profile: "gpt-5.6-family", wantDefault: sdk.ReasoningMedium, wantLevels: 6, wantContext: 1_050_000},
		{model: "grok-4.6-fast", profile: "grok-4.6", wantDefault: sdk.ReasoningHigh, wantLevels: 4},
		{model: "claude-opus-5", profile: "claude-adaptive-thinking", wantLevels: 0},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := ResolveBuiltin("gateway", tt.model, CatalogMetadata{})
			if got.ProfileName != tt.profile || got.Reasoning.Default != tt.wantDefault || len(got.Reasoning.Levels) != tt.wantLevels || got.ContextWindow != tt.wantContext {
				t.Fatalf("ResolveBuiltin() = %+v", got)
			}
			if got.Capabilities.Reasoning != SupportYes || got.Reasoning.Support != SupportYes {
				t.Fatalf("reasoning support = %+v", got)
			}
		})
	}
}

func TestCatalogExplicitMetadataOverridesBuiltin(t *testing.T) {
	no := false
	yes := true
	got := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{
		Tools:         &no,
		Vision:        &no,
		ContextWindow: 1234,
		Reasoning: &CatalogReasoning{
			Supported: &yes,
			Levels:    []sdk.ReasoningEffort{sdk.ReasoningLow},
			Default:   sdk.ReasoningLow,
		},
	})
	if got.Capabilities.Tools != SupportNo || got.Capabilities.Vision != SupportNo || got.ContextWindow != 1234 {
		t.Fatalf("catalog capability override = %+v", got)
	}
	if len(got.Reasoning.Levels) != 1 || got.Reasoning.Levels[0] != sdk.ReasoningLow || got.Reasoning.Default != sdk.ReasoningLow {
		t.Fatalf("catalog reasoning override = %+v", got.Reasoning)
	}
}

func TestCatalogOmissionPreservesBuiltinKnowledge(t *testing.T) {
	got := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{})
	if got.Capabilities.Tools != SupportYes || got.Capabilities.Vision != SupportYes || got.ContextWindow != 1_048_576 {
		t.Fatalf("omitted catalog erased builtin metadata: %+v", got)
	}
	if got.Reasoning.Default != sdk.ReasoningMedium || len(got.Reasoning.Levels) != 3 {
		t.Fatalf("omitted catalog erased reasoning metadata: %+v", got.Reasoning)
	}
}

func TestRegistryMergesProviderFamilyAndExactBySpecificity(t *testing.T) {
	registry, err := NewRegistry(
		Profile{Name: "provider", Match: Matcher{Provider: "openai"}, Capabilities: Capabilities{Tools: SupportYes}},
		Profile{Name: "family", Match: Matcher{Prefixes: []string{"model-"}}, ContextWindow: 100},
		Profile{Name: "exact", Match: Matcher{ExactIDs: []string{"model-x"}}, ContextWindow: 200},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := registry.Resolve("openai", "model-x", CatalogMetadata{})
	if got.ProfileName != "exact" || got.Capabilities.Tools != SupportYes || got.ContextWindow != 200 {
		t.Fatalf("resolved = %+v", got)
	}
}

func TestUnknownModelRemainsUnknown(t *testing.T) {
	got := ResolveBuiltin("custom", "future-model", CatalogMetadata{})
	if got.ProfileName != "" || got.Capabilities.Tools != SupportUnknown || got.Capabilities.Reasoning != SupportUnknown || got.ContextWindow != 0 {
		t.Fatalf("unknown model = %+v", got)
	}
}

func TestResolveProfileReasoningClampsPortablePreference(t *testing.T) {
	profile := Resolved{Reasoning: Reasoning{
		Support: SupportYes,
		Levels:  []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium},
	}}
	if got, ok := profile.ResolveProfileReasoning(sdk.ReasoningHigh); !ok || got != sdk.ReasoningMedium {
		t.Fatalf("ResolveProfileReasoning(high) = %q, %v", got, ok)
	}
	if got, ok := profile.ResolveProfileReasoning(sdk.ReasoningLow); !ok || got != sdk.ReasoningLow {
		t.Fatalf("ResolveProfileReasoning(low) = %q, %v", got, ok)
	}
}

func TestResolveProfileReasoningPreservesUnknownProviderDefault(t *testing.T) {
	profile := Resolved{Reasoning: Reasoning{Support: SupportUnknown}}
	if got, ok := profile.ResolveProfileReasoning(sdk.ReasoningHigh); ok || got != sdk.ReasoningDefault {
		t.Fatalf("ResolveProfileReasoning() = %q, %v", got, ok)
	}
}
