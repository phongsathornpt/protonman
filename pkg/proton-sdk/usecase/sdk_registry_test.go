package usecase_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
)

type registryModel struct{ id string }

func (*registryModel) Provider() string                                            { return "test" }
func (m *registryModel) ModelID() string                                           { return m.id }
func (*registryModel) Capabilities() domain.ModelCapabilities                      { return domain.ModelCapabilities{} }
func (*registryModel) Stream(context.Context, domain.Request) (port.Stream, error) { return nil, nil }

func TestRegistryResolvesScopedModel(t *testing.T) {
	registry := usecase.NewRegistry()
	if err := registry.Register("Test", func(id string) port.LanguageModel { return &registryModel{id: id} }); err != nil {
		t.Fatal(err)
	}
	model, err := registry.Model("test/model-a")
	if err != nil {
		t.Fatal(err)
	}
	if model.Provider() != "test" || model.ModelID() != "model-a" {
		t.Fatalf("unexpected model: provider=%q id=%q", model.Provider(), model.ModelID())
	}
}

func TestRegistryRejectsInvalidScopedModel(t *testing.T) {
	registry := usecase.NewRegistry()
	if _, err := registry.Model("model-a"); err == nil {
		t.Fatal("expected invalid scoped model error")
	}
	if _, err := registry.Model("missing/model-a"); err == nil {
		t.Fatal("expected missing provider error")
	}
}

type middlewareTestModel struct{ calls *[]string }

func (*middlewareTestModel) Provider() string { return "test" }
func (*middlewareTestModel) ModelID() string  { return "model" }
func (*middlewareTestModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true, Tools: true}
}
func (*middlewareTestModel) TokenLimits() domain.TokenLimits {
	return domain.TokenLimits{ContextWindow: 128000, MaxInputTokens: 120000, MaxOutputTokens: 8000}
}
func (*middlewareTestModel) ContextWindow() int { return 128000 }
func (m *middlewareTestModel) Stream(context.Context, domain.Request) (port.Stream, error) {
	*m.calls = append(*m.calls, "model")
	return &eventStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}}, nil
}

func TestWrapLanguageModelOrder(t *testing.T) {
	var calls []string
	makeMiddleware := func(name string) port.Middleware {
		return port.MiddlewareFunc(func(next port.StreamFunc) port.StreamFunc {
			return func(ctx context.Context, request domain.Request) (port.Stream, error) {
				calls = append(calls, name+":before")
				stream, err := next(ctx, request)
				calls = append(calls, name+":after")
				return stream, err
			}
		})
	}
	model := usecase.WrapLanguageModel(&middlewareTestModel{calls: &calls}, makeMiddleware("first"), makeMiddleware("second"))
	if model.Provider() != "test" || model.ModelID() != "model" {
		t.Fatalf("identity changed: %q/%q", model.Provider(), model.ModelID())
	}
	if caps := model.Capabilities(); !caps.Streaming || !caps.Tools {
		t.Fatalf("capabilities changed: %#v", caps)
	}
	wantLimits := domain.TokenLimits{ContextWindow: 128000, MaxInputTokens: 120000, MaxOutputTokens: 8000}
	if got := usecase.ModelContextWindow(model); got != wantLimits.ContextWindow {
		t.Fatalf("context window changed through middleware: got %d, want %d", got, wantLimits.ContextWindow)
	}
	if got := usecase.ModelTokenLimits(model); got != wantLimits {
		t.Fatalf("token limits changed through middleware: got %#v", got)
	}
	if got := usecase.ModelMetadataOf(model).TokenLimits; got != wantLimits {
		t.Fatalf("metadata changed through middleware: got %#v", got)
	}
	if _, err := model.Stream(context.Background(), domain.Request{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"first:before", "second:before", "model", "second:after", "first:after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}
