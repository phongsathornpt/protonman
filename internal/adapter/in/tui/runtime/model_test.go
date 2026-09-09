package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelpicker"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	domainmodel "github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"strings"
	"testing"
	"time"
)

func TestModelCatalogDeleteReleasesProviderEntry(t *testing.T) {
	var catalogs modelcatalog.State
	catalogs.Set("Alpha", []model.RemoteModel{{ID: "large-model", Name: strings.Repeat("x", 4096)}})
	if catalogs.Len() != 1 {
		t.Fatalf("entries before delete = %d, want 1", catalogs.Len())
	}
	catalogs.Delete(" alpha ")
	if catalogs.Len() != 0 {
		t.Fatalf("entries after delete = %d, want 0", catalogs.Len())
	}
	if got := catalogs.Models("alpha"); len(got) != 0 {
		t.Fatalf("deleted provider models = %#v, want none", got)
	}
}

func TestModelCatalogStateScopesByProvider(t *testing.T) {
	var state modelcatalog.State
	state.Set("Provider-A", []model.RemoteModel{{ID: "a-1"}})
	state.Set("provider-b", []model.RemoteModel{{ID: "b-1"}})
	if got := state.Models("provider-a"); len(got) != 1 || got[0].ID != "a-1" {
		t.Fatalf("provider-a catalog = %#v", got)
	}
	if got := state.Models("PROVIDER-B"); len(got) != 1 || got[0].ID != "b-1" {
		t.Fatalf("provider-b catalog = %#v", got)
	}
}

func TestModelCatalogStateReturnsCopies(t *testing.T) {
	var state modelcatalog.State
	state.Set("provider", []model.RemoteModel{{ID: "original"}})
	got := state.Models("provider")
	got[0].ID = "mutated"
	if stored := state.Models("provider"); stored[0].ID != "original" {
		t.Fatalf("catalog mutation leaked into state: %#v", stored)
	}
}

func TestModelPickerRejectsStaleProviderResponse(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"alpha": {Name: "alpha", APIKey: "a"}, "beta": {Name: "beta", APIKey: "b"}}
	m.activeProvider = "alpha"
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	view.fetchRequestID = 2
	view.providerIndex = 1
	updated, _ := m.Update(modelsFetchedMsg{providerName: "alpha", requestID: 1, models: []model.RemoteModel{{ID: "stale-alpha"}}})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if got := m.modelCatalogs.Models("alpha"); len(got) != 0 {
		t.Fatalf("stale alpha response mutated catalog: %#v", got)
	}
	if len(view.models) > 0 && view.models[0].ID == "stale-alpha" {
		t.Fatalf("stale alpha response mutated beta picker: %#v", view.models)
	}
}

func TestModelPickerAcceptsCurrentProviderResponse(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"alpha": {Name: "alpha", APIKey: "a"}, "beta": {Name: "beta", APIKey: "b"}}
	m.activeProvider = "beta"
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	view.fetchRequestID = 3
	updated, _ := m.Update(modelsFetchedMsg{providerName: "beta", requestID: 3, models: []model.RemoteModel{{ID: "beta-model"}}})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if len(view.models) != 1 || view.models[0].ID != "beta-model" {
		t.Fatalf("current response not applied: %#v", view.models)
	}
}

func TestModelPickerLoadingHidesPreviousProviderModels(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"alpha": {Name: "alpha", APIKey: "a"}, "beta": {Name: "beta", APIKey: "b"}}
	m.activeProvider = "alpha"
	m.modelCatalogs.Set("alpha", []model.RemoteModel{{ID: "alpha-only", Name: "Alpha Only"}})
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
	view.fetchCancel = func() {
		canceled = true
	}
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
	view.fetchCancel = func() {
		canceled = true
	}
	updated, _ := m.Update(testKey(tea.KeyEsc))
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
	m.providers = map[string]config.ProviderConfig{"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"}}
	m.activeProvider = "custom"
	view := newModelSelectPaneView(m)
	if len(view.models) != 0 {
		t.Fatalf("custom provider inherited fallback models: %#v", view.models)
	}
}

