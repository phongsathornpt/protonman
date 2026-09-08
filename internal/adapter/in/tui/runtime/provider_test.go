package runtime

import (
	"context"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"strings"
	"testing"
)

func TestProviderSelectViewLaunchViaSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: "https://opencode.ai/zen/v1", Type: "openai"}, "protonman": {Name: "protonman", BaseURL: "https://api.protonman.dev/v1", APIKey: "pm-test-key", Type: "openai"}}
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/provider")
	if !bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal open after /provider")
	}
	view := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if len(view.items) != 6 {
		t.Fatalf("expected 6 items in hub, got %d", len(view.items))
	}
	if view.items[view.index].name != "protonman" {
		t.Fatalf("expected active provider 'protonman' focused, got %s", view.items[view.index].name)
	}
	rendered := bModel.View()
	if !strings.Contains(rendered, "Providers") {
		t.Fatalf("expected provider title in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "✓ Protonman · active") {
		t.Fatalf("expected active provider marker in view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "https://api.protonman.dev/v1") {
		t.Fatalf("expected endpoint in view, got:\n%s", rendered)
	}
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal closed after Esc")
	}
	bModel.executeCommand("/providers")
	if !bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal open after /providers")
	}
	bModel.bottom.remove(providerSelectViewID)
	bModel.executeCommand("/provider select")
	if !bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal open after /provider select")
	}
	bModel.bottom.remove(providerSelectViewID)
	bModel.providers = make(map[string]config.ProviderConfig)
	bModel.executeCommand("/provider")
	if !bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider hub open with presets even when no providers configured")
	}
	viewEmpty := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if len(viewEmpty.items) < 4 {
		t.Fatalf("expected at least 4 presets, got %d", len(viewEmpty.items))
	}
}

func TestProviderSelectViewNavigationAndConfirm(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("PROTON_HOME", tempHome)
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: "https://opencode.ai/zen/v1", Type: "openai"}, "protonman": {Name: "protonman", BaseURL: "https://api.protonman.dev/v1", APIKey: "pm-test-key", Type: "openai"}}
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/provider")
	view := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != 1 {
		t.Fatalf("expected initial index 1, got %d", view.index)
	}
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != 0 {
		t.Fatalf("expected index 0 after Up, got %d", view.index)
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != 1 {
		t.Fatalf("expected index 1 after Down, got %d", view.index)
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != 1 {
		t.Fatalf("number shortcut changed provider index to %d", view.index)
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	bModel = updated.(*bubbleModel)
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	if cmd == nil {
		t.Fatal("expected non-nil cmd on Enter")
	}
	msg := cmd()
	selectedMsg, ok := msg.(providerActiveSelectedMsg)
	if !ok {
		t.Fatalf("expected providerActiveSelectedMsg, got %T", msg)
	}
	if selectedMsg.providerName != "opencode" {
		t.Fatalf("expected provider 'opencode', got %s", selectedMsg.providerName)
	}
	if selectedMsg.err != nil {
		t.Fatalf("unexpected error saving active provider: %v", selectedMsg.err)
	}
	updated, _ = bModel.Update(selectedMsg)
	bModel = updated.(*bubbleModel)
	if bModel.activeProvider != "opencode" {
		t.Fatalf("expected activeProvider 'opencode', got %s", bModel.activeProvider)
	}
	if bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal closed after Enter")
	}
	snap, err := config.Load(context.Background(), config.Options{HomeDir: tempHome})
	if err != nil {
		t.Fatalf("failed to load saved user config: %v", err)
	}
	if snap.Model.Provider != "opencode" {
		t.Fatalf("expected config provider 'opencode', got %s", snap.Model.Provider)
	}
}

func TestProviderSelectViewEditDetails(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"protonman": {Name: "protonman", BaseURL: "https://api.protonman.dev/v1", APIKey: "pm-secret-key-999", Type: "openai"}}
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/provider")
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select view closed")
	}
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected provider view opened in edit mode")
	}
	pv := bModel.bottom.find(providerViewID).(*providerPaneView)
	if !pv.isEditing {
		t.Fatal("expected pv.isEditing to be true")
	}
	if pv.nameInput.Value() != "protonman" {
		t.Fatalf("expected name 'protonman', got %q", pv.nameInput.Value())
	}
	if pv.endpointInput.Value() != "https://api.protonman.dev/v1" {
		t.Fatalf("expected endpoint 'https://api.protonman.dev/v1', got %q", pv.endpointInput.Value())
	}
	if pv.apiKeyInput.Value() != "pm-secret-key-999" {
		t.Fatalf("expected api key prefilled, got %q", pv.apiKeyInput.Value())
	}
	rendered := bModel.View()
	if !strings.Contains(rendered, "Edit Provider: protonman") {
		t.Fatalf("expected 'Edit Provider: protonman' in rendered view, got:\n%s", rendered)
	}
}

