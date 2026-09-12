package modelconfig

import (
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestModelConfigTypes(t *testing.T) {
	provider := Provider{
		Name:    "openrouter",
		Type:    "openai",
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "key-123",
	}
	if provider.Name != "openrouter" || provider.Type != "openai" {
		t.Errorf("Provider struct fields mismatch: %+v", provider)
	}

	selection := Selection{
		Default:  "anthropic/claude-3.5-sonnet",
		Provider: "openrouter",
	}
	if selection.Default != "anthropic/claude-3.5-sonnet" || selection.Provider != "openrouter" {
		t.Errorf("Selection struct fields mismatch: %+v", selection)
	}

	route := SubagentRoute{
		Provider:        "openrouter",
		Model:           "openai/gpt-4o",
		ReasoningEffort: sdk.ReasoningHigh,
	}
	if route.Model != "openai/gpt-4o" || route.ReasoningEffort != sdk.ReasoningHigh {
		t.Errorf("SubagentRoute struct fields mismatch: %+v", route)
	}
}
