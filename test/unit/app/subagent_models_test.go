package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/adapter/out/config"
	"github.com/projectTHORN/proton/internal/app"
	"github.com/projectTHORN/proton/internal/feature/agent"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type subagentFallbackModel struct{ id string }

func (m subagentFallbackModel) Provider() string { return "fallback" }
func (m subagentFallbackModel) ModelID() string  { return m.id }
func (subagentFallbackModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Tools: true}
}
func (subagentFallbackModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	return nil, nil
}

func TestBuildSubagentModelResolverBuildsConfiguredOverride(t *testing.T) {
	resolver, err := app.BuildSubagentModelResolver(app.SubagentModelResolverSpec{
		Providers: map[string]config.ProviderConfig{
			"openai": {Type: "openai", APIKey: "test-key"},
		},
		Overrides: map[string]config.SubagentModelConfig{
			"strength": {Provider: "OPENAI", Model: "gpt-test-coder"},
		},
		SessionID: "session-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolver == nil {
		t.Fatal("resolver is nil")
	}
	fallback := subagentFallbackModel{id: "fallback-model"}
	resolved := resolver.Resolve(agent.ProfileStrength, fallback)
	if resolved.ModelID() != "gpt-test-coder" {
		t.Fatalf("resolved model = %q, want gpt-test-coder", resolved.ModelID())
	}
	inherited := resolver.Resolve(agent.ProfileAgility, fallback)
	if inherited.ModelID() != fallback.id {
		t.Fatalf("inherited model = %q, want %q", inherited.ModelID(), fallback.id)
	}
}

func TestBuildSubagentModelResolverReturnsNilWithoutOverrides(t *testing.T) {
	resolver, err := app.BuildSubagentModelResolver(app.SubagentModelResolverSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if resolver != nil {
		t.Fatalf("resolver = %#v, want nil", resolver)
	}
}
func TestBuildSubagentModelResolverRejectsMissingProvider(t *testing.T) {
	_, err := app.BuildSubagentModelResolver(app.SubagentModelResolverSpec{
		Overrides: map[string]config.SubagentModelConfig{
			"intelligence": {Provider: "missing", Model: "reasoner"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "provider \"missing\" is not configured") {
		t.Fatalf("error = %v, want missing provider error", err)
	}
}

func TestBuildSubagentModelResolverRejectsMissingCredentials(t *testing.T) {
	_, err := app.BuildSubagentModelResolver(app.SubagentModelResolverSpec{
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Type: "anthropic"},
		},
		Overrides: map[string]config.SubagentModelConfig{
			"intelligence": {Provider: "anthropic", Model: "claude-test"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "requires credentials") {
		t.Fatalf("error = %v, want credentials error", err)
	}
}