func TestProviderSelectViewSetupPreset(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: "https://opencode.ai/zen/v1", Type: "openai"}}
	bModel.executeCommand("/provider")
	view := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	ollamaIdx := -1
	for i, it := range view.items {
		if it.name == "ollama" {
			ollamaIdx = i
			break
		}
	}
	if ollamaIdx == -1 {
		t.Fatal("expected ollama preset in items")
	}
	view.index = ollamaIdx
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected provider view opened for preset setup")
	}
	pv := bModel.bottom.find(providerViewID).(*providerPaneView)
	if pv.nameInput.Value() != "ollama" {
		t.Fatalf("expected name 'ollama', got %q", pv.nameInput.Value())
	}
	if pv.endpointInput.Value() != "http://localhost:11434/v1" {
		t.Fatalf("expected ollama endpoint, got %q", pv.endpointInput.Value())
	}
}

func TestProviderSelectViewDelete(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("PROTON_HOME", tempHome)
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: "https://opencode.ai/zen/v1", Type: "openai"}, "protonman": {Name: "protonman", BaseURL: "https://api.protonman.dev/v1", APIKey: "pm-key", Type: "openai"}}
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/provider")
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	bModel = updated.(*bubbleModel)
	if cmd != nil {
		t.Fatal("expected delete confirmation before running a command")
	}
	view := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if !view.deleteConfirm {
		t.Fatal("expected delete confirmation state after 'd'")
	}
	if !strings.Contains(bModel.View(), "Remove Provider?") || !strings.Contains(bModel.View(), "protonman") {
		t.Fatalf("expected provider delete confirmation in view, got:\n%s", bModel.View())
	}
	updated, cmd = bModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bModel = updated.(*bubbleModel)
	if cmd != nil || bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView).deleteConfirm {
		t.Fatal("expected Esc to cancel delete confirmation")
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	bModel = updated.(*bubbleModel)
	updated, cmd = bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	if cmd == nil {
		t.Fatal("expected delete cmd after confirming with Enter")
	}
	msg := cmd()
	delMsg, ok := msg.(providerDeletedMsg)
	if !ok || delMsg.providerName != "protonman" {
		t.Fatalf("expected providerDeletedMsg for protonman, got %+v", msg)
	}
	updated, _ = bModel.Update(delMsg)
	bModel = updated.(*bubbleModel)
	if _, exists := bModel.providers["protonman"]; exists {
		t.Fatal("expected protonman deleted from bModel.providers")
	}
	if bModel.activeProvider != "opencode" {
		t.Fatalf("expected active provider to switch to opencode, got %q", bModel.activeProvider)
	}
}

func TestProviderSelectDirectSlashCommand(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("PROTON_HOME", tempHome)
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: "https://opencode.ai/zen/v1", Type: "openai"}, "protonman": {Name: "protonman", BaseURL: "https://api.protonman.dev/v1", APIKey: "pm-test-key", Type: "openai"}}
	bModel.activeProvider = "protonman"
	cmd := bModel.executeCommand("/provider opencode")
	if cmd == nil {
		t.Fatal("expected cmd from /provider opencode")
	}
	msg := cmd()
	selectedMsg, ok := msg.(providerActiveSelectedMsg)
	if !ok {
		t.Fatalf("expected providerActiveSelectedMsg, got %T", msg)
	}
	if selectedMsg.providerName != "opencode" {
		t.Fatalf("expected 'opencode', got %s", selectedMsg.providerName)
	}
	updated, _ := bModel.Update(selectedMsg)
	bModel = updated.(*bubbleModel)
	if bModel.activeProvider != "opencode" {
		t.Fatalf("expected activeProvider 'opencode', got %s", bModel.activeProvider)
	}
	bModel.executeCommand("/provider ollama")
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected /provider ollama to launch provider view for unconfigured preset")
	}
	pv := bModel.bottom.find(providerViewID).(*providerPaneView)
	if pv.nameInput.Value() != "ollama" {
		t.Fatalf("expected ollama preset loaded, got %q", pv.nameInput.Value())
	}
	bModel.bottom.remove(providerViewID)
	bModel.executeCommand("/provider non-existent")
	rendered := bModel.View()
	if !strings.Contains(rendered, "unknown provider") {
		t.Fatalf("expected 'unknown provider' error in view, got:\n%s", rendered)
	}
}