func TestModelPickerResetSelectionAnchorsActiveModel(t *testing.T) {
	view := &modelSelectPaneView{models: []model.RemoteModel{{ID: "one"}, {ID: "two"}, {ID: "three"}}}
	view.resetSelection("")
	view.picker.Select(2)
	view.resetSelection("two")
	if view.picker.Index() != 1 || (view.picker.Paginator.Page*view.picker.Paginator.PerPage) != 0 {
		t.Fatalf("selection = index:%d offset:%d, want 1/0", view.picker.Index(), (view.picker.Paginator.Page * view.picker.Paginator.PerPage))
	}
}

func TestModelPickerResetSelectionFallsBackToFirstModel(t *testing.T) {
	view := &modelSelectPaneView{models: []model.RemoteModel{{ID: "one"}, {ID: "two"}}}
	view.resetSelection("")
	view.picker.Select(1)
	view.resetSelection("missing")
	if view.picker.Index() != 0 || (view.picker.Paginator.Page*view.picker.Paginator.PerPage) != 0 {
		t.Fatalf("selection = index:%d offset:%d, want 0/0", view.picker.Index(), (view.picker.Paginator.Page * view.picker.Paginator.PerPage))
	}
}

func TestModelCatalogFreshness(t *testing.T) {
	var state modelcatalog.State
	now := time.Now()
	state.SetAt("provider", []model.RemoteModel{{ID: "fresh"}}, now.Add(-time.Minute))
	if got, ok := state.FreshModels("provider", now, 2*time.Minute); !ok || len(got) != 1 || got[0].ID != "fresh" {
		t.Fatalf("fresh catalog = %#v, %t", got, ok)
	}
	if got, ok := state.FreshModels("provider", now.Add(2*time.Minute), 2*time.Minute); ok || got != nil {
		t.Fatalf("stale catalog reported fresh: %#v, %t", got, ok)
	}
}

func TestModelPickerUsesFreshCacheWithoutFetch(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"}}
	m.activeProvider = "custom"
	m.modelCatalogs.Set("custom", []model.RemoteModel{{ID: "cached"}})
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
	m.providers = map[string]config.ProviderConfig{"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"}}
	m.activeProvider = "custom"
	m.modelCatalogs.Set("custom", []model.RemoteModel{{ID: "cached"}})
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
	t.Setenv("PROTONMAN_HOME", t.TempDir())
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
	t.Setenv("PROTONMAN_HOME", t.TempDir())
	m := newTestSkillsModel(t, 1)
	m.activeProvider = model.DefaultProtonmanName
	m.modelCatalogs.Set(model.DefaultProtonmanName, []model.RemoteModel{{ID: "glm-5.3-flash"}})
	cmd := m.selectModelDirect("glm-5.3-flash")
	msg := cmd().(modelSelectedMsg)
	if msg.unverified {
		t.Fatal("discovered model was marked unverified")
	}
}

func TestModelPickerFilterMatchesIDNameVendorAndFeatures(t *testing.T) {
	view := &modelSelectPaneView{}
	view.setModels([]model.RemoteModel{{ID: "deepseek-v4", Name: "DeepSeek V4", Provider: "DeepSeek", Features: []string{"tools", "vision"}}, {ID: "qwen-flash", Name: "Qwen Flash", Provider: "Qwen", Features: []string{"text"}}}, "")
	for _, query := range []string{"deepseek-v4", "DeepSeek V4", "deepseek", "vision"} {
		view.picker.SetFilterText(query)
		view.syncPickerProjection()
		if len(view.models) != 1 || view.models[0].ID != "deepseek-v4" {
			t.Fatalf("filter %q = %#v", query, view.models)
		}
	}
}

func TestModelPickerFilterCanReturnNoResults(t *testing.T) {
	view := &modelSelectPaneView{}
	view.setModels([]model.RemoteModel{{ID: "one"}, {ID: "two"}}, "")
	view.picker.SetFilterText("missing")
	view.syncPickerProjection()
	if len(view.models) != 0 || len(view.allModels) != 2 {
		t.Fatalf("filtered/all models = %#v / %#v", view.models, view.allModels)
	}
}

