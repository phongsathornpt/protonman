package modelconfig

import (
	"testing"

	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
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

	route := SubagentRoute{Provider: "openrouter", Model: "openai/gpt-4o", ReasoningEffort: domain.ReasoningHigh}
	if route.Model != "openai/gpt-4o" || route.ReasoningEffort != domain.ReasoningHigh {
		t.Errorf("SubagentRoute struct fields mismatch: %+v", route)
	}
}

func TestRouteIdentifiersAreDistinctTypes(t *testing.T) {
	var provider ProviderName = "anthropic"
	var model ModelID = "claude-sonnet"

	if !provider.Valid() || provider.String() != "anthropic" {
		t.Fatalf("provider = %q, want valid anthropic", provider)
	}
	if model.String() != "claude-sonnet" {
		t.Fatalf("model = %q, want claude-sonnet", model)
	}
}

func TestRouteIdentifierRejectsEmptyProvider(t *testing.T) {
	if ProviderName("").Valid() {
		t.Fatal("empty provider name should be invalid")
	}
}