func TestProviderSelectSwitchToModels(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: "https://opencode.ai/zen/v1", Type: "openai"}, "protonman": {Name: "protonman", BaseURL: "https://api.protonman.dev/v1", APIKey: "pm-test-key", Type: "openai"}}
	bModel.activeProvider = "opencode"
	bModel.executeCommand("/provider")
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select view removed after pressing 'm'")
	}
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select view opened after pressing 'm'")
	}
}

func TestModelSelectSwitchToProviders(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: "https://opencode.ai/zen/v1", Type: "openai"}, "protonman": {Name: "protonman", BaseURL: "https://api.protonman.dev/v1", APIKey: "pm-test-key", Type: "openai"}}
	bModel.executeCommand("/model")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select view open")
	}
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select view removed after pressing 'p'")
	}
	if !bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select view opened after pressing 'p'")
	}
}

func TestProviderSelectAddShortcut(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"protonman": {Name: "protonman", BaseURL: "https://api.protonman.dev/v1", APIKey: "pm-test-key", Type: "openai"}}
	bModel.executeCommand("/provider")
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select view removed after pressing 'a'")
	}
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected provider add view opened after pressing 'a'")
	}
}

func TestProviderSelectWindowing(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	providers := make(map[string]config.ProviderConfig)
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("provider-%02d", i)
		providers[name] = config.ProviderConfig{Name: name, BaseURL: fmt.Sprintf("https://api-%02d.example.com", i)}
	}
	bModel.providers = providers
	bModel.activeProvider = "provider-01"
	bModel.executeCommand("/provider")
	rendered := bModel.View()
	if !strings.Contains(rendered, "↓") || !strings.Contains(rendered, "more") {
		t.Fatalf("expected downward scroll indicator for 10 providers, got:\n%s", rendered)
	}
	for i := 0; i < 8; i++ {
		updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyDown})
		bModel = updated.(*bubbleModel)
	}
	rendered = bModel.View()
	if !strings.Contains(rendered, "↑") || !strings.Contains(rendered, "more") {
		t.Fatalf("expected upward scroll indicator after scrolling down, got:\n%s", rendered)
	}
}

func TestProviderSelectPagedNavigation(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	providers := make(map[string]config.ProviderConfig)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("provider-%02d", i)
		providers[name] = config.ProviderConfig{Name: name, BaseURL: "https://example.com"}
	}
	m.providers = providers
	m.activeProvider = "provider-00"
	m.resize(40, 14)
	m.executeCommand("/provider")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(*bubbleModel)
	view := m.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != pickerVisibleRows(m.height, maxProviderListRows) {
		t.Fatalf("pgdown index = %d", view.index)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(*bubbleModel)
	view = m.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != len(view.items)-1 {
		t.Fatalf("end index = %d, want %d", view.index, len(view.items)-1)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = updated.(*bubbleModel)
	view = m.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != 0 {
		t.Fatalf("home index = %d, want 0", view.index)
	}
}

func TestProviderViewLaunchViaSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected provider modal open after /provider add")
	}
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.nameInput.Value() != "" {
		t.Fatalf("expected blank provider name, got: %s", view.nameInput.Value())
	}
	if view.endpointInput.Value() != "" {
		t.Fatalf("expected blank endpoint, got: %s", view.endpointInput.Value())
	}
	rendered := bModel.View()
	if !strings.Contains(rendered, "Add Model Provider") {
		t.Fatalf("expected 'Add Model Provider' in rendered view, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "https://api.example.com/v1") {
		t.Fatalf("expected endpoint placeholder in rendered view, got:\n%s", rendered)
	}
}

