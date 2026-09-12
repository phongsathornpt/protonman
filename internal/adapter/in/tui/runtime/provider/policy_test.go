package provider

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func TestKeyPlaceholderFallsBackByRequirement(t *testing.T) {
	optional := model.SupportedProviderPreset{RequiresKey: false}
	if got := KeyPlaceholder(optional); got != "API key (optional)…" {
		t.Fatalf("optional placeholder = %q", got)
	}
	required := model.SupportedProviderPreset{RequiresKey: true}
	if got := KeyPlaceholder(required); got != "API key…" {
		t.Fatalf("required placeholder = %q", got)
	}
}

func TestProtocolPolicy(t *testing.T) {
	if got := ProtocolLabel("", ""); got != "openai · ctrl+r to switch" {
		t.Fatalf("custom protocol label = %q", got)
	}
	if got := ToggleProtocol("openai", ""); got != "anthropic" {
		t.Fatalf("openai toggle = %q", got)
	}
	if got := ToggleProtocol("anthropic", "preset"); got != "anthropic" {
		t.Fatalf("preset toggle changed protocol to %q", got)
	}
}

func TestNormalizePresetID(t *testing.T) {
	cases := map[string]string{
		"1":        model.DefaultProtonmanName,
		"2":        model.DefaultOpenCodeName,
		"3":        model.DefaultOllamaName,
		"4":        model.DefaultOpenAIName,
		"5":        model.DefaultAnthropicName,
		" OpenAI ": model.DefaultOpenAIName,
	}
	for input, want := range cases {
		if got := NormalizePresetID(input); got != want {
			t.Fatalf("NormalizePresetID(%q) = %q, want %q", input, got, want)
		}
	}
}
