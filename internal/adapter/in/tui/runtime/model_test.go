package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"errors"
	"fmt"
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

func TestModelSetupRejectsStaleProviderResponse(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"alpha": {Name: "alpha", APIKey: "a"}, "beta": {Name: "beta", APIKey: "b"}}
	m.activeProvider = "alpha"
	view := newModelSetupPaneView(m)
	m.panes.bottom.push(view)
	view.fetchRequestID = 2
	view.providerIndex = 1
	updated, _ := m.Update(modelsFetchedMsg{providerName: "alpha", requestID: 1, models: []model.RemoteModel{{ID: "stale-alpha"}}})
	m = updated.(*bubbleModel)
	view = m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if got := m.modelCatalogs.Models("alpha"); len(got) != 0 {
		t.Fatalf("stale alpha response mutated catalog: %#v", got)
	}
	if len(view.models) > 0 && view.models[0].ID == "stale-alpha" {
		t.Fatalf("stale alpha response mutated beta picker: %#v", view.models)
	}
}

func TestModelSetupAcceptsCurrentProviderResponse(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"alpha": {Name: "alpha", APIKey: "a"}, "beta": {Name: "beta", APIKey: "b"}}
	m.activeProvider = "beta"
	view := newModelSetupPaneView(m)
	m.panes.bottom.push(view)
	view.fetchRequestID = 3
	updated, _ := m.Update(modelsFetchedMsg{providerName: "beta", requestID: 3, models: []model.RemoteModel{{ID: "beta-model"}}})
	m = updated.(*bubbleModel)
	view = m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if len(view.models) != 1 || view.models[0].ID != "beta-model" {
		t.Fatalf("current response not applied: %#v", view.models)
	}
}

func TestModelSetupLoadingHidesPreviousProviderModels(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"alpha": {Name: "alpha", APIKey: "a"}, "beta": {Name: "beta", APIKey: "b"}}
	m.activeProvider = "alpha"
	m.modelCatalogs.Set("alpha", []model.RemoteModel{{ID: "alpha-only", Name: "Alpha Only"}})
	view := newModelSetupPaneView(m)
	m.panes.bottom.push(view)
	view.providerIndex = 1
	_ = view.beginFetch(m.ctx, "beta", m.providers["beta"])
	rendered := view.Render(newPaneRenderContext(m))
	if !strings.Contains(rendered, "Loading") {
		t.Fatalf("loading state not rendered: %q", rendered)
	}
	if strings.Contains(rendered, "Alpha Only") {
		t.Fatalf("previous provider model leaked into loading state: %q", rendered)
	}
}

func TestModelSetupRendersFetchError(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSetupPaneView(m)
	view.loading = false
	view.err = errors.New("authentication failed (401)")
	rendered := view.Render(newPaneRenderContext(m))
	if !strings.Contains(rendered, "Failed to load models") || !strings.Contains(rendered, "authentication failed") {
		t.Fatalf("error state not rendered: %q", rendered)
	}
}

func TestModelSetupAcceptsEmptyCurrentCatalog(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"alpha": {Name: "alpha", APIKey: "a"}}
	m.activeProvider = "alpha"
	view := newModelSetupPaneView(m)
	m.panes.bottom.push(view)
	view.fetchRequestID = 4
	view.loading = true
	updated, _ := m.Update(modelsFetchedMsg{providerName: "alpha", requestID: 4})
	m = updated.(*bubbleModel)
	view = m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if view.loading || view.err != nil || len(view.models) != 0 {
		t.Fatalf("empty current catalog state = loading:%t err:%v models:%#v", view.loading, view.err, view.models)
	}
}

