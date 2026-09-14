package usecase_test

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

type mockModel struct {
	provider string
	modelID  string
}

func (m *mockModel) Provider() string                      { return m.provider }
func (m *mockModel) ModelID() string                       { return m.modelID }
func (m *mockModel) Capabilities() domain.ModelCapabilities { return domain.ModelCapabilities{} }
func (m *mockModel) Stream(ctx context.Context, req domain.Request) (port.Stream, error) {
	return nil, nil
}

func TestRegistry(t *testing.T) {
	reg := usecase.NewRegistry()
	err := reg.Register("openai", func(modelID string) port.LanguageModel {
		return &mockModel{provider: "openai", modelID: modelID}
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	model, err := reg.Model("openai/gpt-4")
	if err != nil {
		t.Fatalf("model lookup failed: %v", err)
	}
	if model.Provider() != "openai" || model.ModelID() != "gpt-4" {
		t.Fatalf("unexpected model: %s/%s", model.Provider(), model.ModelID())
	}

	if _, err := reg.Model("unknown/model"); err == nil {
		t.Fatal("unknown provider should fail")
	}
	if _, err := reg.Model("invalidformat"); err == nil {
		t.Fatal("invalid format should fail")
	}

	if err := reg.Register("", nil); err == nil {
		t.Fatal("empty provider registration should fail")
	}
}

func TestWrapLanguageModel(t *testing.T) {
	base := &mockModel{provider: "openai", modelID: "gpt-4"}
	called := false
	mw := port.MiddlewareFunc(func(next port.StreamFunc) port.StreamFunc {
		return func(ctx context.Context, req domain.Request) (port.Stream, error) {
			called = true
			return next(ctx, req)
		}
	})

	wrapped := usecase.WrapLanguageModel(base, mw)
	if wrapped.Provider() != "openai" || wrapped.ModelID() != "gpt-4" {
		t.Fatalf("identity mismatch: %s/%s", wrapped.Provider(), wrapped.ModelID())
	}
	_, _ = wrapped.Stream(context.Background(), domain.Request{})
	if !called {
		t.Fatal("middleware was not called")
	}
}
