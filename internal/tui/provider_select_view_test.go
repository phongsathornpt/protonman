package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/config"
)

func TestProviderSelectViewLaunchViaSlashCommand(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{
		"opencode": {
			Name:    "opencode",
			BaseURL: "https://opencode.ai/zen/v1",
			Type:    "openai",
		},
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://api.protonman.dev/v1",
			APIKey:  "pm-test-key",
			Type:    "openai",
		},
	}
	bModel.activeProvider = "protonman"

	// 1. Launch via /provider
	bModel.executeCommand("/provider")
	if !bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal open after /provider")
	}

	view := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	// 2 configured + 2 remaining presets (ollama, openai) + 1 custom = 5 items
	if len(view.items) != 5 {
		t.Fatalf("expected 5 items in hub, got %d", len(view.items))
	}
	// Protonman should be selected because it is the active provider
	if view.items[view.index].name != "protonman" {
		t.Fatalf("expected active provider 'protonman' focused, got %s", view.items[view.index].name)
	}

	// 2. Render checks
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

	// 3. Esc closes the view
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bModel = updated.(*bubbleModel)
	if bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal closed after Esc")
	}

	// 4. Test /providers alias
	bModel.executeCommand("/providers")
	if !bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal open after /providers")
	}
	bModel.bottom.remove(providerSelectViewID)

	// 5. Test /provider select
	bModel.executeCommand("/provider select")
	if !bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal open after /provider select")
	}
	bModel.bottom.remove(providerSelectViewID)

	// 6. Test /provider when no providers configured opens hub with supported presets
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
	bModel.providers = map[string]config.ProviderConfig{
		"opencode": {
			Name:    "opencode",
			BaseURL: "https://opencode.ai/zen/v1",
			Type:    "openai",
		},
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://api.protonman.dev/v1",
			APIKey:  "pm-test-key",
			Type:    "openai",
		},
	}
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/provider")

	view := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	// Currently at index 1 ("protonman")
	if view.index != 1 {
		t.Fatalf("expected initial index 1, got %d", view.index)
	}

	// Up key -> moves to index 0 ("opencode")
	updated, _ := bModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != 0 {
		t.Fatalf("expected index 0 after Up, got %d", view.index)
	}

	// Down key -> moves back to index 1 ("protonman")
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != 1 {
		t.Fatalf("expected index 1 after Down, got %d", view.index)
	}

	// Number shortcuts are intentionally ignored so list navigation stays positional.
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	bModel = updated.(*bubbleModel)
	view = bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	if view.index != 1 {
		t.Fatalf("number shortcut changed provider index to %d", view.index)
	}
	updated, _ = bModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	bModel = updated.(*bubbleModel)

	// Press Enter to confirm switch to "opencode"
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

	// Pass message into model update
	updated, _ = bModel.Update(selectedMsg)
	bModel = updated.(*bubbleModel)
	if bModel.activeProvider != "opencode" {
		t.Fatalf("expected activeProvider 'opencode', got %s", bModel.activeProvider)
	}
	if bModel.bottom.has(providerSelectViewID) {
		t.Fatal("expected provider select modal closed after Enter")
	}

	// Verify persistence in config file
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
	bModel.providers = map[string]config.ProviderConfig{
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://api.protonman.dev/v1",
			APIKey:  "pm-secret-key-999",
			Type:    "openai",
		},
	}
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/provider")

	// Press 'e' on protonman to edit details
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
	bModel.providers = map[string]config.ProviderConfig{
		"opencode": {
			Name:    "opencode",
			BaseURL: "https://opencode.ai/zen/v1",
			Type:    "openai",
		},
	}
	bModel.executeCommand("/provider")

	view := bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView)
	// Find index of unconfigured preset "ollama"
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
	// Press Enter on unconfigured preset -> opens setup with ollama preset
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
	bModel.providers = map[string]config.ProviderConfig{
		"opencode": {
			Name:    "opencode",
			BaseURL: "https://opencode.ai/zen/v1",
			Type:    "openai",
		},
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://api.protonman.dev/v1",
			APIKey:  "pm-key",
			Type:    "openai",
		},
	}
	bModel.activeProvider = "protonman"
	bModel.executeCommand("/provider")

	// Focus is on protonman (index 1). Press 'd' to open the confirmation.
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

	// Esc cancels without changing the provider list.
	updated, cmd = bModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bModel = updated.(*bubbleModel)
	if cmd != nil || bModel.bottom.find(providerSelectViewID).(*providerSelectPaneView).deleteConfirm {
		t.Fatal("expected Esc to cancel delete confirmation")
	}

	// Open it again, then press Enter to perform the deletion.
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
	bModel.providers = map[string]config.ProviderConfig{
		"opencode": {
			Name:    "opencode",
			BaseURL: "https://opencode.ai/zen/v1",
			Type:    "openai",
		},
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://api.protonman.dev/v1",
			APIKey:  "pm-test-key",
			Type:    "openai",
		},
	}
	bModel.activeProvider = "protonman"

	// 1. Switch directly via /provider opencode
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

	// 2. Unconfigured preset /provider ollama opens preset setup
	bModel.executeCommand("/provider ollama")
	if !bModel.bottom.has(providerViewID) {
		t.Fatal("expected /provider ollama to launch provider view for unconfigured preset")
	}
	pv := bModel.bottom.find(providerViewID).(*providerPaneView)
	if pv.nameInput.Value() != "ollama" {
		t.Fatalf("expected ollama preset loaded, got %q", pv.nameInput.Value())
	}
	bModel.bottom.remove(providerViewID)

	// 3. Unknown provider returns error line
	bModel.executeCommand("/provider non-existent")
	rendered := bModel.View()
	if !strings.Contains(rendered, "unknown provider") {
		t.Fatalf("expected 'unknown provider' error in view, got:\n%s", rendered)
	}
}

