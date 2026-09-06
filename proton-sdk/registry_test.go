package protonsdk

import (
	"context"
	"testing"
)

type registryModel struct{ id string }

func (*registryModel) Provider() string                                { return "test" }
func (m *registryModel) ModelID() string                               { return m.id }
func (*registryModel) Capabilities() ModelCapabilities                 { return ModelCapabilities{} }
func (*registryModel) Stream(context.Context, Request) (Stream, error) { return nil, nil }

func TestRegistryResolvesScopedModel(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("Test", func(id string) LanguageModel { return &registryModel{id: id} }); err != nil {
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
	registry := NewRegistry()
	if _, err := registry.Model("model-a"); err == nil {
		t.Fatal("expected invalid scoped model error")
	}
	if _, err := registry.Model("missing/model-a"); err == nil {
		t.Fatal("expected missing provider error")
	}
}
