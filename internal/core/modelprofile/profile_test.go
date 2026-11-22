package modelprofile

import (
	"testing"

	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func TestResolveBuiltinKnownFamilies(t *testing.T) {
	tests := []struct {
		model       string
		profile     string
		wantDefault domain.ReasoningEffort
		wantLevels  int
		wantContext int
	}{
		{model: "gemini-3.8-flash", profile: "gemini-3.8-flash", wantDefault: domain.ReasoningMedium, wantLevels: 3, wantContext: 1_048_576},
		{model: "muse-spark-1.3-contributor-free", profile: "muse-spark-1.3-family", wantDefault: domain.ReasoningHigh, wantLevels: 6},
		{model: "qwen3.8-max", profile: "qwen3.8-max-family", wantDefault: domain.ReasoningMedium, wantLevels: 3},
		{model: "qwen3.8-max-latest", profile: "qwen3.8-max-family", wantDefault: domain.ReasoningMedium, wantLevels: 3},
		{model: "qwen3.8-flash", profile: "qwen3.8-flash-family", wantDefault: domain.ReasoningXHigh, wantLevels: 4},
		{model: "proton/qwen3.8-flash", profile: "qwen3.8-flash-family", wantDefault: domain.ReasoningXHigh, wantLevels: 4},
		{model: "minimax-m3", profile: "minimax-m3-family", wantLevels: 1},
		{model: "qwen3.6-plus", profile: "qwen3-hybrid-thinking", wantLevels: 1},
		{model: "qwen3.6-flash", profile: "qwen3-hybrid-thinking", wantLevels: 1},
		{model: "qwen3.7-max", profile: "qwen3-hybrid-thinking", wantLevels: 1},
		{model: "deepseek-flash", profile: "deepseek-v4.1-flash-family", wantDefault: domain.ReasoningHigh, wantLevels: 4},
		{model: "deepseek-v4.1-flash", profile: "deepseek-v4.1-flash-family", wantDefault: domain.ReasoningHigh, wantLevels: 4},
		{model: "deepseek-v4-flash-free", profile: "deepseek-v4.1-flash-family", wantDefault: domain.ReasoningHigh, wantLevels: 4},
		{model: "router/deepseek-v4-flash-vision-exp", profile: "deepseek-v4.1-flash-family", wantDefault: domain.ReasoningHigh, wantLevels: 4},
		{model: "deepseek-v4-pro", profile: "deepseek-v4-family", wantDefault: domain.ReasoningHigh, wantLevels: 4},
		{model: "glm-5.3-flash", profile: "zai-glm-5.3-flash", wantDefault: domain.ReasoningMax, wantLevels: 3, wantContext: 1_048_576},
		{model: "glm-4.7", profile: "zai-glm-thinking", wantLevels: 1},
		{model: "gpt-5.6-sol", profile: "gpt-5.6-sol", wantDefault: domain.ReasoningMedium, wantLevels: 6, wantContext: 1_050_000},
		{model: "gpt-5.6-terra", profile: "gpt-5.6-terra", wantDefault: domain.ReasoningMedium, wantLevels: 6, wantContext: 1_050_000},
		{model: "gpt-5.6-luna", profile: "gpt-5.6-luna", wantDefault: domain.ReasoningMedium, wantLevels: 6, wantContext: 1_050_000},
		{model: "grok-4.6-fast", profile: "grok-4.6", wantDefault: domain.ReasoningHigh, wantLevels: 4},
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

func TestGeminiFlashProfileCapabilitiesAndReasoningLevels(t *testing.T) {
	for _, modelID := range []string{
		"gemini-3.8-flash",
		"gemini-3.8-flash-latest",
		"router/gemini-3.8-flash",
	} {
		resolved := ResolveBuiltin("google", modelID, CatalogMetadata{})
		if resolved.ProfileName != "gemini-3.8-flash" {
			t.Errorf("%s profile = %q, want gemini-3.8-flash", modelID, resolved.ProfileName)
		}
		if resolved.Capabilities.Tools != SupportYes || resolved.Capabilities.Vision != SupportYes {
			t.Errorf("%s capabilities = %+v", modelID, resolved.Capabilities)
		}
		want := []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningMedium, domain.ReasoningHigh}
		if len(resolved.Reasoning.Levels) != len(want) {
			t.Fatalf("%s reasoning levels = %v, want %v", modelID, resolved.Reasoning.Levels, want)
		}
		for index, effort := range want {
			if resolved.Reasoning.Levels[index] != effort {
				t.Errorf("%s reasoning level %d = %q, want %q", modelID, index, resolved.Reasoning.Levels[index], effort)
			}
		}
	}
}

func TestGPT56ProfilesDeclareCurrentCapabilitiesAndLimits(t *testing.T) {
	for _, modelID := range []string{
		"gpt-5.6-sol",
		"gpt-5.6",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"router/gpt-5.6-sol",
	} {
		resolved := ResolveBuiltin("openai", modelID, CatalogMetadata{})
		if resolved.Capabilities.Tools != SupportYes || resolved.Capabilities.Vision != SupportYes {
			t.Errorf("%s capabilities = %+v", modelID, resolved.Capabilities)
		}
		if resolved.ContextWindow != 1_050_000 || resolved.MaxOutputTokens != 131_072 {
			t.Errorf("%s limits = context %d output %d", modelID, resolved.ContextWindow, resolved.MaxOutputTokens)
		}
	}
}

func TestGPT56ProfilesDoNotApplyToUnknownGPTModels(t *testing.T) {
	resolved := ResolveBuiltin("openai", "gpt-5.7-preview", CatalogMetadata{})
	if resolved.ProfileName != "" || resolved.ContextWindow != 0 || resolved.MaxOutputTokens != 0 {
		t.Fatalf("unknown GPT profile = %+v", resolved)
	}
}

func TestClaudeAdaptiveProfileCapabilities(t *testing.T) {
	resolved := ResolveBuiltin("anthropic", "claude-sonnet-4-6", CatalogMetadata{})
	if resolved.Capabilities.Tools != SupportYes || resolved.Capabilities.Vision != SupportYes {
		t.Fatalf("Claude capabilities = %+v", resolved.Capabilities)
	}
	if resolved.Compatibility.ThinkingMode != ThinkingModeAdaptive {
		t.Fatalf("Claude thinking mode = %q, want adaptive", resolved.Compatibility.ThinkingMode)
	}
	if resolved.Provenance.Tools != MetadataSourceBuiltin || resolved.Provenance.Vision != MetadataSourceBuiltin || resolved.Provenance.ThinkingMode != MetadataSourceBuiltin {
		t.Fatalf("Claude provenance = %+v", resolved.Provenance)
	}
}

func TestClaudeCatalogCapabilitiesOverrideBuiltin(t *testing.T) {
	no := false
	resolved := ResolveBuiltin("anthropic", "claude-sonnet-4-6", CatalogMetadata{Tools: &no, Vision: &no})
	if resolved.Capabilities.Tools != SupportNo || resolved.Capabilities.Vision != SupportNo {
		t.Fatalf("Claude catalog capability override = %+v", resolved.Capabilities)
	}
	if resolved.Compatibility.ThinkingMode != ThinkingModeAdaptive {
		t.Fatalf("catalog omission erased thinking mode = %q", resolved.Compatibility.ThinkingMode)
	}
}

func TestDeepSeekV41FlashCapabilities(t *testing.T) {
	resolved := ResolveBuiltin("gateway", "deepseek-flash", CatalogMetadata{})
	if resolved.Capabilities.Tools != SupportYes || resolved.Capabilities.Vision != SupportYes {
		t.Fatalf("DeepSeek V4.1 Flash capabilities = %+v", resolved.Capabilities)
	}
	if resolved.Provenance.Tools != MetadataSourceBuiltin || resolved.Provenance.Vision != MetadataSourceBuiltin {
		t.Fatalf("DeepSeek capability provenance = %+v", resolved.Provenance)
	}
}

func TestQwenFamilyCapabilities(t *testing.T) {
	for _, modelID := range []string{
		"qwen3.8-flash",
		"dashscope/qwen3.8-flash-latest",
		"qwen3.8-max",
		"router/qwen3.6-plus",
	} {
		resolved := ResolveBuiltin("gateway", modelID, CatalogMetadata{})
		if resolved.Capabilities.Tools != SupportYes {
			t.Errorf("%s tools = %v, want supported", modelID, resolved.Capabilities.Tools)
		}
	}
	for _, modelID := range []string{"qwen3.8-flash", "qwen3.8-max"} {
		resolved := ResolveBuiltin("gateway", modelID, CatalogMetadata{})
		if resolved.Capabilities.Vision != SupportYes {
			t.Errorf("%s vision = %v, want supported", modelID, resolved.Capabilities.Vision)
		}
	}
}

func TestGLM53FlashCapabilities(t *testing.T) {
	for _, modelID := range []string{
		"glm-5.3-flash",
		"glm-5.3-flash-latest",
		"router/glm-5.3-flash",
	} {
		resolved := ResolveBuiltin("gateway", modelID, CatalogMetadata{})
		if resolved.ProfileName != "zai-glm-5.3-flash" {
			t.Errorf("%s profile = %q, want zai-glm-5.3-flash", modelID, resolved.ProfileName)
		}
		if resolved.Capabilities.Tools != SupportYes || resolved.Capabilities.Vision != SupportYes {
			t.Errorf("%s capabilities = %+v", modelID, resolved.Capabilities)
		}
		if resolved.ContextWindow != 1_048_576 || resolved.MaxOutputTokens != 131_072 {
			t.Errorf("%s limits = context %d output %d", modelID, resolved.ContextWindow, resolved.MaxOutputTokens)
		}
	}
}

func TestGLM53BaseProfileDoesNotInheritFlashCapabilities(t *testing.T) {
	resolved := ResolveBuiltin("gateway", "glm-5.3-pro", CatalogMetadata{})
	if resolved.ProfileName != "zai-glm-5.3-family" {
		t.Fatalf("profile = %q, want zai-glm-5.3-family", resolved.ProfileName)
	}
	if resolved.Capabilities.Tools != SupportUnknown || resolved.Capabilities.Vision != SupportUnknown {
		t.Fatalf("base GLM capabilities = %+v, want unknown", resolved.Capabilities)
	}
}

func TestCapabilitiesApplyPreservesUnknownAdapterValues(t *testing.T) {
	base := domain.ModelCapabilities{Streaming: true, Tools: true, Vision: true}
	got := (Capabilities{Tools: SupportUnknown, Vision: SupportUnknown}).Apply(base)
	if got != base {
		t.Fatalf("unknown capability overlay = %+v, want %+v", got, base)
	}
}

func TestCapabilitiesApplyHonorsExplicitModelValues(t *testing.T) {
	base := domain.ModelCapabilities{Streaming: true, Tools: true, Vision: true}
	got := (Capabilities{Tools: SupportNo, Vision: SupportYes}).Apply(base)
	if !got.Vision || got.Tools {
		t.Fatalf("explicit capability overlay = %+v", got)
	}
}

func TestResolveBuiltinMatchesNamespacedModelIDs(t *testing.T) {
	tests := []struct {
		model   string
		profile string
		kind    MatchKind
	}{
		{model: "ag/gemini-3.8-flash", profile: "gemini-3.8-flash", kind: MatchExact},
		{model: "bai/gemini-3.8-flash", profile: "gemini-3.8-flash", kind: MatchExact},
		{model: "dashscope/qwen3.8-max-latest", profile: "qwen3.8-max-family", kind: MatchFamily},
		{model: "router/qwen3.6-plus", profile: "qwen3-hybrid-thinking", kind: MatchFamily},
		{model: "router/gpt-5.6-sol", profile: "gpt-5.6-sol", kind: MatchExact},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := ResolveBuiltin("gateway", tt.model, CatalogMetadata{})
			if got.ProfileName != tt.profile || got.ProfileMatch != tt.kind {
				t.Fatalf("ResolveBuiltin(%q) = profile %q match %q, want %q/%q", tt.model, got.ProfileName, got.ProfileMatch, tt.profile, tt.kind)
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
			Levels:    []domain.ReasoningEffort{domain.ReasoningLow},
			Default:   domain.ReasoningLow,
		},
	})
	if got.Capabilities.Tools != SupportNo || got.Capabilities.Vision != SupportNo || got.Capabilities.ToolChoiceRequired != SupportYes || got.ContextWindow != 1234 {
		t.Fatalf("catalog capability override = %+v", got)
	}
	if len(got.Reasoning.Levels) != 1 || got.Reasoning.Levels[0] != domain.ReasoningLow || got.Reasoning.Default != domain.ReasoningLow {
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
	if len(got.Reasoning.Levels) != 0 || got.Reasoning.Default != domain.ReasoningDefault {
		t.Fatalf("disabled reasoning retained stale metadata = %+v", got.Reasoning)
	}
}

func TestCatalogOmissionPreservesBuiltinKnowledge(t *testing.T) {
	got := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{})
	if got.Capabilities.Tools != SupportYes || got.Capabilities.Vision != SupportYes || got.ContextWindow != 1_048_576 {
		t.Fatalf("omitted catalog erased builtin metadata: %+v", got)
	}
	if got.Reasoning.Default != domain.ReasoningMedium || len(got.Reasoning.Levels) != 3 {
		t.Fatalf("omitted catalog erased reasoning metadata: %+v", got.Reasoning)
	}
}

func TestCatalogCompatibilityOverridesBuiltinWithProvenance(t *testing.T) {
	policy := VisionPolicy{MaxDimension: 6000, MaxPatches: 10000, PatchSize: 32}
	got := ResolveBuiltin("gateway", "muse-spark-1.3-contributor-free", CatalogMetadata{
		ToolSchemaDialect: ToolSchemaGeminiSubset,
		ThinkingMode:      ThinkingModeManual,
		VisionPolicy:      &policy,
	})
	if got.Compatibility.ToolSchemaDialect != ToolSchemaGeminiSubset || got.Compatibility.ThinkingMode != ThinkingModeManual {
		t.Fatalf("compatibility = %+v", got.Compatibility)
	}
	if got.Provenance.ToolSchemaDialect != MetadataSourceCatalog || got.Provenance.ThinkingMode != MetadataSourceCatalog || got.Provenance.VisionPolicy != MetadataSourceCatalog {
		t.Fatalf("compatibility provenance = %+v", got.Provenance)
	}
	if got.VisionPolicy.MaxDimension != 6000 || got.VisionPolicy.MaxPatches != 10000 {
		t.Fatalf("vision policy = %+v", got.VisionPolicy)
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
		Levels:  []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningMedium},
	}}
	if got, ok := profile.ResolveProfileReasoning(domain.ReasoningHigh); !ok || got != domain.ReasoningMedium {
		t.Fatalf("ResolveProfileReasoning(high) = %q, %v", got, ok)
	}
	if got, ok := profile.ResolveProfileReasoning(domain.ReasoningLow); !ok || got != domain.ReasoningLow {
		t.Fatalf("ResolveProfileReasoning(low) = %q, %v", got, ok)
	}
}