func TestModelPickerSearchModeAcceptsReservedLetters(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	updated, _ := m.Update(testText("/"))
	m = updated.(*bubbleModel)
	updated, _ = m.Update(testText("qwen"))
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.picker.FilterValue() != "qwen" {
		t.Fatalf("filter = %q, want qwen", view.picker.FilterValue())
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
	m.modelCatalogs.Set("ProtonMan", []model.RemoteModel{{ID: "text-only", Features: []string{"tools"}}})
	got, ok := m.activeRemoteModel()
	if !ok || got.ID != "text-only" {
		t.Fatalf("activeRemoteModel() = %#v, %v", got, ok)
	}
}

func TestFormatModelTokenLimitsRendersIndependentLimits(t *testing.T) {
	got := modelpicker.FormatTokenLimits(0, 200000, 8192)
	if got != "200K input · 8K output" {
		t.Fatalf("modelpicker.FormatTokenLimits() = %q", got)
	}
}

func seedModelSelectCatalog(m *bubbleModel) {
	m.modelCatalogs.Set(model.DefaultProtonmanName, []model.RemoteModel{{ID: "deepseek-v4-flash-vision-exp", Name: "DeepSeek V4 Flash Vision"}, {ID: "glm-5.3-flash", Name: "GLM 5.3 Flash"}, {ID: "Qwen3.8-Flash", Name: "Qwen 3.8 Flash"}, {ID: "muse-spark", Name: "Muse Spark"}, {ID: "MiniMax-M3", Name: "MiniMax M3"}, {ID: "fixture-six", Name: "Fixture Six"}})
}

func TestModelSelectViewLaunchViaSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	seedModelSelectCatalog(bModel)
	bModel.activeModel = "MiniMax-M3"
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/model")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after /model")
	}
	view := bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if len(view.models) == 0 {
		t.Fatal("expected models in catalog")
	}
	if view.models[view.picker.Index()].ID != "MiniMax-M3" {
		t.Fatalf("expected focused model 'MiniMax-M3', got %s", view.models[view.picker.Index()].ID)
	}
	rendered := bModel.View().Content
	if !strings.Contains(rendered, "Select Model") {
		t.Fatalf("expected 'Select Model' in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "✓") {
		t.Fatalf("expected active model checkmark in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "MiniMax-M3") {
		t.Fatalf("expected 'MiniMax-M3' in view, got:\n%s", rendered)
	}
	updated, _ := bModel.Update(testKey(tea.KeyEsc))
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal closed after Esc")
	}
	bModel.executeCommand("/models")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after /models")
	}
	bModel.bottom.remove(modelSelectViewID)
	bModel.executeCommand("/model select")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after /model select")
	}
}

func TestModelSelectViewToggleKeybinding(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	updated, _ := bModel.Update(testCtrl('p'))
	bModel = updated.(*bubbleModel)
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after Ctrl+P")
	}
	updated, _ = bModel.Update(testCtrl('p'))
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal closed after second Ctrl+P")
	}
	updated, _ = bModel.Update(testAltText("m"))
	bModel = updated.(*bubbleModel)
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after Alt+M")
	}
}

func TestModelSelectViewNavigationAndConfirm(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	seedModelSelectCatalog(bModel)
	bModel.activeModel = "deepseek-v4-flash-vision-exp"
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/model")
	view := bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.picker.Index() != 0 {
		t.Fatalf("expected initial index 0, got %d", view.picker.Index())
	}
	updated, _ := bModel.Update(testText("j"))
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.picker.Index() != 1 {
		t.Fatalf("expected index 1 after 'j', got %d", view.picker.Index())
	}
	updated, _ = bModel.Update(testText("k"))
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.picker.Index() != 0 {
		t.Fatalf("expected index 0 after 'k', got %d", view.picker.Index())
	}
	updated, _ = bModel.Update(testKey(tea.KeyDown))
	bModel = updated.(*bubbleModel)
	updated, _ = bModel.Update(testKey(tea.KeyDown))
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.picker.Index() != 2 {
		t.Fatalf("expected index 2 after moving down twice, got %d", view.picker.Index())
	}
	if view.models[view.picker.Index()].ID != "Qwen3.8-Flash" {
		t.Fatalf("expected Qwen3.8-Flash at index 2, got %s", view.models[view.picker.Index()].ID)
	}
	t.Setenv("PROTONMAN_HOME", t.TempDir())
	updated, cmd := bModel.Update(testKey(tea.KeyEnter))
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected modelSelectViewID removed on Enter")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd on Enter")
	}
	msg := cmd()
	selectedMsg, ok := msg.(modelSelectedMsg)
	if !ok {
		t.Fatalf("expected modelSelectedMsg, got %T", msg)
	}
	if selectedMsg.modelID != "Qwen3.8-Flash" {
		t.Fatalf("expected selected model 'Qwen3.8-Flash', got: %s", selectedMsg.modelID)
	}
	bModel.providers = map[string]config.ProviderConfig{"protonman": {Name: "protonman", APIKey: "plk_test_123"}}
	updated, _ = bModel.Update(selectedMsg)
	bModel = updated.(*bubbleModel)
	if bModel.activeModel != "Qwen3.8-Flash" {
		t.Fatalf("expected activeModel updated to 'Qwen3.8-Flash', got: %s", bModel.activeModel)
	}
	if bModel.runner == nil {
		t.Fatal("expected runner to be configured after model selection with valid API key")
	}
}

