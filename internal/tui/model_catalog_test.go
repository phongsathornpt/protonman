package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
)

func TestModelCatalogStateScopesByProvider(t *testing.T) {
	var state modelCatalogState
	state.set("Provider-A", []model.RemoteModel{{ID: "a-1"}})
	state.set("provider-b", []model.RemoteModel{{ID: "b-1"}})

	if got := state.models("provider-a"); len(got) != 1 || got[0].ID != "a-1" {
		t.Fatalf("provider-a catalog = %#v", got)
	}
	if got := state.models("PROVIDER-B"); len(got) != 1 || got[0].ID != "b-1" {
		t.Fatalf("provider-b catalog = %#v", got)
	}
}

func TestModelCatalogStateReturnsCopies(t *testing.T) {
	var state modelCatalogState
	state.set("provider", []model.RemoteModel{{ID: "original"}})
	got := state.models("provider")
	got[0].ID = "mutated"

	if stored := state.models("provider"); stored[0].ID != "original" {
		t.Fatalf("catalog mutation leaked into state: %#v", stored)
	}
}

func TestModelPickerRejectsStaleProviderResponse(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", APIKey: "a"},
		"beta":  {Name: "beta", APIKey: "b"},
	}
	m.activeProvider = "alpha"
	view := newModelSelectPaneView(m)
	m.bottom.push(view)

	view.fetchRequestID = 2
	view.providerIndex = 1 // beta after sort
	updated, _ := m.Update(modelsFetchedMsg{
		providerName: "alpha",
		requestID:    1,
		models:       []model.RemoteModel{{ID: "stale-alpha"}},
	})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if got := m.modelCatalogs.models("alpha"); len(got) != 0 {
		t.Fatalf("stale alpha response mutated catalog: %#v", got)
	}
	if len(view.models) > 0 && view.models[0].ID == "stale-alpha" {
		t.Fatalf("stale alpha response mutated beta picker: %#v", view.models)
	}
}

func TestModelPickerAcceptsCurrentProviderResponse(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", APIKey: "a"},
		"beta":  {Name: "beta", APIKey: "b"},
	}
	m.activeProvider = "beta"
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	view.fetchRequestID = 3

	updated, _ := m.Update(modelsFetchedMsg{
		providerName: "beta",
		requestID:    3,
		models:       []model.RemoteModel{{ID: "beta-model"}},
	})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if len(view.models) != 1 || view.models[0].ID != "beta-model" {
		t.Fatalf("current response not applied: %#v", view.models)
	}
}

func TestModelPickerLoadingHidesPreviousProviderModels(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", APIKey: "a"},
		"beta":  {Name: "beta", APIKey: "b"},
	}
	m.activeProvider = "alpha"
	m.modelCatalogs.set("alpha", []model.RemoteModel{{ID: "alpha-only", Name: "Alpha Only"}})
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	view.providerIndex = 1
	_ = view.beginFetch(m.ctx, "beta", m.providers["beta"])

	rendered := view.Render(m)
	if !strings.Contains(rendered, "Loading models") {
		t.Fatalf("loading state not rendered: %q", rendered)
	}
	if strings.Contains(rendered, "Alpha Only") {
		t.Fatalf("previous provider model leaked into loading state: %q", rendered)
	}
}

func TestModelPickerRendersFetchError(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSelectPaneView(m)
	view.loading = false
	view.err = errors.New("authentication failed (401)")

	rendered := view.Render(m)
	if !strings.Contains(rendered, "Failed to load models") || !strings.Contains(rendered, "authentication failed") {
		t.Fatalf("error state not rendered: %q", rendered)
	}
}

func TestModelPickerAcceptsEmptyCurrentCatalog(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"alpha": {Name: "alpha", APIKey: "a"}}
	m.activeProvider = "alpha"
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	view.fetchRequestID = 4
	view.loading = true

	updated, _ := m.Update(modelsFetchedMsg{providerName: "alpha", requestID: 4})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.loading || view.err != nil || len(view.models) != 0 {
		t.Fatalf("empty current catalog state = loading:%t err:%v models:%#v", view.loading, view.err, view.models)
	}
}