func TestResolveProfileReasoningPreservesUnknownProviderDefault(t *testing.T) {
	profile := Resolved{Reasoning: Reasoning{Support: SupportUnknown}}
	if got, ok := profile.ResolveProfileReasoning(domain.ReasoningHigh); ok || got != domain.ReasoningDefault {
		t.Fatalf("ResolveProfileReasoning() = %q, %v", got, ok)
	}
}

func TestResolveExplicitReasoningNeverClamps(t *testing.T) {
	profile := Resolved{ModelID: "limited", Reasoning: Reasoning{
		Support: SupportYes,
		Levels:  []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningMedium},
	}}
	if _, err := profile.ResolveExplicitReasoning(domain.ReasoningHigh); err == nil {
		t.Fatal("ResolveExplicitReasoning(high) error = nil")
	}
	if got, err := profile.ResolveExplicitReasoning(domain.ReasoningMedium); err != nil || got != domain.ReasoningMedium {
		t.Fatalf("ResolveExplicitReasoning(medium) = %q, %v", got, err)
	}
}

func TestResolveExplicitReasoningAllowsUnknownMetadata(t *testing.T) {
	profile := Resolved{ModelID: "future", Reasoning: Reasoning{Support: SupportUnknown}}
	if got, err := profile.ResolveExplicitReasoning(domain.ReasoningXHigh); err != nil || got != domain.ReasoningXHigh {
		t.Fatalf("ResolveExplicitReasoning(xhigh) = %q, %v", got, err)
	}
}

