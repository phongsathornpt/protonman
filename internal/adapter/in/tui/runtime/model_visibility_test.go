package runtime

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func testOpenCodeCatalog() []model.RemoteModel {
	return []model.RemoteModel{
		{ID: "nemotron-3.5-lightning-free"},
		{ID: "big-pickle"},
		{ID: "claude-sonnet-5"},
	}
}

func TestModelSetupOpenCodeWithoutKeyShowsFreeOnly(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: model.DefaultOpenCodeEndpoint}}
	m.activeProvider = "opencode"
	m.modelCatalogs.Set("opencode", testOpenCodeCatalog())
	view := newModelSetupPaneView(m)
	if got := len(view.allModels); got != 2 {
		t.Fatalf("visible OpenCode models = %d, want 2 free models", got)
	}
	for _, md := range view.allModels {
		if !model.IsFreeModel(md.ID) {
			t.Fatalf("paid model %q visible without OpenCode key", md.ID)
		}
	}
}

func TestModelSetupOpenCodeWithKeyShowsAll(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.providers = map[string]config.ProviderConfig{"opencode": {
		Name: "opencode", BaseURL: model.DefaultOpenCodeEndpoint, APIKey: "oc-key",
	}}
	m.activeProvider = "opencode"
	m.modelCatalogs.Set("opencode", testOpenCodeCatalog())
	view := newModelSetupPaneView(m)
	if got := len(view.allModels); got != len(testOpenCodeCatalog()) {
		t.Fatalf("visible OpenCode models = %d, want all %d", got, len(testOpenCodeCatalog()))
	}
}

func TestProviderEditorOpenCodeWithKeyShowsAll(t *testing.T) {
	view := newProviderPaneViewWithConfig(config.ProviderConfig{
		Name: "opencode", BaseURL: model.DefaultOpenCodeEndpoint, APIKey: "oc-key",
	})
	view.setFetchedModels(testOpenCodeCatalog())
	if view.filterFreeOnly {
		t.Fatal("OpenCode key should disable forced free-only filtering")
	}
	if got := len(view.currentModels()); got != len(testOpenCodeCatalog()) {
		t.Fatalf("provider editor models = %d, want all %d", got, len(testOpenCodeCatalog()))
	}
}