func TestModelSetupBeginFetchCancelsPreviousRequest(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSetupPaneView(m)
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

func TestModelSetupCloseCancelsFetch(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSetupPaneView(m)
	m.panes.bottom.push(view)
	canceled := false
	view.fetchCancel = func() {
		canceled = true
	}
	updated, _ := m.Update(testKey(tea.KeyEsc))
	m = updated.(*bubbleModel)
	if !canceled {
		t.Fatal("closing model setup did not cancel fetch")
	}
	if m.panes.bottom.has(modelSetupViewID) {
		t.Fatal("model setup remained open after escape")
	}
}

func TestModelSetupCustomProviderDoesNotUseProtonmanFallback(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"}}
	m.activeProvider = "custom"
	view := newModelSetupPaneView(m)
	if len(view.models) != 0 {
		t.Fatalf("custom provider inherited fallback models: %#v", view.models)
	}
}

func TestModelSetupResetSelectionAnchorsActiveModel(t *testing.T) {
	view := &modelSetupPaneView{models: []model.RemoteModel{{ID: "one"}, {ID: "two"}, {ID: "three"}}}
	view.resetSelection("")
	view.picker.Select(2)
	view.resetSelection("two")
	if view.picker.Index() != 1 || (view.picker.Paginator.Page*view.picker.Paginator.PerPage) != 0 {
		t.Fatalf("selection = index:%d offset:%d, want 1/0", view.picker.Index(), (view.picker.Paginator.Page * view.picker.Paginator.PerPage))
	}
}

func TestModelSetupResetSelectionFallsBackToFirstModel(t *testing.T) {
	view := &modelSetupPaneView{models: []model.RemoteModel{{ID: "one"}, {ID: "two"}}}
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

func TestModelSetupUsesFreshCacheWithoutFetch(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"}}
	m.activeProvider = "custom"
	m.modelCatalogs.Set("custom", []model.RemoteModel{{ID: "cached"}})
	view := newModelSetupPaneView(m)
	if cmd := view.loadProvider(m, false); cmd != nil {
		t.Fatal("fresh catalog triggered a network fetch")
	}
	if len(view.models) != 1 || view.models[0].ID != "cached" {
		t.Fatalf("fresh cache not used: %#v", view.models)
	}
}

func TestModelSetupRefreshBypassesFreshCache(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{"custom": {Name: "custom", BaseURL: "https://api.example.com/v1", APIKey: "key"}}
	m.activeProvider = "custom"
	m.modelCatalogs.Set("custom", []model.RemoteModel{{ID: "cached"}})
	view := newModelSetupPaneView(m)
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
	msg, ok := cmd().(modelSetupAppliedMsg)
	if !ok {
		t.Fatalf("expected modelSetupAppliedMsg")
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
	msg := cmd().(modelSetupAppliedMsg)
	if msg.unverified {
		t.Fatal("discovered model was marked unverified")
	}
}

func TestModelSetupFilterMatchesIDNameVendorAndFeatures(t *testing.T) {
	view := &modelSetupPaneView{}
	view.setModels([]model.RemoteModel{{ID: "deepseek-v4", Name: "DeepSeek V4", Provider: "DeepSeek", Features: []string{"tools", "vision"}}, {ID: "qwen-flash", Name: "Qwen Flash", Provider: "Qwen", Features: []string{"text"}}}, view.activeProviderName(), "")
	for _, query := range []string{"deepseek-v4", "DeepSeek V4", "deepseek", "vision"} {
		view.picker.SetFilterText(query)
		view.syncPickerProjection()
		if len(view.models) != 1 || view.models[0].ID != "deepseek-v4" {
			t.Fatalf("filter %q = %#v", query, view.models)
		}
	}
}

func TestModelSetupFilterCanReturnNoResults(t *testing.T) {
	view := &modelSetupPaneView{}
	view.setModels([]model.RemoteModel{{ID: "one"}, {ID: "two"}}, view.activeProviderName(), "")
	view.picker.SetFilterText("missing")
	view.syncPickerProjection()
	if len(view.models) != 0 || len(view.allModels) != 2 {
		t.Fatalf("filtered/all models = %#v / %#v", view.models, view.allModels)
	}
}

func TestModelSetupSearchModeAcceptsReservedLetters(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	view := newModelSetupPaneView(m)
	m.panes.bottom.push(view)
	updated, _ := m.Update(testText("/"))
	m = updated.(*bubbleModel)
	updated, _ = m.Update(testText("qwen"))
	m = updated.(*bubbleModel)
	view = m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if view.picker.FilterValue() != "qwen" {
		t.Fatalf("filter = %q, want qwen", view.picker.FilterValue())
	}
	if !m.panes.bottom.has(modelSetupViewID) {
		t.Fatal("reserved q closed picker while search mode was active")
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

func seedModelSetupCatalog(m *bubbleModel) {
	m.modelCatalogs.Set(model.DefaultProtonmanName, []model.RemoteModel{{ID: "deepseek-v4-flash-vision-exp", Name: "DeepSeek V4 Flash Vision"}, {ID: "glm-5.3-flash", Name: "GLM 5.3 Flash"}, {ID: "Qwen3.8-Flash", Name: "Qwen 3.8 Flash"}, {ID: "muse-spark", Name: "Muse Spark"}, {ID: "MiniMax-M3", Name: "MiniMax M3"}, {ID: "fixture-six", Name: "Fixture Six"}})
}

func TestModelSetupLaunchViaSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	seedModelSetupCatalog(bModel)
	bModel.activeModel = "MiniMax-M3"
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/model")
	if !bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected model select modal open after /model")
	}
	view := bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if len(view.models) == 0 {
		t.Fatal("expected models in catalog")
	}
	if view.models[view.picker.Index()].ID != "MiniMax-M3" {
		t.Fatalf("expected focused model 'MiniMax-M3', got %s", view.models[view.picker.Index()].ID)
	}
	rendered := bModel.View().Content
	if !strings.Contains(rendered, "Switch Model") {
		t.Fatalf("expected 'Switch Model' in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "current") {
		t.Fatalf("expected active model current badge in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "MiniMax M3") {
		t.Fatalf("expected display name 'MiniMax M3' in view, got:\n%s", rendered)
	}
	updated, _ := bModel.Update(testKey(tea.KeyEsc))
	bModel = updated.(*bubbleModel)
	if bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected model select modal closed after Esc")
	}
	bModel.executeCommand("/model select")
	if !bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected model select modal open after /model select")
	}
}

func TestModelSetupPreservesComposerDraft(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	seedModelSetupCatalog(bModel)
	bModel.activeModel = "MiniMax-M3"
	bModel.activeProvider = "protonman"
	bModel.panes.bottom.prompt().SetValue("draft before model picker")
	bModel.executeCommand("/model")
	if !bModel.panes.bottom.composerVisible() {
		t.Fatal("model setup should overlay the composer, not replace it")
	}
	rendered := testPlain(bModel.View().Content)
	for _, want := range []string{"Switch Model", "> draft before model picker"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("model setup view missing %q:\n%s", want, rendered)
		}
	}
	updated, _ := bModel.Update(testKey(tea.KeyEsc))
	bModel = updated.(*bubbleModel)
	if got := bModel.panes.bottom.prompt().Value(); got != "draft before model picker" {
		t.Fatalf("composer draft after model setup close = %q", got)
	}
}

func TestBottomPanePresentationPolicy(t *testing.T) {
	belowComposer := []bottomPaneView{
		&skillsPaneView{}, &todoPaneView{}, &slashPaneView{}, &agentsPaneView{},
		&shortcutsPaneView{}, &modelSetupPaneView{}, &providerSelectPaneView{},
	}
	for _, view := range belowComposer {
		if view.PresentationMode() != paneBelowComposer {
			t.Fatalf("%T presentation mode = %v, want below composer", view, view.PresentationMode())
		}
	}
	blocking := []bottomPaneView{&permissionPaneView{}, &providerPaneView{}}
	for _, view := range blocking {
		if view.PresentationMode() != paneBlocking {
			t.Fatalf("%T presentation mode = %v, want blocking", view, view.PresentationMode())
		}
	}
}

func TestModelSetupToggleKeybinding(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	updated, _ := bModel.Update(testCtrl('p'))
	bModel = updated.(*bubbleModel)
	if !bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected model select modal open after Ctrl+P")
	}
	updated, _ = bModel.Update(testCtrl('p'))
	bModel = updated.(*bubbleModel)
	if bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected model select modal closed after second Ctrl+P")
	}
	updated, _ = bModel.Update(testAltText("m"))
	bModel = updated.(*bubbleModel)
	if bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("legacy Alt+M alias reopened model setup")
	}
}