func TestModelPickerBeginFetchCancelsPreviousRequest(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSelectPaneView(m)
	canceled := false
	view.fetchCancel = func() { canceled = true }

	_ = view.beginFetch(m.ctx, "protonman", config.ProviderConfig{Name: "protonman", APIKey: "key"})
	if !canceled {
		t.Fatal("previous model fetch was not canceled")
	}
	if view.fetchCancel == nil {
		t.Fatal("new model fetch cancel function was not installed")
	}
}

func TestModelPickerCloseCancelsFetch(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	canceled := false
	view.fetchCancel = func() { canceled = true }

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*bubbleModel)
	if !canceled {
		t.Fatal("closing model picker did not cancel fetch")
	}
	if m.bottom.has(modelSelectViewID) {
		t.Fatal("model picker remained open after escape")
	}
}

func TestModelPickerCustomProviderDoesNotUseProtonmanFallback(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"},
	}
	m.activeProvider = "custom"
	view := newModelSelectPaneView(m)
	if len(view.models) != 0 {
		t.Fatalf("custom provider inherited fallback models: %#v", view.models)
	}
}

func TestModelPickerResetSelectionAnchorsActiveModel(t *testing.T) {
	view := &modelSelectPaneView{models: []model.RemoteModel{{ID: "one"}, {ID: "two"}, {ID: "three"}}}
	view.index = 2
	view.offset = 2
	view.resetSelection("two")
	if view.index != 1 || view.offset != 0 {
		t.Fatalf("selection = index:%d offset:%d, want 1/0", view.index, view.offset)
	}
}

func TestModelPickerResetSelectionFallsBackToFirstModel(t *testing.T) {
	view := &modelSelectPaneView{models: []model.RemoteModel{{ID: "one"}, {ID: "two"}}}
	view.index = 1
	view.offset = 1
	view.resetSelection("missing")
	if view.index != 0 || view.offset != 0 {
		t.Fatalf("selection = index:%d offset:%d, want 0/0", view.index, view.offset)
	}
}

func TestModelCatalogFreshness(t *testing.T) {
	var state modelCatalogState
	now := time.Now()
	state.setAt("provider", []model.RemoteModel{{ID: "fresh"}}, now.Add(-time.Minute))
	if got, ok := state.freshModels("provider", now, 2*time.Minute); !ok || len(got) != 1 || got[0].ID != "fresh" {
		t.Fatalf("fresh catalog = %#v, %t", got, ok)
	}
	if got, ok := state.freshModels("provider", now.Add(2*time.Minute), 2*time.Minute); ok || got != nil {
		t.Fatalf("stale catalog reported fresh: %#v, %t", got, ok)
	}
}

func TestModelPickerUsesFreshCacheWithoutFetch(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"},
	}
	m.activeProvider = "custom"
	m.modelCatalogs.set("custom", []model.RemoteModel{{ID: "cached"}})
	view := newModelSelectPaneView(m)
	if cmd := view.loadProvider(m, false); cmd != nil {
		t.Fatal("fresh catalog triggered a network fetch")
	}
	if len(view.models) != 1 || view.models[0].ID != "cached" {
		t.Fatalf("fresh cache not used: %#v", view.models)
	}
}

func TestModelPickerRefreshBypassesFreshCache(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"},
	}
	m.activeProvider = "custom"
	m.modelCatalogs.set("custom", []model.RemoteModel{{ID: "cached"}})
	view := newModelSelectPaneView(m)
	if cmd := view.loadProvider(m, true); cmd == nil {
		t.Fatal("forced refresh did not start a network fetch")
	}
	if !view.loading {
		t.Fatal("forced refresh did not enter loading state")
	}
	view.cancelFetch()
}

