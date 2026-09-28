package modelclient

import (
	"context"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	port "github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
)

type mockLanguageModel struct {
	modelID  string
	provider string
}

func (m *mockLanguageModel) ModelID() string  { return m.modelID }
func (m *mockLanguageModel) Provider() string { return m.provider }
func (m *mockLanguageModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}
func (m *mockLanguageModel) Stream(ctx context.Context, req domain.Request) (port.Stream, error) {
	return nil, nil
}

type mockFactory struct{}

func (f *mockFactory) Build(req Request) port.LanguageModel {
	return &mockLanguageModel{
		modelID:  req.ModelID,
		provider: req.ProviderName,
	}
}

func TestModelClientFactory(t *testing.T) {
	var factory Factory = &mockFactory{}

	req := Request{
		ProviderName:   "anthropic",
		ProviderType:   "anthropic",
		BaseURL:        "https://api.anthropic.com",
		APIKey:         "secret-key",
		ModelID:        "claude-3-5-sonnet",
		SessionID:      "sess-123",
		AgentProfile:   "general",
		RequestTimeout: 30 * time.Second,
		RemoteModel: &modelcatalog.RemoteModel{
			ID:   "claude-3-5-sonnet",
			Name: "Claude 3.5 Sonnet",
		},
		LowConcurrency: modelconfig.LowConcurrencyAuto,
	}

	model := factory.Build(req)
	if model == nil {
		t.Fatal("factory.Build returned nil")
	}
	if model.ModelID() != "claude-3-5-sonnet" {
		t.Errorf("ModelID = %q; want claude-3-5-sonnet", model.ModelID())
	}
	if model.Provider() != "anthropic" {
		t.Errorf("Provider = %q; want anthropic", model.Provider())
	}
	if !model.Capabilities().Streaming {
		t.Error("Streaming capability should be true")
	}
}
