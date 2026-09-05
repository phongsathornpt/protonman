package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/model"
)

func TestProviderViewLaunchViaSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)

	// Launch via /provider add
	bModel.executeCommand("/provider add")
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected provider modal open after /provider add")
	}

	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.nameInput.Value() != "protonman" {
		t.Fatalf("expected prefilled provider 'protonman', got: %s", view.nameInput.Value())
	}
	if view.endpointInput.Value() != "https://protonman.dev/api/v1" {
		t.Fatalf("expected prefilled endpoint 'https://protonman.dev/api/v1', got: %s", view.endpointInput.Value())
	}

	rendered := bModel.View()
	if !strings.Contains(rendered, "Add Model Provider") {
		t.Fatalf("expected 'Add Model Provider' in rendered view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "https://protonman.dev/api/v1") {
		t.Fatalf("expected endpoint in rendered view, got:\n%s", rendered)
	}
}

func TestProviderViewTabCycleAndEsc(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)

	// Initial focus is on API key (index 2)
	if view.focusIndex != 2 {
		t.Fatalf("expected initial focusIndex 2, got %d", view.focusIndex)
	}

	// Press Tab -> focus index 0 (name)
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyTab})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.focusIndex != 0 {
		t.Fatalf("expected focusIndex 0 after Tab, got %d", view.focusIndex)
	}

	// Press Tab again -> focus index 1 (endpoint)
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyTab})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.focusIndex != 1 {
		t.Fatalf("expected focusIndex 1 after Tab, got %d", view.focusIndex)
	}

	// Press Esc -> modal should close
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(providerViewID) {
		t.Fatal("expected modal closed on Esc")
	}
}

func TestProviderViewFetchAndModelSelectionFlow(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model add")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)

	// Set test API key
	view.apiKeyInput.SetValue("plk_test_mock_key")

	// Press Enter to trigger fetch
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateFetching {
		t.Fatalf("expected state providerStateFetching, got %v", view.state)
	}
	if cmd == nil {
		t.Fatal("expected fetchModelsCmd command returned on Enter")
	}

	// Deliver modelsFetchedMsg
	sampleModels := []model.RemoteModel{
		{ID: "deepseek-v4-flash-vision-exp", Name: "DeepSeek V4 Flash Vision", ContextWindow: 1000000, Features: []string{"vision", "tools"}},
		{ID: "glm-5.3-flash", Name: "GLM-5.3 Flash", ContextWindow: 1048576, Features: []string{"coding", "tools"}},
	}
	updated, _ = bModel.Update(modelsFetchedMsg{
		providerName: "protonman",
		baseURL:      "https://protonman.dev/api/v1",
		apiKey:       "plk_test_mock_key",
		models:       sampleModels,
	})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)

	if view.state != providerStateSelectModel {
		t.Fatalf("expected state providerStateSelectModel, got %v", view.state)
	}
	if len(view.models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(view.models))
	}

	rendered := bModel.View()
	if !strings.Contains(rendered, "deepseek-v4-flash-vision-exp") || !strings.Contains(rendered, "1.0M ctx") {
		t.Fatalf("expected models in view, got:\n%s", rendered)
	}

	// Navigate to item 2 using 'j' or down
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex 1, got %d", view.selectedIndex)
	}

	// Press Enter to select glm-5.3-flash and save
	updated, saveCmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	if saveCmd == nil {
		t.Fatal("expected saveProviderCmd on selection Enter")
	}
	if bModel.bottom.has(providerViewID) {
		t.Fatal("expected modal closed on selection Enter")
	}

	// Deliver providerSavedMsg
	updated, _ = bModel.Update(providerSavedMsg{
		providerName: "protonman",
		baseURL:      "https://protonman.dev/api/v1",
		modelID:      "glm-5.3-flash",
	})
	bModel = updated.(*bubbleModel)

	transcript := bModel.viewport.View()
	if !strings.Contains(transcript, "Configured provider protonman") || !strings.Contains(transcript, "glm-5.3-flash") {
		t.Fatalf("expected confirmation in transcript, got:\n%s", transcript)
	}
}

func TestProviderViewErrorDisplayAndRetry(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")

	// Deliver error in modelsFetchedMsg
	updated, _ := bModel.Update(modelsFetchedMsg{
		providerName: "protonman",
		baseURL:      "https://protonman.dev/api/v1",
		apiKey:       "bad_key",
		err:          errors.New("authentication failed (401): invalid API key"),
	})
	bModel = updated.(*bubbleModel)
	view := bModel.bottom.find(providerViewID).(*providerPaneView)

	if view.state != providerStateError {
		t.Fatalf("expected providerStateError, got %v", view.state)
	}
	rendered := bModel.View()
	if !strings.Contains(rendered, "Connection Failed") || !strings.Contains(rendered, "invalid API key") {
		t.Fatalf("expected error banner in view, got:\n%s", rendered)
	}

	// Press Enter to return to edit form
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateInput {
		t.Fatalf("expected returned to providerStateInput, got %v", view.state)
	}
}