func TestProviderSelectSwitchToModels(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)
	bModel.providers = map[string]config.ProviderConfig{
		"opencode": {
			Name:    "opencode",
			BaseURL: "https://opencode.ai/zen/v1",
			Type:    "openai",
		},
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://api.protonman.dev/v1",
			APIKey:  "pm-test-key",
			Type:    "openai",
		},
	}
	bModel.activeProvider = "opencode"
	bModel.executeCommand("/provider")

	// Press 'm' to open models pane for selected provider
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
	bModel.providers = map[string]config.ProviderConfig{
		"opencode": {
			Name:    "opencode",
			BaseURL: "https://opencode.ai/zen/v1",
			Type:    "openai",
		},
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://api.protonman.dev/v1",
			APIKey:  "pm-test-key",
			Type:    "openai",
		},
	}
	bModel.executeCommand("/model")
	if !bModel.bottom.has(modelSelectViewID) {
		t.Fatal("expected model select view open")
	}

	// Press 'p' in model select pane to switch to provider select pane
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
	bModel.providers = map[string]config.ProviderConfig{
		"protonman": {
			Name:    "protonman",
			BaseURL: "https://api.protonman.dev/v1",
			APIKey:  "pm-test-key",
			Type:    "openai",
		},
	}
	bModel.executeCommand("/provider")

	// Press 'a' to switch to add provider pane
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
		providers[name] = config.ProviderConfig{
			Name:    name,
			BaseURL: fmt.Sprintf("https://api-%02d.example.com", i),
		}
	}
	bModel.providers = providers
	bModel.activeProvider = "provider-01"
	bModel.executeCommand("/provider")

	rendered := bModel.View()
	if !strings.Contains(rendered, "↓") || !strings.Contains(rendered, "more") {
		t.Fatalf("expected downward scroll indicator for 10 providers, got:\n%s", rendered)
	}

	// Move down 8 times
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