func TestProviderModalsFitSmallTerminals(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	for _, size := range [][2]int{{80, 24}, {60, 18}, {40, 14}, {24, 12}} {
		bModel.resize(size[0], size[1])
		rendered := view.Render(bModel)
		if got := lipgloss.Width(rendered); got > size[0] {
			t.Errorf("provider form width %d exceeds terminal width %d at %dx%d", got, size[0], size[0], size[1])
		}
		if got := lipgloss.Height(rendered); got > size[1] {
			t.Errorf("provider form height %d exceeds terminal height %d at %dx%d", got, size[1], size[0], size[1])
		}
		if strings.Contains(bModel.View(), "enter send") {
			t.Errorf("provider modal still shows the composer footer at %dx%d", size[0], size[1])
		}
	}
	bModel.bottom.remove(providerViewID)
	bModel.executeCommand("/provider")
	viewHub := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	bModel.resize(40, 14)
	rendered := viewHub.Render(bModel)
	if got := lipgloss.Width(rendered); got > 40 {
		t.Fatalf("provider hub width %d exceeds terminal width 40", got)
	}
	if got := lipgloss.Height(rendered); got > 14 {
		t.Fatalf("provider hub height %d exceeds terminal height 14", got)
	}
}

func TestProviderViewTabCycleAndEsc(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.focusIndex != 0 {
		t.Fatalf("expected initial focusIndex 0, got %d", view.focusIndex)
	}
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyTab})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.focusIndex != 1 {
		t.Fatalf("expected focusIndex 1 after Tab, got %d", view.focusIndex)
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyTab})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.focusIndex != 2 {
		t.Fatalf("expected focusIndex 2 after Tab, got %d", view.focusIndex)
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(providerViewID) {
		t.Fatal("expected modal closed on Esc")
	}
}

func TestProviderViewValidationBeforeFetch(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if cmd != nil {
		t.Fatal("expected no fetch command for an empty draft")
	}
	if view.state != providerStateInput {
		t.Fatalf("expected input state after empty submit, got %v", view.state)
	}
	if view.fieldErrors[providerFieldName] != "required" || view.fieldErrors[providerFieldEndpoint] != "required" {
		t.Fatalf("expected required errors for name and endpoint, got %#v", view.fieldErrors)
	}
	if view.focusIndex != int(providerFieldName) {
		t.Fatalf("expected focus on provider name, got %d", view.focusIndex)
	}
	view.nameInput.SetValue("custom")
	view.endpointInput.SetValue("ftp://provider.example.com/v1")
	view.apiKeyInput.SetValue("key")
	updated, cmd = bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if cmd != nil {
		t.Fatal("expected no fetch command for an invalid endpoint")
	}
	if view.fieldErrors[providerFieldEndpoint] != "use an HTTP(S) URL" {
		t.Fatalf("expected endpoint scheme error, got %#v", view.fieldErrors)
	}
	if view.focusIndex != int(providerFieldEndpoint) {
		t.Fatalf("expected focus on endpoint, got %d", view.focusIndex)
	}
}

func TestProviderViewDuplicateNameConfirmation(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"protonman": {Name: "protonman", BaseURL: "https://protonman.dev/api/v1", APIKey: "existing-key"}}
	bModel.executeCommand("/provider add")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	view.nameInput.SetValue("protonman")
	view.endpointInput.SetValue("https://replacement.example.com/v1")
	view.apiKeyInput.SetValue("replacement-key")
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if cmd != nil {
		t.Fatal("expected overwrite confirmation before fetching")
	}
	if view.state != providerStateConfirmOverwrite {
		t.Fatalf("expected overwrite confirmation state, got %v", view.state)
	}
	if !strings.Contains(bModel.View(), "Provider Already Exists") {
		t.Fatalf("expected overwrite warning in view, got:\n%s", bModel.View())
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateInput {
		t.Fatalf("expected Esc to return to input, got %v", view.state)
	}
}

func TestProviderViewPresetSwitchClearsAPIKey(t *testing.T) {
	view := newProviderPaneViewWithPreset(model.DefaultProtonmanName)
	view.apiKeyInput.SetValue("protonman-key")
	view.applyPreset("openai")
	if view.apiKeyInput.Value() != "" {
		t.Fatalf("expected API key cleared after switching preset, got %q", view.apiKeyInput.Value())
	}
	if !view.requiresAPIKey {
		t.Fatal("expected OpenAI preset to require an API key")
	}
}

