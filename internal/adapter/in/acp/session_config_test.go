package acp

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestSessionConfigOptionsIncludesDiscoveredModels(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	WithSessionRuntimeControls(SessionRuntimeSettings{
		Provider:       "openai",
		Model:          "current-model",
		Reasoning:      "auto",
		LowConcurrency: "auto",
	}, nil)(server)
	WithSessionConfigModelOptions(func(context.Context, SessionRuntimeSettings) ([]SessionConfigSelectOption, error) {
		return []SessionConfigSelectOption{
			{Value: "fast-model", Name: "Fast model", Description: "Low latency"},
			{Value: "deep-model", Name: "Deep model", Description: "More reasoning"},
		}, nil
	})(server)
	WithSessionConfigProviderOptions(func(context.Context, SessionRuntimeSettings) ([]SessionConfigSelectOption, error) {
		return []SessionConfigSelectOption{
			{Value: "openai", Name: "OpenAI"},
			{Value: "anthropic", Name: "Anthropic"},
		}, nil
	})(server)

	sess := &Session{}
	bindSessionRuntime(server, sess)
	options := server.sessionConfigOptions(context.Background(), sess)
	model, ok := findSessionConfigOption(options, configIDModel)
	if !ok {
		t.Fatal("model config option was not advertised")
	}
	if model.CurrentValue != "current-model" {
		t.Fatalf("current model = %q, want current-model", model.CurrentValue)
	}
	if len(model.Options) != 3 {
		t.Fatalf("model options = %d, want discovered models plus current value", len(model.Options))
	}
	if model.Options[0].Value != "current-model" {
		t.Fatalf("first model option = %q, want current-model", model.Options[0].Value)
	}
	provider, ok := findSessionConfigOption(options, configIDProvider)
	if !ok {
		t.Fatal("provider config option was not advertised")
	}
	if provider.CurrentValue != "openai" || len(provider.Options) != 2 {
		t.Fatalf("provider option = %#v, want openai with two values", provider)
	}
}