func TestModelSetupNavigationAndConfirm(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	seedModelSetupCatalog(bModel)
	bModel.activeModel = "deepseek-v4-flash-vision-exp"
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/model")
	view := bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if view.picker.Index() != 0 {
		t.Fatalf("expected initial index 0, got %d", view.picker.Index())
	}
	updated, _ := bModel.Update(testText("j"))
	bModel = updated.(*bubbleModel)
	view = bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if view.picker.Index() != 1 {
		t.Fatalf("expected index 1 after 'j', got %d", view.picker.Index())
	}
	updated, _ = bModel.Update(testText("k"))
	bModel = updated.(*bubbleModel)
	view = bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if view.picker.Index() != 0 {
		t.Fatalf("expected index 0 after 'k', got %d", view.picker.Index())
	}
	updated, _ = bModel.Update(testKey(tea.KeyDown))
	bModel = updated.(*bubbleModel)
	updated, _ = bModel.Update(testKey(tea.KeyDown))
	bModel = updated.(*bubbleModel)
	view = bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if view.picker.Index() != 2 {
		t.Fatalf("expected index 2 after moving down twice, got %d", view.picker.Index())
	}
	if view.models[view.picker.Index()].ID != "Qwen3.8-Flash" {
		t.Fatalf("expected Qwen3.8-Flash at index 2, got %s", view.models[view.picker.Index()].ID)
	}
	t.Setenv("PROTONMAN_HOME", t.TempDir())
	updated, cmd := bModel.Update(testKey(tea.KeyEnter))
	bModel = updated.(*bubbleModel)
	if bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected modelSetupViewID removed on Enter")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd on Enter")
	}
	msg := cmd()
	selectedMsg, ok := msg.(modelSetupAppliedMsg)
	if !ok {
		t.Fatalf("expected modelSetupAppliedMsg, got %T", msg)
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

func TestModelSetupDirectModelCommand(t *testing.T) {
	t.Setenv("PROTONMAN_HOME", t.TempDir())
	bModel := newTestSkillsModel(t, 1)
	cmd := bModel.executeCommand("/model glm-5.3-flash")
	if cmd == nil {
		t.Fatal("expected cmd from /model <id>")
	}
	msg := cmd()
	selectedMsg, ok := msg.(modelSetupAppliedMsg)
	if !ok {
		t.Fatalf("expected modelSetupAppliedMsg, got %T", msg)
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

func TestModelSetupSwitchToAddProvider(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model")
	if !bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected model select modal open")
	}
	updated, _ := bModel.Update(testText("a"))
	bModel = updated.(*bubbleModel)
	if bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected modelSetupViewID removed after 'a'")
	}
	if !bModel.panes.bottom.has(providerViewID) {
		t.Fatal("expected providerViewID added after 'a'")
	}
}

func TestModelSetupInfoViewAndWelcome(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.activeModel = "deepseek-v4-flash-vision-exp"
	bModel.activeProvider = "protonman"
	info := bModel.infoView()
	if info != "" {
		t.Fatalf("idle infoView = %q, want no persistent model metadata", info)
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

func TestModelSetupPagedNavigation(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.activeProvider = model.DefaultProtonmanName
	seedModelSetupCatalog(m)
	models := m.modelCatalogs.Models(model.DefaultProtonmanName)
	models = append(models, domainmodel.RemoteModel{ID: "fixture-seven", Name: "Fixture Seven"})
	m.modelCatalogs.Set(model.DefaultProtonmanName, models)
	m.resize(40, 14)
	m.executeCommand("/model")
	view := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	view.picker.Select(0)
	updated, _ := m.Update(testKey(tea.KeyPgDown))
	m = updated.(*bubbleModel)
	if view.picker.Index() <= 0 {
		t.Fatalf("pgdown did not advance selection: index=%d", view.picker.Index())
	}
	updated, _ = m.Update(testKey(tea.KeyEnd))
	m = updated.(*bubbleModel)
	view = m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if view.picker.Index() != len(view.models)-1 {
		t.Fatalf("end index = %d, want %d", view.picker.Index(), len(view.models)-1)
	}
	updated, _ = m.Update(testKey(tea.KeyHome))
	m = updated.(*bubbleModel)
	view = m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if view.picker.Index() != 0 {
		t.Fatalf("home index = %d, want 0", view.picker.Index())
	}
}

func TestBubbleModelRunsToolCommandThroughService(t *testing.T) {
	registry, handler := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
	model.resize(80, 24)
	model.panes.bottom.prompt().SetValue(`:call read {"path":"README.md"}`)
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
	model.panes.bottom.prompt().SetValue(":help")
	if command := model.submit(); command != nil {
		t.Fatalf("busy submit command = %v, want nil", command)
	}
	if got := model.panes.bottom.prompt().Value(); got != "" {
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
	model.panes.bottom.prompt().SetValue("pwd")
	command := model.submit()
	if command == nil {
		t.Fatal("bash submit command = nil")
	}
	if model.panes.bottom.bashMode() {
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
	initial := []tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}}
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

func TestTUICyclePermissionUpdatesCoordinator(t *testing.T) {
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
	model.cyclePermission()
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
	model.cyclePermission()
	if model.planMode {
		t.Fatal("expected planMode to be false after second cycle")
	}
	if coordinator.CallGuard() != nil {
		t.Fatal("expected coordinator call guard to be cleared")
	}
	if coordinator.PermissionMode() != permission.ModeAlwaysApprove {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAlwaysApprove, coordinator.PermissionMode())
	}
	model.cyclePermission()
	if coordinator.PermissionMode() != permission.ModeAsk {
		t.Fatalf("expected coordinator mode %v, got %v", permission.ModeAsk, coordinator.PermissionMode())
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

func TestModelSetupReconcilesIncompatibleReasoningEffort(t *testing.T) {
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
	msg := modelSetupAppliedMsg{
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

func TestModelSetupOllamaKeylessDiscovery(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{
		"ollama": {Name: "ollama", BaseURL: "http://localhost:11434", APIKey: ""},
	}
	view := newModelSetupPaneView(bModel)
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

func TestModelSetupEmptyFilterShowsSearchInput(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model")
	view, ok := bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if !ok || view == nil {
		t.Fatal("expected modelSetupViewID open")
	}
	view.picker.SetFilterText("nonexistent-model-xyz")
	view.picker.SetFilterState(list.Filtering)
	view.syncPickerProjection()
	rendered := view.Render(newPaneRenderContext(bModel))
	if !strings.Contains(rendered, "Search: nonexistent-model-xyz") {
		t.Fatalf("expected search query in rendered output: %s", rendered)
	}
	if !strings.Contains(rendered, "No matches") {
		t.Fatalf("expected 'No matches' in rendered output: %s", rendered)
	}
}

func TestModelSetupShiftTabCyclesPermissionWithoutChangingProvider(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", BaseURL: "https://alpha.example.com", APIKey: "k1"},
		"beta":  {Name: "beta", BaseURL: "https://beta.example.com", APIKey: "k2"},
	}
	bModel.executeCommand("/model")
	view, ok := bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if !ok || view == nil {
		t.Fatal("expected modelSetupViewID open")
	}
	initialIdx := view.providerIndex
	updated, _ := bModel.Update(testShiftTab())
	bModel = updated.(*bubbleModel)
	if !bModel.planMode {
		t.Fatal("shift+tab did not cycle permission into plan mode")
	}
	if view.providerIndex != initialIdx {
		t.Fatalf("shift+tab changed provider index from %d to %d", initialIdx, view.providerIndex)
	}
}

func TestModelSetupKeepsSelectionAcrossResponsiveResize(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	models := make([]domainmodel.RemoteModel, 30)
	for i := range models {
		models[i] = domainmodel.RemoteModel{ID: fmt.Sprintf("model-%02d", i), Name: fmt.Sprintf("Model %02d", i)}
	}
	view := newModelSetupPaneView(m)
	view.setModels(models, view.activeProviderName(), "model-20")
	m.panes.bottom.push(view)
	for _, size := range [][2]int{{120, 32}, {40, 12}, {24, 8}, {80, 24}} {
		m.resize(size[0], size[1])
		_ = view.Render(newPaneRenderContext(m))
		selected, ok := view.picker.SelectedItem().(modelListItem)
		if !ok || selected.model.ID != "model-20" {
			t.Fatalf("selected model after resize %dx%d = %#v, want model-20", size[0], size[1], view.picker.SelectedItem())
		}
		if got := lipgloss.Width(view.Render(newPaneRenderContext(m))); got > size[0] {
			t.Fatalf("model setup width=%d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
	}
}

func TestModelSetupFilteredSelectionSurvivesResize(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 28)
	m.panes.bottom.push(newModelSetupPaneView(m))
	view := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	models := make([]domainmodel.RemoteModel, 24)
	for i := range models {
		models[i] = domainmodel.RemoteModel{ID: fmt.Sprintf("model-%02d", i), Name: fmt.Sprintf("Model %02d", i)}
	}
	view.setModels(models, view.activeProviderName(), "")
	view.picker.SetFilterText("model-1")
	view.picker.SetFilterState(list.FilterApplied)
	view.syncPickerProjection()
	view.picker.Select(4)
	selected, ok := view.picker.SelectedItem().(modelListItem)
	if !ok {
		t.Fatal("filtered picker has no selection")
	}
	want := selected.model.ID
	for _, size := range [][2]int{{40, 12}, {24, 8}, {120, 32}} {
		m.resize(size[0], size[1])
		_ = view.Render(newPaneRenderContext(m))
		selected, ok = view.picker.SelectedItem().(modelListItem)
		if !ok || selected.model.ID != want {
			t.Fatalf("filtered selection after resize %dx%d=%v want %q", size[0], size[1], selected.model.ID, want)
		}
		if got := lipgloss.Width(view.Render(newPaneRenderContext(m))); got > size[0] {
			t.Fatalf("filtered model setup width=%d exceeds %d", got, size[0])
		}
	}
}

func TestModelSetupEnterWhileFilteringSelectsModel(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model")
	view, ok := bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if !ok || view == nil {
		t.Fatal("expected modelSetupViewID open")
	}
	view.setModels([]domainmodel.RemoteModel{{ID: "deepseek-chat", Name: "DeepSeek Chat"}}, view.activeProviderName(), "")
	view.picker.SetFilterText("deepseek")
	view.picker.Select(0)

	handled, cmd := bModel.handlePaneKey(testKey(tea.KeyEnter))
	if !handled {
		t.Fatal("enter while filtering was not handled")
	}
	if cmd == nil {
		t.Fatal("expected model setup apply command returned on enter while filtering")
	}
	if bModel.panes.bottom.has(modelSetupViewID) {
		t.Fatal("expected model setup closed after enter selection")
	}
}

func TestModelSetupEnterOnZeroMatchesDoesNotOpenProviderEditor(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model")
	view, ok := bModel.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	if !ok || view == nil {
		t.Fatal("expected modelSetupViewID open")
	}
	view.setModels(nil, view.activeProviderName(), "")

	handled, cmd := bModel.handlePaneKey(testKey(tea.KeyEnter))
	if !handled {
		t.Fatal("enter on 0 matches was not handled")
	}
	if cmd != nil {
		t.Fatal("unexpected command on enter with 0 models")
	}
	if bModel.panes.bottom.has(providerViewID) {
		t.Fatal("enter on 0 models should not open providerViewID")
	}
}

func TestStaleModelSetupDoesNotMutateReopenedPane(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.activeModel = "before"
	oldID := nextAsyncOperationID()
	m.activeModelSetup = oldID
	m.panes.bottom.push(newModelSetupPaneView(m))
	if m.activeModelSetup != 0 {
		t.Fatalf("reopened picker did not invalidate prior selection: %d", m.activeModelSetup)
	}

	updated, _ := m.Update(modelSetupAppliedMsg{operationID: oldID, providerName: "protonman", modelID: "stale"})
	m = updated.(*bubbleModel)
	if m.activeModel != "before" {
		t.Fatalf("stale model selection changed active model to %q", m.activeModel)
	}
	if !m.panes.bottom.has(modelSetupViewID) {
		t.Fatal("stale model selection closed reopened picker")
	}
}

func TestModelFetchRequiresRuntimeContext(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	v := newModelSetupPaneView(m)
	cmd := v.beginFetch(nil, "protonman", config.ProviderConfig{Name: "protonman"})
	if cmd != nil {
		t.Fatalf("nil-context model fetch command = %v, want nil", cmd)
	}
	if v.err == nil || !strings.Contains(v.err.Error(), "runtime context") {
		t.Fatalf("nil-context model fetch error = %v", v.err)
	}
}

func TestModelSetupDefaultsToOpenCodeWhenNoProviderConfigured(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = nil
	m.activeProvider = ""
	view := newModelSetupPaneView(m)
	if got := view.activeProviderName(); got != model.DefaultOpenCodeName {
		t.Fatalf("default picker provider = %q, want %q", got, model.DefaultOpenCodeName)
	}
}

func TestReasoningCompatibilityFallbackPreservesAndRestoresPreference(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.activeProvider = "custom"
	m.projectConfigProvenance = map[string]config.ValueSource{config.FieldAgentReasoningEffort: config.SourceUser}
	m.applyReasoningPreference(sdk.ReasoningHigh, reasoningPreferenceConfig)

	no := false
	m.activeModel = "plain-model"
	m.modelCatalogs.Set("custom", []model.RemoteModel{{ID: "plain-model", Reasoning: &modelprofile.CatalogReasoning{Supported: &no}}})
	if !m.reconcileReasoningForActiveModel() {
		t.Fatal("unsupported model did not trigger compatibility fallback")
	}
	if m.reasoningEffort != sdk.ReasoningDefault || m.reasoningPreference != sdk.ReasoningHigh {
		t.Fatalf("fallback effective=%q preference=%q, want auto/high", m.reasoningEffort, m.reasoningPreference)
	}

	yes := true
	m.activeModel = "reasoning-model"
	m.modelCatalogs.Set("custom", []model.RemoteModel{{ID: "reasoning-model", Reasoning: &modelprofile.CatalogReasoning{Supported: &yes, Levels: []sdk.ReasoningEffort{sdk.ReasoningHigh}}}})
	if !m.reconcileReasoningForActiveModel() {
		t.Fatal("compatible model did not restore requested reasoning")
	}
	if m.reasoningEffort != sdk.ReasoningHigh || m.reasoningCompatibilityFallback {
		t.Fatalf("restored effective=%q fallback=%v, want high/false", m.reasoningEffort, m.reasoningCompatibilityFallback)
	}
}

func TestSelectModelDirectDefaultsToOpenCodeWhenProviderUnset(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("PROTONMAN_HOME", homeDir)
	m := newTestSkillsModel(t, 1)
	m.activeProvider = ""
	cmd := m.selectModelDirect("custom-model")
	if cmd == nil {
		t.Fatal("direct model selection returned nil command")
	}
	msg := cmd().(modelSetupAppliedMsg)
	if msg.providerName != model.DefaultOpenCodeName {
		t.Fatalf("provider = %q, want %q", msg.providerName, model.DefaultOpenCodeName)
	}
}

func TestReconfigureRunnerUsesOpenCodeFallbackWhenProviderUnset(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.activeProvider = ""
	m.activeModel = model.DefaultOpenCodeModel
	m.providers = map[string]config.ProviderConfig{
		model.DefaultOpenCodeName:  {Name: model.DefaultOpenCodeName, Type: "openai", BaseURL: model.DefaultOpenCodeEndpoint},
		model.DefaultProtonmanName: {Name: model.DefaultProtonmanName, Type: "openai", BaseURL: "https://protonman.dev/api/v1"},
	}
	m.reconfigureRunner()
	if m.runner == nil {
		t.Fatal("provider-less runner did not use OpenCode fallback")
	}
}

func TestUnifiedModelSetupAppliesModelAndThinkingTogether(t *testing.T) {
	t.Setenv("PROTONMAN_HOME", t.TempDir())
	m := newTestSkillsModel(t, 1)
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"
	m.providers = map[string]config.ProviderConfig{"protonman": {Name: "protonman", Type: "openai", BaseURL: "https://protonman.dev/api/v1", APIKey: "key"}}
	m.modelCatalogs.Set("protonman", []domainmodel.RemoteModel{{ID: "gemini-3.8-flash", Name: "Gemini 3.8 Flash"}})
	m.executeCommand("/model")
	view := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView)
	view.reasoningIndex = 3 // high: auto, low, medium, high
	updated, cmd := m.Update(testKey(tea.KeyEnter))
	m = updated.(*bubbleModel)
	if cmd == nil {
		t.Fatal("enter did not produce model setup apply command")
	}
	msg, ok := cmd().(modelSetupAppliedMsg)
	if !ok {
		t.Fatalf("apply message = %T, want modelSetupAppliedMsg", cmd())
	}
	if msg.modelID != "gemini-3.8-flash" || msg.reasoning != sdk.ReasoningHigh {
		t.Fatalf("selection = %q/%q, want gemini-3.8-flash/high", msg.modelID, msg.reasoning)
	}
	updated, _ = m.Update(msg)
	m = updated.(*bubbleModel)
	if m.activeModel != "gemini-3.8-flash" || m.reasoningEffort != sdk.ReasoningHigh {
		t.Fatalf("effective selection = %q/%q", m.activeModel, m.reasoningEffort)
	}
}

func TestModelSetupCurrentMarkerUsesProviderModelPair(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", Type: "openai", BaseURL: "https://alpha.example/v1", APIKey: "x"},
		"beta":  {Name: "beta", Type: "openai", BaseURL: "https://beta.example/v1", APIKey: "x"},
	}
	m.activeProvider = "alpha"
	m.activeModel = "shared-model"
	m.modelCatalogs.Set("alpha", []domainmodel.RemoteModel{{ID: "shared-model", Name: "Alpha Shared"}})
	m.modelCatalogs.Set("beta", []domainmodel.RemoteModel{{ID: "shared-model", Name: "Beta Shared"}})
	view := newModelSetupPaneView(m)
	for i, name := range view.providerNames {
		if name == "beta" {
			view.providerIndex = i
			break
		}
	}
	view.setModels(m.modelCatalogs.Models("beta"), m.activeProvider, m.activeModel)
	item, ok := view.picker.SelectedItem().(modelListItem)
	if !ok {
		t.Fatal("expected beta model item")
	}
	if item.current {
		t.Fatal("same model id on another provider was marked current")
	}
}

func TestModelDisplayNameHumanizesIdentifier(t *testing.T) {
	got := modelDisplayName(domainmodel.RemoteModel{ID: "nemotron-3.5-lightning-free"})
	if got != "Nemotron 3.5 Lightning" {
		t.Fatalf("display name = %q", got)
	}
}

func TestModelSetupMuseSparkUsesFamilyReasoningLevels(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "opencode"
	m.activeModel = "muse-spark-1.3-contributor-free"
	m.modelCatalogs.Set("opencode", []domainmodel.RemoteModel{{ID: m.activeModel, Name: "Muse Spark 1.3 Contributor"}})
	view := newModelSetupPaneView(m)
	want := []sdk.ReasoningEffort{sdk.ReasoningDefault, sdk.ReasoningMinimal, sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningHigh, sdk.ReasoningXHigh, sdk.ReasoningMax}
	if len(view.reasoningChoices) != len(want) {
		t.Fatalf("muse reasoning choices = %v, want %v", view.reasoningChoices, want)
	}
	for i := range want {
		if view.reasoningChoices[i] != want[i] {
			t.Fatalf("muse reasoning choices = %v, want %v", view.reasoningChoices, want)
		}
	}
	profile := domainmodel.ResolveModelProfile("opencode", m.activeModel, nil)
	if profile.Reasoning.Default != sdk.ReasoningHigh {
		t.Fatalf("muse default reasoning = %q, want high", profile.Reasoning.Default)
	}
	rendered := testPlain(view.Render(newPaneRenderContext(m)))
	for _, level := range []string{"minimal", "low", "medium", "high", "xhigh", "max"} {
		if !strings.Contains(rendered, level) {
			t.Fatalf("muse picker missing %q: %s", level, rendered)
		}
	}
}

func TestModelSetupSingleItemKeepsThinkingNearModel(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "opencode"
	m.activeModel = "nemotron-3.5-lightning-free"
	m.modelCatalogs.Set("opencode", []domainmodel.RemoteModel{{ID: m.activeModel}})
	view := newModelSetupPaneView(m)
	rendered := strings.Split(view.Render(newPaneRenderContext(m)), "\n")
	modelLine, effortLine := -1, -1
	for index, line := range rendered {
		if strings.Contains(line, "Nemotron 3.5 Lightning") {
			modelLine = index
		}
		if strings.Contains(line, "Effort") {
			effortLine = index
		}
	}
	if modelLine < 0 || effortLine < 0 || effortLine-modelLine > 3 {
		t.Fatalf("excessive vertical gap: model=%d effort=%d", modelLine, effortLine)
	}
}

func TestModelSetupMatchesReferenceHierarchy(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "opencode"
	m.activeModel = "qwen3.6-plus"
	m.modelCatalogs.Set("opencode", []domainmodel.RemoteModel{{ID: "qwen3.6-plus"}, {ID: "qwen3.5-plus"}})
	m.panes.bottom.prompt().SetValue("draft")
	m.executeCommand("/model")

	plain := testPlain(m.View().Content)
	composer := strings.Index(plain, "> draft")
	panel := strings.Index(plain, "Switch Model")
	if composer < 0 || panel < 0 || composer > panel {
		t.Fatalf("reference hierarchy requires composer before model panel:\n%s", plain)
	}
	for _, want := range []string{"(current)", "Effort", "Keyboard:", "Qwen3.6 Plus · auto"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("reference model panel missing %q:\n%s", want, plain)
		}
	}
	if footer := testPlain(m.footerView()); footer != "" {
		t.Fatalf("model panel leaked generic composer footer: %q", footer)
	}
}

func TestModelRowsAlignMetadataColumnAndSelectionMarker(t *testing.T) {
	freeShort := modelListItem{model: domainmodel.RemoteModel{ID: "big-pickle", Name: "Big Pickle"}}
	freeSelected := modelListItem{model: domainmodel.RemoteModel{ID: "muse-spark-1.3-contributor-free", Name: "Muse Spark 1.3 Contributor"}, current: true}
	plainShort := testPlain(renderModelRow(freeShort, false, 56))
	plainSelected := testPlain(renderModelRow(freeSelected, true, 56))
	if !strings.HasPrefix(plainShort, "  Big Pickle") {
		t.Fatalf("unselected row marker/padding = %q", plainShort)
	}
	if !strings.HasPrefix(plainSelected, "> Muse Spark") {
		t.Fatalf("selected row marker = %q, want ASCII >", plainSelected)
	}
	if !strings.HasSuffix(plainShort, "FREE") || !strings.HasSuffix(plainSelected, "FREE  (current)") {
		t.Fatalf("metadata column missing: short=%q selected=%q", plainShort, plainSelected)
	}
	shortFree := strings.Index(plainShort, "FREE")
	selectedFree := strings.Index(plainSelected, "FREE")
	if shortFree != selectedFree {
		t.Fatalf("FREE column drifted: short=%d selected=%d\nshort=%q\nselected=%q", shortFree, selectedFree, plainShort, plainSelected)
	}
}

func TestModelRowDropsMetadataBeforeTruncatingUsefulNameSpace(t *testing.T) {
	entry := modelListItem{model: domainmodel.RemoteModel{ID: "muse-spark-1.3-contributor-free", Name: "Muse Spark 1.3 Contributor"}, current: true}
	plain := testPlain(renderModelRow(entry, true, 22))
	if strings.Contains(plain, "FREE") || strings.Contains(plain, "current") {
		t.Fatalf("narrow row kept metadata instead of prioritizing model name: %q", plain)
	}
	if !strings.HasPrefix(plain, "> Muse") {
		t.Fatalf("narrow row lost selected model identity: %q", plain)
	}
}

func TestModelSetupGLM53FamilyExposesNativeEffortLevels(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "protonman"
	m.activeModel = "glm-5.3-flash"
	m.modelCatalogs.Set("protonman", []domainmodel.RemoteModel{{ID: m.activeModel}})
	view := newModelSetupPaneView(m)
	want := []sdk.ReasoningEffort{sdk.ReasoningDefault, sdk.ReasoningLow, sdk.ReasoningHigh, sdk.ReasoningMax}
	if got := view.reasoningChoices; len(got) != len(want) {
		t.Fatalf("GLM-5.3 choices = %#v, want %#v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("GLM-5.3 choices = %#v, want %#v", got, want)
			}
		}
	}
	plain := testPlain(view.Render(newPaneRenderContext(m)))
	for _, label := range []string{"auto", "low", "high", "max", "←/→ Effort"} {
		if !strings.Contains(plain, label) {
			t.Fatalf("GLM-5.3 picker missing %q:\n%s", label, plain)
		}
	}
	if strings.Contains(plain, "none") {
		t.Fatalf("GLM-5.3 picker exposed unsupported none level:\n%s", plain)
	}
}

func TestModelSetupQwen38FlashExposesNativeEffortLevels(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "protonman"
	m.activeModel = "qwen3.8-flash"
	m.modelCatalogs.Set("protonman", []domainmodel.RemoteModel{{ID: m.activeModel}})
	view := newModelSetupPaneView(m)
	want := []sdk.ReasoningEffort{sdk.ReasoningDefault, sdk.ReasoningNone, sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningXHigh}
	if got := view.reasoningChoices; len(got) != len(want) {
		t.Fatalf("Qwen3.8 Flash choices = %#v, want %#v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Qwen3.8 Flash choices = %#v, want %#v", got, want)
			}
		}
	}
}

func TestModelSetupMiniMaxM3ExposesThinkingToggle(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "protonman"
	m.activeModel = "minimax-m3"
	m.modelCatalogs.Set("protonman", []domainmodel.RemoteModel{{ID: m.activeModel}})
	view := newModelSetupPaneView(m)
	if got := view.reasoningChoices; len(got) != 2 || got[0] != sdk.ReasoningDefault || got[1] != sdk.ReasoningNone {
		t.Fatalf("MiniMax M3 choices = %#v, want auto/none", got)
	}
}

func TestModelSetupDeepSeekV4FamilyExposesNativeEffortLevels(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "protonman"
	m.activeModel = "deepseek-v4-flash-free"
	m.modelCatalogs.Set("protonman", []domainmodel.RemoteModel{{ID: m.activeModel}})
	view := newModelSetupPaneView(m)
	want := []sdk.ReasoningEffort{sdk.ReasoningDefault, sdk.ReasoningNone, sdk.ReasoningLow, sdk.ReasoningHigh, sdk.ReasoningMax}
	if got := view.reasoningChoices; len(got) != len(want) {
		t.Fatalf("DeepSeek V4 choices = %#v, want %#v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("DeepSeek V4 choices = %#v, want %#v", got, want)
			}
		}
	}
	plain := testPlain(view.Render(newPaneRenderContext(m)))
	for _, label := range []string{"auto", "none", "low", "high", "max", "←/→ Effort"} {
		if !strings.Contains(plain, label) {
			t.Fatalf("DeepSeek V4 picker missing %q:\n%s", label, plain)
		}
	}
}

func TestModelSetupUnknownFamilyExposesAutoOnly(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "opencode"
	m.activeModel = "future-unknown-model"
	m.modelCatalogs.Set("opencode", []domainmodel.RemoteModel{{ID: m.activeModel}})
	view := newModelSetupPaneView(m)
	if len(view.reasoningChoices) != 1 || view.reasoningChoices[0] != sdk.ReasoningDefault {
		t.Fatalf("unknown family choices = %#v, want auto only", view.reasoningChoices)
	}
	plain := testPlain(view.Render(newPaneRenderContext(m)))
	if !strings.Contains(plain, "Effort    auto") || strings.Contains(plain, "←/→ Effort") {
		t.Fatalf("unknown family effort UI is not auto-only:\n%s", plain)
	}
}

func TestModelSetupCatalogReasoningOverridesUnknownFamily(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.resize(100, 30)
	m.activeProvider = "opencode"
	m.activeModel = "future-reasoner"
	yes := true
	m.modelCatalogs.Set("opencode", []domainmodel.RemoteModel{{ID: m.activeModel, Reasoning: &modelprofile.CatalogReasoning{Supported: &yes, Levels: []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningHigh}}}})
	view := newModelSetupPaneView(m)
	if got := view.reasoningChoices; len(got) != 3 || got[0] != sdk.ReasoningDefault || got[1] != sdk.ReasoningLow || got[2] != sdk.ReasoningHigh {
		t.Fatalf("catalog reasoning choices = %#v", got)
	}
	plain := testPlain(view.Render(newPaneRenderContext(m)))
	if !strings.Contains(plain, "low") || !strings.Contains(plain, "high") || !strings.Contains(plain, "←/→") {
		t.Fatalf("catalog effort UI missing levels:\n%s", plain)
	}
}

