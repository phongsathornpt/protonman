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

func TestNewProviderClientSelectsOfficialOnlyForOpenAI(t *testing.T) {
	official := NewProviderClient("openai", DefaultOpenAIEndpoint, "key", "gpt-4o")
	if _, ok := official.(*OfficialOpenAIClient); !ok {
		t.Fatalf("OpenAI client type = %T, want *OfficialOpenAIClient", official)
	}

	compatible := NewProviderClient(DefaultOpenCodeName, DefaultOpenCodeEndpoint, "", "muse-spark-1.3-contributor-free")
	if _, ok := compatible.(*OpenAIClient); !ok {
		t.Fatalf("OpenCode client type = %T, want *OpenAIClient", compatible)
	}
}
