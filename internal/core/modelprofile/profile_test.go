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
		Tools:              &no,
		Vision:             &no,
		ToolChoiceRequired: &yes,
		ContextWindow:      1234,
		Reasoning: &CatalogReasoning{
			Supported: &yes,
			Levels:    []sdk.ReasoningEffort{sdk.ReasoningLow},
			Default:   sdk.ReasoningLow,
		},
	})
	if got.Capabilities.Tools != SupportNo || got.Capabilities.Vision != SupportNo || got.Capabilities.ToolChoiceRequired != SupportYes || got.ContextWindow != 1234 {
		t.Fatalf("catalog capability override = %+v", got)
	}
	if len(got.Reasoning.Levels) != 1 || got.Reasoning.Levels[0] != sdk.ReasoningLow || got.Reasoning.Default != sdk.ReasoningLow {
		t.Fatalf("catalog reasoning override = %+v", got.Reasoning)
	}
}

func TestCatalogToolChoiceRequiredTracksOverrideProvenance(t *testing.T) {
	yes := true
	got := ResolveBuiltin("gateway", "future-model", CatalogMetadata{ToolChoiceRequired: &yes})
	if !got.CatalogOverride || got.Capabilities.ToolChoiceRequired != SupportYes {
		t.Fatalf("tool choice provenance = %+v", got)
	}
}

func TestCatalogReasoningDisabledClearsInheritedLevels(t *testing.T) {
	no := false
	got := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{
		Reasoning: &CatalogReasoning{Supported: &no},
	})
	if got.Reasoning.Support != SupportNo || got.Capabilities.Reasoning != SupportNo {
		t.Fatalf("reasoning support = %+v", got)
	}
	if len(got.Reasoning.Levels) != 0 || got.Reasoning.Default != sdk.ReasoningDefault {
		t.Fatalf("disabled reasoning retained stale metadata = %+v", got.Reasoning)
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

func TestResolveExplicitReasoningNeverClamps(t *testing.T) {
	profile := Resolved{ModelID: "limited", Reasoning: Reasoning{
		Support: SupportYes,
		Levels:  []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium},
	}}
	if _, err := profile.ResolveExplicitReasoning(sdk.ReasoningHigh); err == nil {
		t.Fatal("ResolveExplicitReasoning(high) error = nil")
	}
	if got, err := profile.ResolveExplicitReasoning(sdk.ReasoningMedium); err != nil || got != sdk.ReasoningMedium {
		t.Fatalf("ResolveExplicitReasoning(medium) = %q, %v", got, err)
	}
}

func TestResolveExplicitReasoningAllowsUnknownMetadata(t *testing.T) {
	profile := Resolved{ModelID: "future", Reasoning: Reasoning{Support: SupportUnknown}}
	if got, err := profile.ResolveExplicitReasoning(sdk.ReasoningXHigh); err != nil || got != sdk.ReasoningXHigh {
		t.Fatalf("ResolveExplicitReasoning(xhigh) = %q, %v", got, err)
	}
}

func TestResolveReasoningRecordsPortableClampProvenance(t *testing.T) {
	profile := Resolved{Reasoning: Reasoning{
		Support: SupportYes, Levels: []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium},
	}}
	got, err := profile.ResolveReasoning(sdk.ReasoningHigh, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Requested != sdk.ReasoningHigh || got.Effective != sdk.ReasoningMedium || got.Source != ReasoningSourceAgentProfile || !got.Clamped {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestResolveReasoningRecordsExplicitProvenance(t *testing.T) {
	profile := Resolved{Reasoning: Reasoning{Support: SupportUnknown}}
	got, err := profile.ResolveReasoning(sdk.ReasoningXHigh, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Effective != sdk.ReasoningXHigh || got.Source != ReasoningSourceExplicit || got.Clamped {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestResolvedProfileTracksMatchAndCatalogProvenance(t *testing.T) {
	exact := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{})
	if exact.ProfileMatch != MatchExact || exact.CatalogOverride {
		t.Fatalf("exact provenance = %+v", exact)
	}
	family := ResolveBuiltin("gateway", "gpt-5.6-sol", CatalogMetadata{})
	if family.ProfileMatch != MatchFamily || family.CatalogOverride {
		t.Fatalf("family provenance = %+v", family)
	}
	yes := true
	withCatalog := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{Tools: &yes})
	if withCatalog.ProfileMatch != MatchExact || !withCatalog.CatalogOverride {
		t.Fatalf("catalog provenance = %+v", withCatalog)
	}
	unknown := ResolveBuiltin("gateway", "future-model", CatalogMetadata{})
	if unknown.ProfileMatch != MatchNone || unknown.CatalogOverride {
		t.Fatalf("unknown provenance = %+v", unknown)
	}
}

func TestCatalogIndependentTokenLimitsOverrideProfile(t *testing.T) {
	got := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{MaxInputTokens: 900000, MaxOutputTokens: 32000})
	if !got.CatalogOverride || got.ContextWindow != 1_048_576 || got.MaxInputTokens != 900000 || got.MaxOutputTokens != 32000 {
		t.Fatalf("token limit metadata = %+v", got)
	}
}

func TestResolvedProfileTracksFieldProvenance(t *testing.T) {
	yes := true
	got := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{
		Tools:          &yes,
		MaxInputTokens: 900000,
	})
	if got.Provenance.Tools != MetadataSourceCatalog || got.Provenance.MaxInputTokens != MetadataSourceCatalog {
		t.Fatalf("catalog provenance = %+v", got.Provenance)
	}
	if got.Provenance.Vision != MetadataSourceBuiltin || got.Provenance.ContextWindow != MetadataSourceBuiltin {
		t.Fatalf("builtin provenance = %+v", got.Provenance)
	}
	if got.Provenance.ToolSchemaDialect != MetadataSourceBuiltin || got.Provenance.PromptHints != MetadataSourceBuiltin {
		t.Fatalf("policy provenance = %+v", got.Provenance)
	}
}

func TestCatalogReasoningProvenanceOverridesOnlyPublishedFields(t *testing.T) {
	yes := true
	got := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{
		Reasoning: &CatalogReasoning{Supported: &yes, Levels: []sdk.ReasoningEffort{sdk.ReasoningLow}},
	})
	if got.Provenance.ReasoningSupport != MetadataSourceCatalog || got.Provenance.ReasoningLevels != MetadataSourceCatalog {
		t.Fatalf("reasoning provenance = %+v", got.Provenance)
	}
	if got.Provenance.ReasoningDefault != MetadataSourceBuiltin {
		t.Fatalf("default provenance = %+v", got.Provenance)
	}
}

func TestMetadataProvenanceSummaryIsDeterministic(t *testing.T) {
	got := (MetadataProvenance{
		Tools:             MetadataSourceCatalog,
		ContextWindow:     MetadataSourceBuiltin,
		ToolSchemaDialect: MetadataSourceBuiltin,
	}).Summary()
	want := "tools=catalog,context_window=builtin,tool_schema_dialect=builtin"
	if got != want {
		t.Fatalf("Summary() = %q, want %q", got, want)
	}
}