func TestModelSelectViewDirectModelCommand(t *testing.T) {
	t.Setenv("PROTONMAN_HOME", t.TempDir())
	bModel := newTestSkillsModel(t, 1)
	cmd := bModel.executeCommand("/model glm-5.3-flash")
	if cmd == nil {
		t.Fatal("expected cmd from /model <id>")
	}
	msg := cmd()
	selectedMsg, ok := msg.(modelSelectedMsg)
	if !ok {
		t.Fatalf("expected modelSelectedMsg, got %T", msg)
	}
	if selectedMsg.modelID != "glm-5.3-flash" {
		t.Fatalf("expected modelID 'glm-5.3-flash', got: %s", selectedMsg.modelID)
	}
	updated, _ := bModel.Update(selectedMsg)
	bModel = updated.(*bubbleModel)
	if bModel.activeModel != "glm-5.3-flash" {
		t.Fatalf("expected activeModel 'glm-5.3-flash', got: %s", bModel.activeModel)
	}
}

func TestModelSelectViewSwitchToAddProvider(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open")
	}
	updated, _ := bModel.Update(testText("a"))
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected modelSelectViewID removed after 'a'")
	}
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected providerViewID added after 'a'")
	}
}

func TestModelSelectInfoViewAndWelcome(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.activeModel = "deepseek-v4-flash-vision-exp"
	bModel.activeProvider = "protonman"
	info := bModel.infoView()
	if !strings.Contains(info, "deepseek-v4") {
		t.Fatalf("expected model in infoView, got: %s", info)
	}
	if strings.Contains(info, "ctrl+p") {
		t.Fatalf("minimal infoView leaked shortcut chrome: %s", info)
	}
	welcome := bModel.welcomeCard()
	if strings.Contains(welcome, "deepseek-v4-flash-vision-exp") {
		t.Fatalf("welcomeCard duplicated model already shown in status bar: %s", welcome)
	}
}

func TestProviderListSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"protonman": {Name: "protonman", BaseURL: "https://protonman.dev/api/v1"}}
	bModel.activeProvider = "protonman"
	bModel.activeModel = "MiniMax-M3"
	bModel.executeCommand("/provider list")
	rendered := bModel.View().Content
	if !strings.Contains(rendered, "Configured Providers") {
		t.Fatalf("expected 'Configured Providers' in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "https://protonman.dev/api/v1") {
		t.Fatalf("expected endpoint in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[active]") {
		t.Fatalf("expected '[active]' in view, got:\n%s", rendered)
	}
}

func TestModelSelectPagedNavigation(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	seedModelSelectCatalog(m)
	m.resize(40, 14)
	m.executeCommand("/model")
	view := m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	view.picker.Select(0)
	updated, _ := m.Update(testKey(tea.KeyPgDown))
	m = updated.(*bubbleModel)
	if view.picker.Index() <= 0 {
		t.Fatalf("pgdown did not advance selection: index=%d", view.picker.Index())
	}
	updated, _ = m.Update(testKey(tea.KeyEnd))
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.picker.Index() != len(view.models)-1 {
		t.Fatalf("end index = %d, want %d", view.picker.Index(), len(view.models)-1)
	}
	updated, _ = m.Update(testKey(tea.KeyHome))
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.picker.Index() != 0 {
		t.Fatalf("home index = %d, want 0", view.picker.Index())
	}
}

func TestBubbleModelRunsToolCommandThroughService(t *testing.T) {
	registry, handler := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
	model.resize(80, 24)
	model.prompt.SetValue(`:call read {"path":"README.md"}`)
	command := model.submit()
	if command == nil {
		t.Fatal("submit() command = nil, want tool command")
	}
	message := command()
	resultMessage, ok := message.(toolResultMsg)
	if !ok {
		t.Fatalf("tool command message = %T, want toolResultMsg", message)
	}
	updated, _ := model.Update(resultMessage)
	model = updated.(*bubbleModel)
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
	if !strings.Contains(plainTranscript(model), "file contents") {
		t.Fatalf("scrollback does not contain tool output: %#v", model.historyState.Cells())
	}
}

func TestSubmitWhileBusyQueuesDraft(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	model.resize(80, 24)
	model.busy = true
	model.prompt.SetValue(":help")
	if command := model.submit(); command != nil {
		t.Fatalf("busy submit command = %v, want nil", command)
	}
	if got := model.prompt.Value(); got != "" {
		t.Fatalf("busy submit cleared prompt = %q, want empty", got)
	}
	if len(model.queue) != 1 || model.queue[0] != ":help" {
		t.Fatalf("queue = %#v, want [:help]", model.queue)
	}
	if !strings.Contains(plainTranscript(model), "queued (1): :help") {
		t.Fatalf("scrollback missing queue notice: %#v", model.historyState.Cells())
	}
	model.busy = false
	if command := model.drainQueue(); command != nil {
		t.Fatalf("queued :help command = %v, want nil", command)
	}
	if len(model.queue) != 0 {
		t.Fatalf("queue after drain = %#v, want empty", model.queue)
	}
	if !strings.Contains(plainTranscript(model), "/help") {
		t.Fatalf("drained :help did not render: %#v", model.historyState.Cells())
	}
}

func TestAppendTurnResultCoalescesAssistantText(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.appendTurnResult([]applicationturn.Event{{Kind: applicationturn.EventTextDelta, Text: "hello"}, {Kind: applicationturn.EventTextDelta, Text: " world"}}, applicationturn.Result{Message: domainmodel.Message{Content: "hello world"}}, nil)
	plain := plainTranscript(model)
	if strings.Count(plain, "hello world") != 1 {
		t.Fatalf("assistant text count = %d, want 1: %q", strings.Count(plain, "hello world"), plain)
	}
	if strings.Contains(plain, "assistant: hello") {
		t.Fatalf("text deltas were not coalesced: %q", plain)
	}
}

func TestAppendTurnResultPreservesToolNewlines(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.appendTurnResult([]applicationturn.Event{{Kind: applicationturn.EventToolResult, Call: tool.Call{Name: "read"}, Result: tool.Result{Output: "alpha\nbeta\ngamma"}}}, applicationturn.Result{}, nil)
	plain := plainTranscript(model)
	if !strings.Contains(plain, "alpha\nbeta\ngamma") {
		t.Fatalf("tool output newlines were flattened: %q", plain)
	}
}

func TestStartTurnStreamsSinkEvents(t *testing.T) {
	runner := &scriptedRunner{events: []applicationturn.Event{{Kind: applicationturn.EventTextDelta, Text: "hello"}, {Kind: applicationturn.EventTextDelta, Text: " stream"}}, result: applicationturn.Result{Message: domainmodel.Message{Role: domainmodel.RoleAssistant, Content: "hello stream"}}}
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), runner, newPermissionBridge(), "")
	if command := model.startTurn("hi"); command == nil {
		t.Fatal("startTurn command = nil")
	}
	deadline := time.Now().Add(time.Second)
	for model.busy {
		if time.Now().After(deadline) {
			t.Fatal("streamed turn did not finish")
		}
		events := model.turnEvents
		if events == nil {
			t.Fatal("busy turn has no event channel")
		}
		message := turnmsg.Wait(events)()
		updated, _ := model.Update(message)
		model = updated.(*bubbleModel)
	}
	plain := plainTranscript(model)
	if !strings.Contains(plain, "hello stream") {
		t.Fatalf("streamed transcript = %q, want hello stream", plain)
	}
	if model.busy {
		t.Fatal("model still busy after streamed turn")
	}
}

func TestClosedTurnEventsRenderTerminalFailure(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.busy = true
	events := make(chan tea.Msg)
	close(events)
	model.turnEvents = events
	updated, _ := model.Update(turnmsg.EventsClosed{})
	model = updated.(*bubbleModel)
	if model.busy {
		t.Fatal("model remained busy after turn event channel closed")
	}
	if !strings.Contains(plainTranscript(model), "turn event stream closed before completion") {
		t.Fatalf("closed turn channel missing terminal failure: %q", plainTranscript(model))
	}
}

