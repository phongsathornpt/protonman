package model

import (
	"testing"

	sdk "github.com/phongsathornpt/proton/proton-sdk"
)

func TestResolveProviderBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		configured string
		want       string
	}{
		{name: "configured URL", provider: "openai", configured: "https://proxy.example/v1/", want: "https://proxy.example/v1"},
		{name: "OpenAI default", provider: "openai", want: DefaultOpenAIEndpoint},
		{name: "OpenCode default", provider: "opencode", want: DefaultOpenCodeEndpoint},
		{name: "Protonman default", provider: "protonman", want: DefaultProtonmanEndpoint},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolveProviderBaseURL(test.provider, test.configured); got != test.want {
				t.Fatalf("ResolveProviderBaseURL(%q, %q) = %q, want %q", test.provider, test.configured, got, test.want)
			}
		})
	}
}

func TestResolveProviderBaseURLForProtocolUsesCustomProviderProtocol(t *testing.T) {
	if got := ResolveProviderBaseURLForProtocol("custom-claude", string(ProviderProtocolAnthropic), ""); got != DefaultAnthropicEndpoint {
		t.Fatalf("Anthropic default = %q, want %q", got, DefaultAnthropicEndpoint)
	}
	if got := ResolveProviderBaseURLForProtocol("custom-openai", string(ProviderProtocolOpenAI), ""); got != DefaultOpenAIEndpoint {
		t.Fatalf("OpenAI default = %q, want %q", got, DefaultOpenAIEndpoint)
	}
}

func TestNewProviderLanguageModelSelectsProtocol(t *testing.T) {
	openAIModel := NewProviderLanguageModel(DefaultOpenAIName, string(ProviderProtocolOpenAI), DefaultOpenAIEndpoint, "key", "test-model")
	if openAIModel.Provider() != string(ProviderProtocolOpenAI) {
		t.Fatalf("OpenAI language model provider = %q", openAIModel.Provider())
	}
	anthropicModel := NewProviderLanguageModel(DefaultAnthropicName, string(ProviderProtocolAnthropic), DefaultAnthropicEndpoint, "key", "claude-test")
	if anthropicModel.Provider() != string(ProviderProtocolAnthropic) {
		t.Fatalf("Anthropic language model provider = %q", anthropicModel.Provider())
	}
}

func TestNewProviderLanguageModelOverridesVisionCapability(t *testing.T) {
	withoutVision := NewProviderLanguageModel(DefaultOpenAIName, string(ProviderProtocolOpenAI), DefaultOpenAIEndpoint, "key", "text-only", WithVisionSupport(false))
	if withoutVision.Capabilities().Vision {
		t.Fatalf("Capabilities().Vision = true, want false")
	}
	withVision := NewProviderLanguageModel(DefaultOpenAIName, string(ProviderProtocolOpenAI), DefaultOpenAIEndpoint, "key", "vision-model", WithVisionSupport(true))
	if !withVision.Capabilities().Vision {
		t.Fatalf("Capabilities().Vision = false, want true")
	}
}

func TestNewProviderLanguageModelOverridesToolsCapability(t *testing.T) {
	withoutTools := NewProviderLanguageModel(DefaultOpenAIName, string(ProviderProtocolOpenAI), DefaultOpenAIEndpoint, "key", "text-model", WithToolsSupport(false))
	if withoutTools.Capabilities().Tools {
		t.Fatalf("Capabilities().Tools = true, want false")
	}
	withTools := NewProviderLanguageModel(DefaultOpenAIName, string(ProviderProtocolOpenAI), DefaultOpenAIEndpoint, "key", "tool-model", WithToolsSupport(true))
	if !withTools.Capabilities().Tools {
		t.Fatalf("Capabilities().Tools = false, want true")
	}
}

func TestNewProviderLanguageModelPreservesContextWindowMetadata(t *testing.T) {
	m := NewProviderLanguageModel(DefaultOpenAIName, string(ProviderProtocolOpenAI), DefaultOpenAIEndpoint, "key", "catalog-model",
		WithVisionSupport(false), WithToolsSupport(false), WithContextWindow(12345))
	if got := sdk.ModelContextWindow(m); got != 12345 {
		t.Fatalf("ModelContextWindow() = %d, want 12345", got)
	}
	if m.Capabilities().Vision || m.Capabilities().Tools {
		t.Fatalf("capability overrides were lost: %+v", m.Capabilities())
	}
}

func TestNewProviderLanguageModelAppliesBuiltinModelProfile(t *testing.T) {
	m := NewProviderLanguageModel(DefaultOpenAIName, string(ProviderProtocolOpenAI), DefaultOpenAIEndpoint, "key", "gemini-3.8-flash")
	if got := sdk.ModelContextWindow(m); got != 1_048_576 {
		t.Fatalf("ModelContextWindow() = %d, want 1048576", got)
	}
	profile, ok := ResolvedModelProfile(m)
	if !ok || profile.ProfileName != "gemini-3.8-flash" || len(profile.Reasoning.Levels) != 3 {
		t.Fatalf("ResolvedModelProfile() = %+v, %v", profile, ok)
	}
}

func TestRemoteModelProfileOverridesBuiltinMetadata(t *testing.T) {
	no := false
	remote := RemoteModel{
		ID:            "gemini-3.8-flash",
		ContextWindow: 2048,
		ToolSupport:   &no,
		VisionSupport: &no,
	}
	m := NewProviderLanguageModel(DefaultOpenAIName, string(ProviderProtocolOpenAI), DefaultOpenAIEndpoint, "key", remote.ID, WithRemoteModelProfile(DefaultOpenAIName, remote))
	if m.Capabilities().Tools || m.Capabilities().Vision {
		t.Fatalf("catalog capability overrides were lost: %+v", m.Capabilities())
	}
	if got := sdk.ModelContextWindow(m); got != 2048 {
		t.Fatalf("ModelContextWindow() = %d, want 2048", got)
	}
	profile, ok := ResolvedModelProfile(m)
	if !ok || profile.ContextWindow != 2048 {
		t.Fatalf("ResolvedModelProfile() = %+v, %v", profile, ok)
	}
}

func TestResolveRemoteMetadataUsesResolvedCapabilitiesForDisplay(t *testing.T) {
	no := false
	remote := RemoteModel{
		ID:            "gemini-3.8-flash",
		Features:      []string{"coding", "tools", "vision", "coding"},
		ToolSupport:   &no,
		VisionSupport: &no,
	}
	got := ResolveRemoteMetadata(DefaultProtonmanName, remote)
	if got.Profile.ContextWindow != 1_048_576 {
		t.Fatalf("context window = %d", got.Profile.ContextWindow)
	}
	if len(got.Features) != 2 || got.Features[0] != "coding" || got.Features[1] != "reasoning" {
		t.Fatalf("resolved features = %#v", got.Features)
	}
}

func TestRemoteModelProfilePreservesIndependentTokenLimits(t *testing.T) {
	remote := RemoteModel{ID: "future-model", MaxInputTokens: 200000, MaxOutputTokens: 8192}
	m := NewProviderLanguageModel(DefaultAnthropicName, string(ProviderProtocolAnthropic), DefaultAnthropicEndpoint, "key", remote.ID, WithRemoteModelProfile(DefaultAnthropicName, remote))
	limits := sdk.ModelTokenLimits(m)
	if limits.ContextWindow != 0 || limits.MaxInputTokens != 200000 || limits.MaxOutputTokens != 8192 {
		t.Fatalf("token limits = %+v", limits)
	}
}
