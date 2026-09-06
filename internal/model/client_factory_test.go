package model

import "testing"

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