func TestResolveReasoningRecordsPortableClampProvenance(t *testing.T) {
	profile := Resolved{Reasoning: Reasoning{
		Support: SupportYes, Levels: []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningMedium},
	}}
	got, err := profile.ResolveReasoning(domain.ReasoningHigh, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Requested != domain.ReasoningHigh || got.Effective != domain.ReasoningMedium || got.Source != ReasoningSourceAgentProfile || !got.Clamped {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestResolveReasoningRecordsExplicitProvenance(t *testing.T) {
	profile := Resolved{Reasoning: Reasoning{Support: SupportUnknown}}
	got, err := profile.ResolveReasoning(domain.ReasoningXHigh, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Effective != domain.ReasoningXHigh || got.Source != ReasoningSourceExplicit || got.Clamped {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestResolvedProfileTracksMatchAndCatalogProvenance(t *testing.T) {
	exact := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{})
	if exact.ProfileMatch != MatchExact || exact.CatalogOverride {
		t.Fatalf("exact provenance = %+v", exact)
	}
	exactGPT := ResolveBuiltin("gateway", "gpt-5.6-sol", CatalogMetadata{})
	if exactGPT.ProfileMatch != MatchExact || exactGPT.CatalogOverride {
		t.Fatalf("GPT exact provenance = %+v", exactGPT)
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
	if got.Provenance.ToolSchemaDialect != MetadataSourceBuiltin {
		t.Fatalf("policy provenance = %+v", got.Provenance)
	}
}

func TestCatalogReasoningProvenanceOverridesOnlyPublishedFields(t *testing.T) {
	yes := true
	got := ResolveBuiltin("gateway", "gemini-3.8-flash", CatalogMetadata{
		Reasoning: &CatalogReasoning{Supported: &yes, Levels: []domain.ReasoningEffort{domain.ReasoningLow}},
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

func TestEffectiveCompactionPolicyScalesWithContextLength(t *testing.T) {
	small := EffectiveCompactionPolicy(Resolved{MaxInputTokens: 128_000})
	large := EffectiveCompactionPolicy(Resolved{MaxInputTokens: 1_050_000})
	if small.SoftThresholdRatio >= large.SoftThresholdRatio {
		t.Fatalf("small soft threshold = %.2f, large = %.2f", small.SoftThresholdRatio, large.SoftThresholdRatio)
	}
	if small.TargetRatio >= large.TargetRatio {
		t.Fatalf("small target = %.2f, large = %.2f", small.TargetRatio, large.TargetRatio)
	}
}

func TestEffectiveCompactionPolicyUsesProfileOverride(t *testing.T) {
	got := EffectiveCompactionPolicy(Resolved{
		MaxInputTokens: 200_000,
		Compaction:     CompactionPolicy{SoftThresholdRatio: 0.75, TargetRatio: 0.58, MinRecentMessages: 12},
	})
	if got.SoftThresholdRatio != 0.75 || got.TargetRatio != 0.58 || got.MinRecentMessages != 12 {
		t.Fatalf("policy = %+v", got)
	}
	if got.MediumThresholdRatio == 0 || got.EmergencyThresholdRatio == 0 {
		t.Fatalf("tier defaults were not retained: %+v", got)
	}
}
