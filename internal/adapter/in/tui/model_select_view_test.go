package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func seedModelSelectCatalog(m *bubbleModel) {
	m.modelCatalogs.set(model.DefaultProtonmanName, []model.RemoteModel{
		{ID: "deepseek-v4-flash-vision-exp", Name: "DeepSeek V4 Flash Vision"},
		{ID: "glm-5.3-flash", Name: "GLM 5.3 Flash"},
		{ID: "Qwen3.8-Flash", Name: "Qwen 3.8 Flash"},
		{ID: "muse-spark", Name: "Muse Spark"},
		{ID: "MiniMax-M3", Name: "MiniMax M3"},
		{ID: "fixture-six", Name: "Fixture Six"},
	})
}

func TestModelSelectViewLaunchViaSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	seedModelSelectCatalog(bModel)
	bModel.activeModel = "MiniMax-M3"
	bModel.activeProvider = "protonman"

	// 1. Launch via /model
	bModel.executeCommand("/model")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after /model")
	}

	view := bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if len(view.models) == 0 {
		t.Fatal("expected models in catalog")
	}
	// Verify focus is on active model (MiniMax-M3 is 5th model in DefaultProtonmanModels, index 4)
	if view.models[view.index].ID != "MiniMax-M3" {
		t.Fatalf("expected focused model 'MiniMax-M3', got %s", view.models[view.index].ID)
	}

	// 2. Render checks
	rendered := bModel.View()
	if !strings.Contains(rendered, "Select Model") {
		t.Fatalf("expected 'Select Model' in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "✓") {
		t.Fatalf("expected active model checkmark in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "MiniMax-M3") {
		t.Fatalf("expected 'MiniMax-M3' in view, got:\n%s", rendered)
	}

	// 3. Esc closes the view
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal closed after Esc")
	}

	// 4. Test /models alias
	bModel.executeCommand("/models")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after /models")
	}
	bModel.bottom.remove(modelSelectViewID)

	// 5. Test /model select
	bModel.executeCommand("/model select")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after /model select")
	}
}

func TestModelSelectViewToggleKeybinding(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)

	// Press Ctrl+P -> opens modal
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	bModel = updated.(*bubbleModel)
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal open after Ctrl+P")
	}

	// Press Ctrl+P again -> closes modal
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select modal closed after second Ctrl+P")
	}

	// Press Alt+M -> opens modal
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
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
	if view.index != 0 {
		t.Fatalf("expected initial index 0, got %d", view.index)
	}

	// Move down with 'j'
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.index != 1 {
		t.Fatalf("expected index 1 after 'j', got %d", view.index)
	}

	// Move up with 'k'
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.index != 0 {
		t.Fatalf("expected index 0 after 'k', got %d", view.index)
	}

	// Move to item 3 with deterministic navigation.
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	bModel = updated.(*bubbleModel)
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.index != 2 {
		t.Fatalf("expected index 2 after moving down twice, got %d", view.index)
	}
	if view.models[view.index].ID != "Qwen3.8-Flash" {
		t.Fatalf("expected Qwen3.8-Flash at index 2, got %s", view.models[view.index].ID)
	}

	// Press Enter to confirm selection
	t.Setenv("PROTON_HOME", t.TempDir())
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected modelSelectViewID removed on Enter")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd on Enter")
	}

	// Execute cmd and verify modelSelectedMsg
	msg := cmd()
	selectedMsg, ok := msg.(modelSelectedMsg)
	if !ok {
		t.Fatalf("expected modelSelectedMsg, got %T", msg)
	}
	if selectedMsg.modelID != "Qwen3.8-Flash" {
		t.Fatalf("expected selected model 'Qwen3.8-Flash', got: %s", selectedMsg.modelID)
	}

	// Deliver modelSelectedMsg to bModel
	bModel.providers = map[string]config.ProviderConfig{
		"protonman": {
			Name:   "protonman",
			APIKey: "plk_test_123",
		},
	}
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
	t.Setenv("PROTON_HOME", t.TempDir())
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

	// Press 'a' to switch to add provider
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
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
	if !strings.Contains(info, "ctrl+p model") {
		t.Fatalf("expected 'ctrl+p model' in infoView, got: %s", info)
	}

	welcome := bModel.welcomeCard()
	if strings.Contains(welcome, "deepseek-v4-flash-vision-exp") {
		t.Fatalf("welcomeCard duplicated model already shown in status bar: %s", welcome)
	}
}

func TestProviderListSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://protonman.dev/api/v1",
		},
	}
	bModel.activeProvider = "protonman"
	bModel.activeModel = "MiniMax-M3"

	bModel.executeCommand("/provider list")
	rendered := bModel.View()
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
	view.index = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.index != pickerVisibleRows(m.height, maxModelSelectRows) {
		t.Fatalf("pgdown index = %d", view.index)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.index != len(view.models)-1 {
		t.Fatalf("end index = %d, want %d", view.index, len(view.models)-1)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if view.index != 0 {
		t.Fatalf("home index = %d, want 0", view.index)
	}
}