func TestTurnDoneAppendsProducedToolHistory(t *testing.T) {
	runner := &scriptedRunner{result: applicationturn.Result{Message: domainmodel.Message{Role: domainmodel.RoleAssistant, Content: "done"}, Messages: []domainmodel.Message{{Role: domainmodel.RoleAssistant, ToolCalls: []domainmodel.ToolCall{{ID: "call-1", Name: "read", Arguments: []byte(`{"path":"README.md"}`)}}}, {Role: domainmodel.RoleTool, ToolCallID: "call-1", ToolName: "read", Content: `{"output":"ok"}`}, {Role: domainmodel.RoleAssistant, Content: "done"}}}}
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	m := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), runner, newPermissionBridge(), "")
	message := m.startTurn("inspect")()
	updated, _ := m.Update(message)
	m = updated.(*bubbleModel)
	if got, want := len(m.messages), 4; got != want {
		t.Fatalf("provider history length = %d, want %d", got, want)
	}
	if m.messages[1].Role != domainmodel.RoleAssistant || m.messages[2].Role != domainmodel.RoleTool {
		t.Fatalf("provider history = %#v, want assistant/tool exchange", m.messages)
	}
}

func TestBangPrefixSubmitsBashCall(t *testing.T) {
	registry := newNamedTestRegistry(tool.Definition{Name: "bash", Description: "run a shell command", Kind: tool.KindBash, PermissionDetailKey: "command"})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
	model.setBashMode(true)
	model.prompt.SetValue("pwd")
	command := model.submit()
	if command == nil {
		t.Fatal("bash submit command = nil")
	}
	if model.bottom.bashMode() {
		t.Fatal("bash mode stayed on after submit")
	}
	message := command()
	resultMessage, ok := message.(toolResultMsg)
	if !ok {
		t.Fatalf("bash message = %T, want toolResultMsg", message)
	}
	if resultMessage.err != nil {
		t.Fatalf("bash call error = %v", resultMessage.err)
	}
	if registry.handler.calls != 1 {
		t.Fatalf("bash handler calls = %d, want 1", registry.handler.calls)
	}
}

func TestTodoStoreRevisionSyncsAfterToolResult(t *testing.T) {
	initial := []TodoItem{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}}
	store, err := tododomain.NewStore(initial)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestBubbleModel(t, permission.ModeAsk, initial)
	m.todoStore = store
	m.todoRevision = store.Snapshot().Revision
	if _, err := store.Replace(context.Background(), []tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusCompleted}}); err != nil {
		t.Fatal(err)
	}
	m.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventToolResult, Call: tool.Call{ID: "todo-1", Name: "todo"}, Result: tool.Result{CallID: "todo-1", ToolName: "todo"}})
	if len(m.todo) != 1 || m.todo[0].Status != tododomain.StatusCompleted {
		t.Fatalf("todo = %#v", m.todo)
	}
	if m.todoRevision != store.Snapshot().Revision {
		t.Fatalf("revision = %d", m.todoRevision)
	}
	if m.syncTodoSnapshot() {
		t.Fatal("unchanged revision reported a sync")
	}
}

func TestTUIWithCoordinatorOption(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	ws, err := workspace.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	coordinator := agent.NewCoordinator(nil, registry, ws, policy)
	defer func() {
		_ = coordinator.Close()
	}()
	ui, err := NewBubbleTea(service, registry, nil, WithCoordinator(coordinator))
	if err != nil {
		t.Fatalf("NewBubbleTea() error = %v", err)
	}
	if !ui.agents.Available() {
		t.Fatal("expected agent service to be available on BubbleTeaUI")
	}
}

func TestTUICycleModeUpdatesCoordinator(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	ws, err := workspace.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	coordinator := agent.NewCoordinator(nil, model.registry, ws, policy)
	defer func() {
		_ = coordinator.Close()
	}()
	model.agents = app.NewAgents(coordinator)
	if model.planMode {
		t.Fatal("expected planMode initially false")
	}
	model.cycleMode()
	if !model.planMode {
		t.Fatal("expected planMode to be true after first cycle")
	}
	guard := coordinator.CallGuard()
	if guard == nil {
		t.Fatal("expected coordinator call guard to be set in plan mode")
	}
	err = guard(context.Background(), permission.Request{ToolKind: permission.ToolBash, ToolName: "bash"})
	if err == nil {
		t.Fatal("expected plan mode guard to block bash execution")
	}
	err = guard(context.Background(), permission.Request{ToolKind: permission.ToolRead, ToolName: "read"})
	if err != nil {
		t.Fatalf("expected plan mode guard to allow read, got: %v", err)
	}
	model.cycleMode()
	if model.planMode {
		t.Fatal("expected planMode to be false after second cycle")
	}
	if coordinator.CallGuard() != nil {
		t.Fatal("expected coordinator call guard to be cleared")
	}
	if coordinator.PermissionMode() != permission.ModeAlwaysApprove {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAlwaysApprove, coordinator.PermissionMode())
	}
	model.cycleMode()
	if coordinator.PermissionMode() != permission.ModeAsk {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAsk, coordinator.PermissionMode())
	}
}

