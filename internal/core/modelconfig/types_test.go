package modelconfig

import (
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestModelConfigTypes(t *testing.T) {
	provider := Provider{Name: "openrouter", Type: "openai", BaseURL: "https://openrouter.ai/api/v1", APIKey: "key-123"}
	if provider.Name != "openrouter" || provider.Type != "openai" {
		t.Errorf("Provider struct fields mismatch: %+v", provider)
	}

	selection := Selection{Default: "anthropic/claude-3.5-sonnet", Provider: "openrouter"}
	if selection.Default != "anthropic/claude-3.5-sonnet" || selection.Provider != "openrouter" {
		t.Errorf("Selection struct fields mismatch: %+v", selection)
	}

	route := SubagentRoute{Provider: "openrouter", Model: "openai/gpt-4o", ReasoningEffort: sdk.ReasoningHigh}
	if route.Model != "openai/gpt-4o" || route.ReasoningEffort != sdk.ReasoningHigh {
		t.Errorf("SubagentRoute struct fields mismatch: %+v", route)
	}
}

func TestRouteIdentifiersAreDistinctTypes(t *testing.T) {
	var provider ProviderName = "anthropic"
	var model ModelID = "claude-sonnet"

	if !provider.Valid() || provider.String() != "anthropic" {
		t.Fatalf("provider = %q, want valid anthropic", provider)
	}
	if !model.Valid() || model.String() != "claude-sonnet" {
		t.Fatalf("model = %q, want valid claude-sonnet", model)
	}
}

func TestRouteIdentifiersRejectEmptyValues(t *testing.T) {
	if ProviderName("").Valid() {
		t.Fatal("empty provider name should be invalid")
	}
	if ModelID("").Valid() {
		t.Fatal("empty model ID should be invalid")
	}
}

func TestRouteIdentifiersTrimAndValidateInput(t *testing.T) {
	provider, err := ParseProviderName("  anthropic ")
	if err != nil || provider != "anthropic" {
		t.Fatalf("provider=%q err=%v", provider, err)
	}
	model, err := ParseModelID("  claude-sonnet ")
	if err != nil || model != "claude-sonnet" {
		t.Fatalf("model=%q err=%v", model, err)
	}
	if _, err := ParseProviderName(" "); err == nil {
		t.Fatal("expected empty provider error")
	}
	if _, err := ParseModelID(" "); err == nil {
		t.Fatal("expected empty model error")
	}
}