func TestProviderViewOpenCodePresetLaunch(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)

	// Launch via /provider add opencode
	bModel.executeCommand("/provider add opencode")
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected provider modal open after /provider add opencode")
	}

	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.nameInput.Value() != "opencode" {
		t.Fatalf("expected prefilled provider 'opencode', got: %s", view.nameInput.Value())
	}
	if view.endpointInput.Value() != "https://opencode.ai/zen/v1" {
		t.Fatalf("expected prefilled endpoint 'https://opencode.ai/zen/v1', got: %s", view.endpointInput.Value())
	}
	if !strings.Contains(view.apiKeyInput.Placeholder, "Optional") {
		t.Fatalf("expected placeholder with 'Optional', got: %s", view.apiKeyInput.Placeholder)
	}

	rendered := bModel.View()
	if !strings.Contains(rendered, "opencode") || !strings.Contains(rendered, "https://opencode.ai/zen/v1") {
		t.Fatalf("expected opencode in rendered view, got:\n%s", rendered)
	}
}

func TestProviderViewEmptyKeyAllowedForOpenCode(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add opencode")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)

	// Ensure API key is empty
	view.apiKeyInput.SetValue("")

	// Press Enter -> should proceed to fetch even without API key
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)

	if view.state != providerStateFetching {
		t.Fatalf("expected state providerStateFetching for empty key on opencode, got %v", view.state)
	}
	if cmd == nil {
		t.Fatal("expected fetchModelsCmd command returned on Enter")
	}
}

func TestProviderViewFreeBadgeAndFiltering(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add opencode")

	sampleModels := []model.RemoteModel{
		{ID: "nemotron-3.5-lightning-free", Name: "Nemotron 3.5 Lightning (Free)"},
		{ID: "big-pickle", Name: "Big Pickle (Free)"},
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5"},
		{ID: "gpt-5.5", Name: "GPT 5.5"},
	}

	updated, _ := bModel.Update(modelsFetchedMsg{
		providerName: "opencode",
		baseURL:      "https://opencode.ai/zen/v1",
		apiKey:       "",
		models:       sampleModels,
	})
	bModel = updated.(*bubbleModel)
	view := bModel.bottom.find(providerViewID).(*providerPaneView)

	if view.state != providerStateSelectModel {
		t.Fatalf("expected providerStateSelectModel, got %v", view.state)
	}
	// For opencode, filterFreeOnly defaults to true
	if !view.filterFreeOnly {
		t.Fatal("expected filterFreeOnly true by default for opencode")
	}

	rendered := bModel.View()
	if !strings.Contains(rendered, "[FREE]") {
		t.Fatalf("expected [FREE] badge in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "nemotron-3.5-lightning-free") || !strings.Contains(rendered, "big-pickle") {
		t.Fatalf("expected free models in view, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "claude-sonnet-5") {
		t.Fatalf("expected paid models filtered out when filterFreeOnly is true, got:\n%s", rendered)
	}

	// Press 'f' to toggle filter
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.filterFreeOnly {
		t.Fatal("expected filterFreeOnly toggled to false after 'f'")
	}

	renderedAll := bModel.View()
	if !strings.Contains(renderedAll, "claude-sonnet-5") || !strings.Contains(renderedAll, "gpt-5.5") {
		t.Fatalf("expected all models shown after 'f' toggle, got:\n%s", renderedAll)
	}

	// Free models should still have [FREE] badge
	if !strings.Contains(renderedAll, "[FREE]") {
		t.Fatalf("expected [FREE] badge in full view, got:\n%s", renderedAll)
	}
}

func TestProviderViewWindowingWithManyModels(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")

	// 15 models
	manyModels := make([]model.RemoteModel, 15)
	for i := 0; i < 15; i++ {
		manyModels[i] = model.RemoteModel{ID: strings.Repeat("a", i+1)}
	}

	updated, _ := bModel.Update(modelsFetchedMsg{
		providerName: "custom",
		baseURL:      "https://api.custom.com/v1",
		apiKey:       "key",
		models:       manyModels,
	})
	bModel = updated.(*bubbleModel)
	view := bModel.bottom.find(providerViewID).(*providerPaneView)

	if len(view.models) != 15 {
		t.Fatalf("expected 15 models, got %d", len(view.models))
	}

	rendered := bModel.View()
	if !strings.Contains(rendered, "more below") {
		t.Fatalf("expected 'more below' indicator for 15 models, got:\n%s", rendered)
	}

	// Scroll down 8 times
	for i := 0; i < 8; i++ {
		updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyDown})
		bModel = updated.(*bubbleModel)
	}
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.selectedIndex != 8 {
		t.Fatalf("expected selectedIndex 8, got %d", view.selectedIndex)
	}
	if view.scrollOffset == 0 {
		t.Fatalf("expected scrollOffset > 0 after scrolling down past 8 rows, got %d", view.scrollOffset)
	}

	scrolledView := bModel.View()
	if !strings.Contains(scrolledView, "more above") {
		t.Fatalf("expected 'more above' indicator after scrolling down, got:\n%s", scrolledView)
	}
}

func TestSlashCommandModelFree(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)

	bModel.executeCommand("/model free")
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected provider modal open after /model free")
	}

	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.nameInput.Value() != "opencode" {
		t.Fatalf("expected opencode preset from /model free, got: %s", view.nameInput.Value())
	}
}