func TestModelSetupHelpUsesWholeResponsiveLabels(t *testing.T) {
	for _, width := range []int{100, 70, 50, 30} {
		help := modelSetupHelp(width, true)
		if lipgloss.Width(help) > width {
			t.Fatalf("help width=%d exceeds width=%d: %q", lipgloss.Width(help), width, help)
		}
		if strings.HasSuffix(help, " b") || strings.HasSuffix(help, " bac") {
			t.Fatalf("help clipped mid-label at width %d: %q", width, help)
		}
	}
}

func TestEffortLayoutAlignsLabelsWithTrackSlots(t *testing.T) {
	view := &modelSetupPaneView{
		reasoningChoices: []sdk.ReasoningEffort{
			sdk.ReasoningDefault, sdk.ReasoningMinimal, sdk.ReasoningLow,
			sdk.ReasoningMedium, sdk.ReasoningHigh, sdk.ReasoningXHigh,
		},
		reasoningIndex: 5,
	}
	track := testPlain(view.effortRow())
	labels := testPlain(view.effortLabels())
	trackRunes := []rune(track)
	labelRunes := []rune(labels)
	dots := make([]int, 0, len(view.reasoningChoices))
	for i, r := range trackRunes {
		if r == '●' {
			dots = append(dots, i)
		}
	}
	if len(dots) != len(view.reasoningChoices) {
		t.Fatalf("dot positions = %v in %q", dots, track)
	}
	for i, effort := range view.reasoningChoices {
		label := reasoningEffortLabel(effort)
		start := strings.Index(string(labelRunes), label)
		if start < 0 {
			t.Fatalf("label %q missing from %q", label, labels)
		}
		center := start + len([]rune(label))/2
		if delta := center - dots[i]; delta < -1 || delta > 1 {
			t.Fatalf("label %q center=%d dot=%d: track=%q labels=%q", label, center, dots[i], track, labels)
		}
		labelRunes[start] = ' '
	}
}

func TestBottomPanePushReplacesExistingViewWithSameID(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	first := &skillsPaneView{}
	second := &skillsPaneView{}
	m.panes.bottom.push(first)
	m.panes.bottom.push(second)
	if got := len(m.panes.bottom.views); got != 1 {
		t.Fatalf("pane stack size = %d, want 1", got)
	}
	if got := m.panes.bottom.find(skillsViewID); got != second {
		t.Fatalf("active skills pane = %p, want latest %p", got, second)
	}
}