func TestDirectModelSelectionMarksUnknownModelUnverified(t *testing.T) {
	t.Setenv("PROTON_HOME", t.TempDir())
	m := newTestSkillsModel(t, 1)
	m.activeProvider = model.DefaultProtonmanName

	cmd := m.selectModelDirect("custom-unlisted-model")
	if cmd == nil {
		t.Fatal("direct model selection returned nil command")
	}
	msg, ok := cmd().(modelSelectedMsg)
	if !ok {
		t.Fatalf("expected modelSelectedMsg")
	}
	if !msg.unverified {
		t.Fatal("unknown model was not marked unverified")
	}
}

func TestDirectModelSelectionRecognizesDiscoveredModel(t *testing.T) {
	t.Setenv("PROTON_HOME", t.TempDir())
	m := newTestSkillsModel(t, 1)
	m.activeProvider = model.DefaultProtonmanName
	m.modelCatalogs.set(model.DefaultProtonmanName, []model.RemoteModel{{ID: "glm-5.3-flash"}})

	cmd := m.selectModelDirect("glm-5.3-flash")
	msg := cmd().(modelSelectedMsg)
	if msg.unverified {
		t.Fatal("discovered model was marked unverified")
	}
}

func TestModelPickerFilterMatchesIDNameVendorAndFeatures(t *testing.T) {
	view := &modelSelectPaneView{}
	view.setModels([]model.RemoteModel{
		{ID: "deepseek-v4", Name: "DeepSeek V4", Provider: "DeepSeek", Features: []string{"tools", "vision"}},
		{ID: "qwen-flash", Name: "Qwen Flash", Provider: "Qwen", Features: []string{"text"}},
	}, "")
	for _, query := range []string{"deepseek-v4", "DeepSeek V4", "deepseek", "vision"} {
		view.filter = query
		view.applyFilter("")
		if len(view.models) != 1 || view.models[0].ID != "deepseek-v4" {
			t.Fatalf("filter %q = %#v", query, view.models)
		}
	}
}

func TestModelPickerFilterCanReturnNoResults(t *testing.T) {
	view := &modelSelectPaneView{}
	view.setModels([]model.RemoteModel{{ID: "one"}, {ID: "two"}}, "")
	view.filter = "missing"
	view.applyFilter("")
	if len(view.models) != 0 || len(view.allModels) != 2 {
		t.Fatalf("filtered/all models = %#v / %#v", view.models, view.allModels)
	}
}

func TestModelPickerSearchModeAcceptsReservedLetters(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSelectPaneView(m)
	m.bottom.push(view)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(*bubbleModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q', 'w', 'e', 'n'}})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.filter != "qwen" {
		t.Fatalf("filter = %q, want qwen", view.filter)
	}
	if !m.bottom.has(modelSelectViewID) {
		t.Fatal("reserved q closed picker while search mode was active")
	}
}

func TestCanonicalSlashNameNormalizesModelAlias(t *testing.T) {
	if got := canonicalSlashName("models"); got != "model" {
		t.Fatalf("canonical name = %q, want model", got)
	}
	if got := canonicalSlashName("MODEL"); got != "model" {
		t.Fatalf("canonical uppercase name = %q, want model", got)
	}
}

func TestActiveRemoteModelFindsSelectedCatalogModel(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = "protonman"
	m.activeModel = "TEXT-ONLY"
	m.modelCatalogs.set("ProtonMan", []model.RemoteModel{{ID: "text-only", Features: []string{"tools"}}})
	got, ok := m.activeRemoteModel()
	if !ok || got.ID != "text-only" {
		t.Fatalf("activeRemoteModel() = %#v, %v", got, ok)
	}
}

func TestFormatModelTokenLimitsRendersIndependentLimits(t *testing.T) {
	got := formatModelTokenLimits(0, 200000, 8192)
	if got != "200K input · 8K output" {
		t.Fatalf("formatModelTokenLimits() = %q", got)
	}
}