func TestProviderViewFetchAndModelSelectionFlow(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/model add")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	view.nameInput.SetValue("protonman")
	view.endpointInput.SetValue("https://protonman.dev/api/v1")
	view.apiKeyInput.SetValue("plk_test_mock_key")
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateFetching {
		t.Fatalf("expected state providerStateFetching, got %v", view.state)
	}
	if cmd == nil {
		t.Fatal("expected fetchModelsCmd command returned on Enter")
	}
	sampleModels := []model.RemoteModel{{ID: "deepseek-v4-flash-vision-exp", Name: "DeepSeek V4 Flash Vision", ContextWindow: 1000000, Features: []string{"vision", "tools"}}, {ID: "glm-5.3-flash", Name: "GLM-5.3 Flash", ContextWindow: 1048576, Features: []string{"coding", "tools"}}}
	updated, _ = bModel.Update(modelsFetchedMsg{providerName: "protonman", baseURL: "https://protonman.dev/api/v1", apiKey: "plk_test_mock_key", models: sampleModels, requestID: view.fetchRequestID})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateSelectModel {
		t.Fatalf("expected state providerStateSelectModel, got %v", view.state)
	}
	if len(view.models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(view.models))
	}
	rendered := bModel.View()
	if !strings.Contains(rendered, "deepseek-v4-flash-vision-exp") || !strings.Contains(rendered, "1.0M context") {
		t.Fatalf("expected models in view, got:\n%s", rendered)
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex 1, got %d", view.selectedIndex)
	}
	updated, saveCmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	if saveCmd == nil {
		t.Fatal("expected saveProviderCmd on selection Enter")
	}
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateSaving {
		t.Fatalf("expected saving state on selection Enter, got %v", view.state)
	}
	updated, _ = bModel.Update(providerSavedMsg{providerName: "protonman", baseURL: "https://protonman.dev/api/v1", modelID: "glm-5.3-flash", activated: true})
	bModel = updated.(*bubbleModel)
	transcript := bModel.viewport.View()
	if !strings.Contains(transcript, "Configured provider protonman") || !strings.Contains(transcript, "glm-5.3-flash") {
		t.Fatalf("expected confirmation in transcript, got:\n%s", transcript)
	}
}

