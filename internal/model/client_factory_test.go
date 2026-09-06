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

func TestNewProviderClientRoutesThroughProtonSDK(t *testing.T) {
	providers := []struct {
		name string
		url  string
	}{
		{name: DefaultOpenAIName, url: DefaultOpenAIEndpoint},
		{name: DefaultOpenCodeName, url: DefaultOpenCodeEndpoint},
		{name: DefaultProtonmanName, url: DefaultProtonmanEndpoint},
	}
	for _, provider := range providers {
		client := NewProviderClient(provider.name, string(ProviderProtocolOpenAI), provider.url, "key", "test-model")
		if _, ok := client.(*sdkModelClient); !ok {
			t.Fatalf("%s client type = %T, want *sdkModelClient", provider.name, client)
		}
	}
}

func TestNewProviderClientRoutesAnthropicThroughProtonSDK(t *testing.T) {
	client := NewProviderClient(DefaultAnthropicName, string(ProviderProtocolAnthropic), DefaultAnthropicEndpoint, "key", "claude-test")
	if _, ok := client.(*sdkModelClient); !ok {
		t.Fatalf("Anthropic client type = %T, want *sdkModelClient", client)
	}
}
