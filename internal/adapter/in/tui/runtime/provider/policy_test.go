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