func TestTUISlashModeUpdatesCoordinator(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	ws, err := workspace.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	coordinator := agent.NewCoordinator(nil, model.registry, ws, policy)
	defer func() {
		_ = coordinator.Close()
	}()
	model.agents = app.NewAgents(coordinator)
	_ = model.executeCommand("/mode always-approve")
	if coordinator.PermissionMode() != permission.ModeAlwaysApprove {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAlwaysApprove, coordinator.PermissionMode())
	}
	_ = model.executeCommand("/mode ask")
	if coordinator.PermissionMode() != permission.ModeAsk {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAsk, coordinator.PermissionMode())
	}
	_ = model.executeCommand("/yolo")
	if coordinator.PermissionMode() != permission.ModeAlwaysApprove {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAlwaysApprove, coordinator.PermissionMode())
	}
	_ = model.executeCommand("/plan on")
	if coordinator.CallGuard() == nil {
		t.Fatal("expected coordinator call guard to be set after /plan on")
	}
	_ = model.executeCommand("/plan off")
	if coordinator.CallGuard() != nil {
		t.Fatal("expected coordinator call guard to be cleared after /plan off")
	}
}

func TestTUIReconfigureRunnerUpdatesCoordinatorClient(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	ws, err := workspace.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	coordinator := agent.NewCoordinator(nil, model.registry, ws, policy)
	defer func() {
		_ = coordinator.Close()
	}()
	model.agents = app.NewAgents(coordinator)
	if coordinator.LanguageModel() != nil {
		t.Fatal("expected coordinator language model initially nil")
	}
	model.activeModel = "test-model"
	model.activeProvider = "openai"
	model.providers = map[string]config.ProviderConfig{"openai": {Name: "openai", APIKey: "sk-test-key", BaseURL: "https://api.openai.com/v1"}}
	model.reconfigureRunner()
	if coordinator.LanguageModel() == nil {
		t.Fatal("expected coordinator language model to be updated after reconfigureRunner()")
	}
}

func TestPlanModeAllowsTaskMetadataButBlocksWorkspaceEdit(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       tool.Kind
		wantDenied bool
	}{{name: "todo", kind: tool.KindTask, wantDenied: false}, {name: "edit", kind: tool.KindEdit, wantDenied: true}} {
		t.Run(tc.name, func(t *testing.T) {
			registry := newNamedTestRegistry(tool.Definition{Name: tc.name, Description: tc.name, Kind: tc.kind, Mutability: tool.MutabilityMutating})
			service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
			m := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "/tmp/proton")
			m.setPlanEnabled(true)
			call, err := tool.NewCall("call", tc.name, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Call(context.Background(), call)
			if tc.wantDenied && err == nil {
				t.Fatal("workspace edit was allowed in plan mode")
			}
			if !tc.wantDenied && err != nil {
				t.Fatalf("task metadata blocked in plan mode: %v", err)
			}
		})
	}
}

func TestModelSelectReconcilesIncompatibleReasoningEffort(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.activeProvider = "openai"
	bModel.activeModel = "o3-mini"
	bModel.reasoningEffort = sdk.ReasoningHigh
	bModel.providers = map[string]config.ProviderConfig{
		"openai": {Name: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "test-key"},
	}
	// Populate catalog entry for gpt-4o declaring no reasoning support
	noReasoning := false
	bModel.modelCatalogs.Set("openai", []domainmodel.RemoteModel{
		{ID: "gpt-4o", Reasoning: &modelprofile.CatalogReasoning{Supported: &noReasoning}},
	})

	// Switch to gpt-4o which does not support reasoning
	msg := modelSelectedMsg{
		providerName: "openai",
		modelID:      "gpt-4o",
	}
	updated, _ := bModel.Update(msg)
	bModel = updated.(*bubbleModel)

	if bModel.activeModel != "gpt-4o" {
		t.Fatalf("activeModel = %q, want gpt-4o", bModel.activeModel)
	}
	if bModel.reasoningEffort != sdk.ReasoningDefault {
		t.Fatalf("reasoningEffort was not reset to auto: got %q", bModel.reasoningEffort)
	}
}