func TestProviderViewInactiveEditKeepsActiveProvider(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("PROTON_HOME", homeDir)
	if err := config.SaveUserProviderConfig(homeDir, config.ProviderConfig{Name: "opencode", Type: "openai", BaseURL: "https://opencode.ai/zen/v1"}, "free-model"); err != nil {
		t.Fatalf("save active provider fixture: %v", err)
	}
	if err := config.SaveUserProviderConfigWithOptions(homeDir, config.ProviderConfig{Name: "protonman", Type: "openai", BaseURL: "https://protonman.dev/api/v1", APIKey: "old-key"}, config.ProviderSaveOptions{}); err != nil {
		t.Fatalf("save inactive provider fixture: %v", err)
	}
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{"opencode": {Name: "opencode", BaseURL: "https://opencode.ai/zen/v1"}, "protonman": {Name: "protonman", BaseURL: "https://protonman.dev/api/v1", APIKey: "old-key"}}
	bModel.activeProvider = "opencode"
	bModel.activeModel = "free-model"
	bModel.executeCommand("/provider")
	hub := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	for i, item := range hub.items {
		if item.name == "protonman" {
			hub.index = i
			break
		}
	}
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	bModel = updated.(*bubbleModel)
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.activateOnSave {
		t.Fatal("expected editing an inactive provider to preserve the active provider")
	}
	if !strings.Contains(bModel.View(), "active provider stays") {
		t.Fatalf("expected inactive edit hint in view, got:\n%s", bModel.View())
	}
	view.endpointInput.SetValue("https://protonman.dev/v2")
	view.apiKeyInput.SetValue("new-key")
	view.state = providerStateSelectModel
	view.models = []model.RemoteModel{{ID: "unused-model", Name: "Unused Model"}}
	updated, saveCmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	if saveCmd == nil || view.state != providerStateSaving {
		t.Fatalf("expected inactive edit to enter saving state, got state=%v cmd=%v", view.state, saveCmd != nil)
	}
	updated, _ = bModel.Update(saveCmd())
	bModel = updated.(*bubbleModel)
	if bModel.activeProvider != "opencode" || bModel.activeModel != "free-model" {
		t.Fatalf("inactive provider edit changed active defaults: provider=%q model=%q", bModel.activeProvider, bModel.activeModel)
	}
	if got := bModel.providers["protonman"]; got.BaseURL != "https://protonman.dev/v2" || got.APIKey != "new-key" {
		t.Fatalf("expected provider details to update, got %+v", got)
	}
	snapshot, err := config.Load(context.Background(), config.Options{HomeDir: homeDir, WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("load saved inactive edit: %v", err)
	}
	if snapshot.Model.Provider != "opencode" || snapshot.Model.Default != "free-model" {
		t.Fatalf("inactive edit changed persisted active defaults: %+v", snapshot.Model)
	}
	if !strings.Contains(bModel.viewport.View(), "Active provider remains opencode") {
		t.Fatalf("expected active provider preservation message, got:\n%s", bModel.viewport.View())
	}
}

func TestProviderViewSaveFailureKeepsPane(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	view.state = providerStateSaving
	view.selectedModel = "gpt-5"
	view.nameInput.SetValue("custom")
	view.endpointInput.SetValue("https://api.example.com/v1")
	view.apiKeyInput.SetValue("key")
	updated, _ := bModel.Update(providerSavedMsg{err: errors.New("permission denied")})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateSaveError {
		t.Fatalf("expected save error state, got %v", view.state)
	}
	if view.nameInput.Value() != "custom" || view.apiKeyInput.Value() != "key" {
		t.Fatal("expected provider draft to remain after save failure")
	}
	if !strings.Contains(bModel.View(), "permission denied") {
		t.Fatalf("expected save error in view, got:\n%s", bModel.View())
	}
	updated, cmd := bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if cmd == nil || view.state != providerStateSaving {
		t.Fatalf("expected retry to enter saving state, got state=%v cmd=%v", view.state, cmd != nil)
	}
}

func TestProviderFetchInheritsParentCancellation(t *testing.T) {
	view := newProviderPaneView()
	view.nameInput.SetValue("custom")
	view.endpointInput.SetValue("http://127.0.0.1:1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	msg := view.beginFetch(ctx)()
	fetched, ok := msg.(modelsFetchedMsg)
	if !ok {
		t.Fatalf("fetch message = %T, want modelsFetchedMsg", msg)
	}
	if fetched.err == nil || !errors.Is(fetched.err, context.Canceled) {
		t.Fatalf("fetch error = %v, want context canceled", fetched.err)
	}
}

func TestProviderViewIgnoresStaleFetchResults(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	view.nameInput.SetValue("custom")
	view.endpointInput.SetValue("https://api.example.com/v1")
	view.beginFetch(context.Background())
	firstRequestID := view.fetchRequestID
	view.beginFetch(context.Background())
	secondRequestID := view.fetchRequestID
	if secondRequestID <= firstRequestID {
		t.Fatalf("expected fetch request ID to advance, got %d then %d", firstRequestID, secondRequestID)
	}
	updated, _ := bModel.Update(modelsFetchedMsg{providerName: "custom", baseURL: "https://api.example.com/v1", models: []model.RemoteModel{{ID: "stale-model"}}, requestID: firstRequestID})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateFetching || len(view.models) != 0 {
		t.Fatalf("stale fetch result changed pane: state=%v models=%v", view.state, view.models)
	}
	updated, _ = bModel.Update(modelsFetchedMsg{providerName: "custom", baseURL: "https://api.example.com/v1", models: []model.RemoteModel{{ID: "current-model"}}, requestID: secondRequestID})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateSelectModel || len(view.models) != 1 || view.models[0].ID != "current-model" {
		t.Fatalf("current fetch result was not applied: state=%v models=%v", view.state, view.models)
	}
}

func TestProviderViewErrorDisplayAndRetry(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	updated, _ := bModel.Update(modelsFetchedMsg{providerName: "protonman", baseURL: "https://protonman.dev/api/v1", apiKey: "bad_key", err: errors.New("authentication failed (401): invalid API key")})
	bModel = updated.(*bubbleModel)
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateError {
		t.Fatalf("expected providerStateError, got %v", view.state)
	}
	rendered := bModel.View()
	if !strings.Contains(rendered, "Connection Failed") || !strings.Contains(rendered, "invalid API key") {
		t.Fatalf("expected error banner in view, got:\n%s", rendered)
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateInput {
		t.Fatalf("expected returned to providerStateInput, got %v", view.state)
	}
}

func TestProviderViewOpenCodePresetLaunch(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
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
	if !strings.Contains(strings.ToLower(view.apiKeyInput.Placeholder), "optional") {
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
	view.apiKeyInput.SetValue("")
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
	sampleModels := []model.RemoteModel{{ID: "nemotron-3.5-lightning-free", Name: "Nemotron 3.5 Lightning (Free)"}, {ID: "big-pickle", Name: "Big Pickle (Free)"}, {ID: "claude-sonnet-5", Name: "Claude Sonnet 5"}, {ID: "gpt-5.5", Name: "GPT 5.5"}}
	updated, _ := bModel.Update(modelsFetchedMsg{providerName: "opencode", baseURL: "https://opencode.ai/zen/v1", apiKey: "", models: sampleModels})
	bModel = updated.(*bubbleModel)
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if view.state != providerStateSelectModel {
		t.Fatalf("expected providerStateSelectModel, got %v", view.state)
	}
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
	if !strings.Contains(renderedAll, "[FREE]") {
		t.Fatalf("expected [FREE] badge in full view, got:\n%s", renderedAll)
	}
}

func TestProviderViewWindowingWithManyModels(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.executeCommand("/provider add")
	manyModels := make([]model.RemoteModel, 15)
	for i := 0; i < 15; i++ {
		manyModels[i] = model.RemoteModel{ID: strings.Repeat("a", i+1)}
	}
	updated, _ := bModel.Update(modelsFetchedMsg{providerName: "custom", baseURL: "https://api.custom.com/v1", apiKey: "key", models: manyModels})
	bModel = updated.(*bubbleModel)
	view := bModel.bottom.find(providerViewID).(*providerPaneView)
	if len(view.models) != 15 {
		t.Fatalf("expected 15 models, got %d", len(view.models))
	}
	rendered := bModel.View()
	if !strings.Contains(rendered, "more below") {
		t.Fatalf("expected 'more below' indicator for 15 models, got:\n%s", rendered)
	}
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

func TestAnthropicProviderPanePreservesProtocol(t *testing.T) {
	view := newProviderPaneViewWithPreset(model.DefaultAnthropicName)
	if view.providerType != string(model.ProviderProtocolAnthropic) {
		t.Fatalf("providerType = %q, want anthropic", view.providerType)
	}
	configured := newProviderPaneViewWithConfig(config.ProviderConfig{Name: "custom-claude", Type: string(model.ProviderProtocolAnthropic), BaseURL: "https://anthropic.example", APIKey: "key"})
	if configured.providerType != string(model.ProviderProtocolAnthropic) {
		t.Fatalf("configured providerType = %q, want anthropic", configured.providerType)
	}
}

func TestAnthropicProviderKeyPlaceholder(t *testing.T) {
	view := newProviderPaneViewWithPreset(model.DefaultAnthropicName)
	if got := view.apiKeyInput.Placeholder; got != "sk-ant-…" {
		t.Fatalf("Anthropic API key placeholder = %q, want %q", got, "sk-ant-…")
	}
}

func TestCustomProviderCanToggleProtocol(t *testing.T) {
	view := newProviderPaneView()
	if view.providerType != string(model.ProviderProtocolOpenAI) {
		t.Fatalf("initial providerType = %q", view.providerType)
	}
	view.toggleProtocol()
	if view.providerType != string(model.ProviderProtocolAnthropic) {
		t.Fatalf("toggled providerType = %q, want anthropic", view.providerType)
	}
	view.toggleProtocol()
	if view.providerType != string(model.ProviderProtocolOpenAI) {
		t.Fatalf("second toggle providerType = %q, want openai", view.providerType)
	}
}

func TestPresetProviderProtocolCannotToggle(t *testing.T) {
	view := newProviderPaneViewWithPreset(model.DefaultAnthropicName)
	view.toggleProtocol()
	if view.providerType != string(model.ProviderProtocolAnthropic) {
		t.Fatalf("preset providerType = %q, want anthropic", view.providerType)
	}
}

func TestProviderSavedMessagePreservesAnthropicTypeInMemory(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{}
	updated, _ := bModel.Update(providerSavedMsg{providerName: "custom-claude", providerType: string(model.ProviderProtocolAnthropic), baseURL: "https://anthropic.example", apiKey: "key", modelID: "claude-test"})
	bModel = updated.(*bubbleModel)
	got := bModel.providers["custom-claude"]
	if got.Type != string(model.ProviderProtocolAnthropic) {
		t.Fatalf("in-memory provider type = %q, want anthropic", got.Type)
	}
}
