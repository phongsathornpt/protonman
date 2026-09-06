package model

import (
	"testing"

	sdk "github.com/projectTHORN/proton/proton-sdk"
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