func TestReconfigureRunnerInvalidatesRunnerOnError(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.activeProvider = "openai"
	bModel.activeModel = "non-existent-model"
	bModel.providers = map[string]config.ProviderConfig{
		"openai": {Name: "openai", BaseURL: "https://api.openai.com/v1", APIKey: ""}, // empty key -> no auth
	}
	bModel.reconfigureRunner()
	if bModel.runner != nil {
		t.Fatal("expected runner to be nil when provider lacks valid auth")
	}
}

func TestModelPickerOllamaKeylessDiscovery(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{
		"ollama": {Name: "ollama", BaseURL: "http://localhost:11434", APIKey: ""},
	}
	view := newModelSelectPaneView(bModel)
	for i, name := range view.providerNames {
		if strings.EqualFold(name, "ollama") {
			view.providerIndex = i
			break
		}
	}
	cmd := view.loadProvider(bModel, true)
	if cmd == nil {
		t.Fatal("expected discovery command for keyless Ollama provider")
	}
}

func TestModelPickerEmptyFilterShowsSearchInput(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model")
	view, ok := bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if !ok || view == nil {
		t.Fatal("expected modelSelectViewID open")
	}
	view.picker.SetFilterText("nonexistent-model-xyz")
	view.picker.SetFilterState(list.Filtering)
	view.syncPickerProjection()
	rendered := view.Render(bModel)
	if !strings.Contains(rendered, "Search: nonexistent-model-xyz") {
		t.Fatalf("expected search query in rendered output: %s", rendered)
	}
	if !strings.Contains(rendered, "No models match") {
		t.Fatalf("expected 'No models match' in rendered output: %s", rendered)
	}
}

func TestModelPickerShiftTabCyclesProvidersWithoutLeaking(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", BaseURL: "https://alpha.example.com", APIKey: "k1"},
		"beta":  {Name: "beta", BaseURL: "https://beta.example.com", APIKey: "k2"},
	}
	bModel.executeCommand("/model")
	view, ok := bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if !ok || view == nil {
		t.Fatal("expected modelSelectViewID open")
	}
	initialIdx := view.providerIndex
	initialMode := bModel.service.Mode()

	handled, _ := view.HandleKey(bModel, testShiftTab())
	if !handled {
		t.Fatal("shift+tab was not handled by model picker")
	}
	if bModel.service.Mode() != initialMode {
		t.Fatalf("permission mode changed from %s to %s on shift+tab", initialMode, bModel.service.Mode())
	}
	if view.providerIndex == initialIdx && len(view.providerNames) > 1 {
		t.Fatalf("providerIndex did not change on shift+tab: %d", view.providerIndex)
	}
}

func TestModelPickerEnterWhileFilteringSelectsModel(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model")
	view, ok := bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if !ok || view == nil {
		t.Fatal("expected modelSelectViewID open")
	}
	view.setModels([]domainmodel.RemoteModel{{ID: "deepseek-chat", Name: "DeepSeek Chat"}}, "")
	view.picker.SetFilterText("deepseek")
	view.picker.Select(0)

	handled, cmd := view.HandleKey(bModel, testKey(tea.KeyEnter))
	if !handled {
		t.Fatal("enter while filtering was not handled")
	}
	if cmd == nil {
		t.Fatal("expected saveDefaultModelCmd returned on enter while filtering")
	}
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model picker closed after enter selection")
	}
}

func TestModelPickerEnterOnZeroMatchesDoesNotOpenProviderEditor(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model")
	view, ok := bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if !ok || view == nil {
		t.Fatal("expected modelSelectViewID open")
	}
	view.setModels(nil, "")

	handled, cmd := view.HandleKey(bModel, testKey(tea.KeyEnter))
	if !handled {
		t.Fatal("enter on 0 matches was not handled")
	}
	if cmd != nil {
		t.Fatal("unexpected command on enter with 0 models")
	}
	if bModel.bottom.has(providerViewID) {
		t.Fatal("enter on 0 models should not open providerViewID")
	}
}
