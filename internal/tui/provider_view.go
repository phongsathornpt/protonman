package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
)

const providerViewID = "add_provider"

type providerPaneState int

const (
	providerStateInput providerPaneState = iota
	providerStateFetching
	providerStateSelectModel
	providerStateError
)

type modelsFetchedMsg struct {
	providerName string
	baseURL      string
	apiKey       string
	models       []model.RemoteModel
	err          error
}

type providerSavedMsg struct {
	providerName string
	baseURL      string
	apiKey       string
	modelID      string
	err          error
}

const maxProviderSelectRows = 8

type providerPaneView struct {
	state          providerPaneState
	focusIndex     int
	isEditing      bool
	nameInput      textinput.Model
	endpointInput  textinput.Model
	apiKeyInput    textinput.Model
	models         []model.RemoteModel
	selectedIndex  int
	scrollOffset   int
	filterFreeOnly bool
	errorMessage   string
}

func newProviderPaneView() *providerPaneView {
	return newProviderPaneViewWithPreset(model.DefaultProtonmanName)
}

func newProviderPaneViewWithConfig(cfg config.ProviderConfig) *providerPaneView {
	pv := newProviderPaneViewWithPreset(cfg.Name)
	pv.isEditing = true
	if cfg.Name != "" {
		pv.nameInput.SetValue(cfg.Name)
	}
	if cfg.BaseURL != "" {
		pv.endpointInput.SetValue(cfg.BaseURL)
	}
	if cfg.APIKey != "" {
		pv.apiKeyInput.SetValue(cfg.APIKey)
	}
	return pv
}

func newProviderPaneViewWithPreset(preset string) *providerPaneView {
	preset = strings.ToLower(strings.TrimSpace(preset))
	name := model.DefaultProtonmanName
	endpoint := model.DefaultProtonmanEndpoint
	keyPlaceholder := "plk_live_..."
	filterFree := false

	if p := model.LookupPreset(preset); p != nil {
		name = p.ID
		endpoint = p.BaseURL
		if !p.RequiresKey {
			keyPlaceholder = "(Optional for " + p.Name + ")"
		} else if p.ID == "openai" {
			keyPlaceholder = "sk-..."
		}
		if p.ID == model.DefaultOpenCodeName {
			filterFree = true
		}
	} else if preset == "opencode-free" || preset == "free" {
		name = model.DefaultOpenCodeName
		endpoint = model.DefaultOpenCodeEndpoint
		keyPlaceholder = "(Optional for free models)"
		filterFree = true
	}

	nameIn := textinput.New()
	nameIn.Prompt = glyphPrompt
	nameIn.Placeholder = name
	nameIn.SetValue(name)
	nameIn.CharLimit = 64

	endpointIn := textinput.New()
	endpointIn.Prompt = glyphPrompt
	endpointIn.Placeholder = endpoint
	endpointIn.SetValue(endpoint)
	endpointIn.CharLimit = 256

	keyIn := textinput.New()
	keyIn.Prompt = glyphPrompt
	keyIn.Placeholder = keyPlaceholder
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.EchoCharacter = '•'
	keyIn.CharLimit = 256
	keyIn.Focus()

	return &providerPaneView{
		state:          providerStateInput,
		focusIndex:     2, // Start with focus on API key
		nameInput:      nameIn,
		endpointInput:  endpointIn,
		apiKeyInput:    keyIn,
		filterFreeOnly: filterFree,
	}
}

func (*providerPaneView) ID() string             { return providerViewID }
func (*providerPaneView) ReplacesComposer() bool { return true }

func (v *providerPaneView) isZeroKeyAllowed() bool {
	ep := strings.ToLower(v.endpointInput.Value())
	name := strings.ToLower(strings.TrimSpace(v.nameInput.Value()))
	return strings.Contains(ep, "opencode.ai") ||
		strings.Contains(ep, "localhost") ||
		strings.Contains(ep, "127.0.0.1") ||
		name == model.DefaultOpenCodeName ||
		name == "ollama" ||
		name == "local"
}

func (v *providerPaneView) isOpenCode() bool {
	return strings.Contains(strings.ToLower(v.endpointInput.Value()), "opencode.ai") ||
		strings.EqualFold(strings.TrimSpace(v.nameInput.Value()), model.DefaultOpenCodeName)
}

func (v *providerPaneView) applyPreset(preset string) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	if preset == "1" {
		preset = model.DefaultProtonmanName
	} else if preset == "2" {
		preset = model.DefaultOpenCodeName
	} else if preset == "3" {
		preset = "ollama"
	} else if preset == "4" {
		preset = "openai"
	}

	if p := model.LookupPreset(preset); p != nil {
		v.nameInput.SetValue(p.ID)
		v.endpointInput.SetValue(p.BaseURL)
		if !p.RequiresKey {
			v.apiKeyInput.Placeholder = "(Optional for " + p.Name + ")"
		} else if p.ID == "openai" {
			v.apiKeyInput.Placeholder = "sk-..."
		} else {
			v.apiKeyInput.Placeholder = "plk_live_..."
		}
		v.filterFreeOnly = (p.ID == model.DefaultOpenCodeName)
	}
}

func (v *providerPaneView) setFetchedModels(models []model.RemoteModel) {
	if v.isOpenCode() {
		freeList := make([]model.RemoteModel, 0)
		paidList := make([]model.RemoteModel, 0)
		for _, m := range models {
			if model.IsFreeModel(m.ID) {
				freeList = append(freeList, m)
			} else {
				paidList = append(paidList, m)
			}
		}
		v.models = append(freeList, paidList...)
		v.filterFreeOnly = len(freeList) > 0
	} else {
		v.models = models
		v.filterFreeOnly = false
	}
	v.selectedIndex = 0
	v.scrollOffset = 0
}

func (v *providerPaneView) currentModels() []model.RemoteModel {
	if !v.filterFreeOnly {
		return v.models
	}
	var filtered []model.RemoteModel
	for _, m := range v.models {
		if model.IsFreeModel(m.ID) {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		return v.models
	}
	return filtered
}

func (v *providerPaneView) Render(m *bubbleModel) string {
	maxWidth := maxInt(1, m.width-4)

	switch v.state {
	case providerStateFetching:
		rows := []string{
			brandStyle.Render("◆ Connecting to " + v.nameInput.Value()),
			"",
			fmt.Sprintf("  %s Querying %s/models...", m.spinner.View(), v.endpointInput.Value()),
			mutedStyle.Render("  Validating proxy key & discovering model catalog"),
			"",
			mutedStyle.Render("esc cancel"),
		}
		return modalStyle.
			BorderForeground(accentAssistant).
			MaxWidth(maxWidth).
			Render(strings.Join(rows, "\n"))

	case providerStateSelectModel:
		models := v.currentModels()
		hasFreeModels := false
		for _, md := range v.models {
			if model.IsFreeModel(md.ID) {
				hasFreeModels = true
				break
			}
		}

		title := fmt.Sprintf("✓ Select Active Model (%d discovered) [Step 2/2]", len(models))
		if hasFreeModels {
			if v.filterFreeOnly {
				title = fmt.Sprintf("✓ Select Active Model (%d free models · [f] show all %d) [Step 2/2]", len(models), len(v.models))
			} else {
				title = fmt.Sprintf("✓ Select Active Model (%d discovered · [f] show free only) [Step 2/2]", len(v.models))
			}
		}

		if len(models) == 0 {
			rows := []string{
				brandStyle.Render(title),
				"",
				mutedStyle.Render("No matching models found."),
				"",
				mutedStyle.Render("f toggle filter · esc back"),
			}
			return modalStyle.
				BorderForeground(accentUser).
				MaxWidth(maxWidth).
				Render(strings.Join(rows, "\n"))
		}

		if v.selectedIndex >= len(models) {
			v.selectedIndex = len(models) - 1
		}
		if v.selectedIndex < 0 {
			v.selectedIndex = 0
		}

		// Dynamic scroll windowing
		if v.selectedIndex < v.scrollOffset {
			v.scrollOffset = v.selectedIndex
		}
		if v.selectedIndex >= v.scrollOffset+maxProviderSelectRows {
			v.scrollOffset = v.selectedIndex - maxProviderSelectRows + 1
		}
		if v.scrollOffset > len(models)-maxProviderSelectRows {
			v.scrollOffset = len(models) - maxProviderSelectRows
		}
		if v.scrollOffset < 0 {
			v.scrollOffset = 0
		}

		visibleEnd := v.scrollOffset + maxProviderSelectRows
		if visibleEnd > len(models) {
			visibleEnd = len(models)
		}
		visible := models[v.scrollOffset:visibleEnd]

		rows := []string{
			brandStyle.Render(title),
			"",
		}

		if v.scrollOffset > 0 {
			rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▲ %d more above", v.scrollOffset)))
		}

		for i, md := range visible {
			idx := v.scrollOffset + i
			prefix := "    "
			if idx == v.selectedIndex {
				prefix = brandStyle.Render("  ❯ ")
			}
			line := fmt.Sprintf("%d. %s", idx+1, md.ID)
			if model.IsFreeModel(md.ID) {
				line += " " + successStyle.Render("[FREE]")
			}
			if md.ContextWindow > 0 {
				line += fmt.Sprintf(" [%s ctx]", formatContextTokens(md.ContextWindow))
			}
			if len(md.Features) > 0 {
				line += fmt.Sprintf(" (%s)", strings.Join(md.Features, ", "))
			}
			if idx == v.selectedIndex {
				rows = append(rows, prefix+brandStyle.Render(line))
			} else {
				rows = append(rows, prefix+mutedStyle.Render(line))
			}
		}

		if visibleEnd < len(models) {
			rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▼ %d more below", len(models)-visibleEnd)))
		}

		footer := "↑/↓ or j/k move · 1-9 select · enter confirm & save · esc back"
		if hasFreeModels {
			footer = "↑/↓ move · 1-9 select · f toggle free only · enter confirm · esc back"
		}
		rows = append(rows, "", mutedStyle.Render(footer))
		return modalStyle.
			BorderForeground(accentUser).
			MaxWidth(maxWidth).
			Render(strings.Join(rows, "\n"))

	case providerStateError:
		rows := []string{
			errorStyle.Render("✕ Connection Failed"),
			"",
			"  " + v.errorMessage,
			"",
			mutedStyle.Render("enter / esc return to credentials"),
		}
		return modalStyle.
			BorderForeground(accentError).
			MaxWidth(maxWidth).
			Render(strings.Join(rows, "\n"))

	case providerStateInput:
		fallthrough
	default:
		keyLabel := "API Key (shown masked):"
		if v.isZeroKeyAllowed() {
			keyLabel = "API Key (optional):"
		}

		title := "◆ Add Model Provider [Step 1/2: Credentials]"
		if v.isEditing {
			title = fmt.Sprintf("✓ Edit Provider: %s [Step 1/2: Credentials]", v.nameInput.Value())
		}

		rows := []string{
			brandStyle.Render(title),
			"",
			mutedStyle.Render("Presets: [alt+1] Protonman · [alt+2] OpenCode (Free) · [alt+3] Ollama (Local) · [alt+4] OpenAI"),
			"",
			mutedStyle.Render("Provider Name:"),
			v.nameInput.View(),
			"",
			mutedStyle.Render("Endpoint (Base URL):"),
			v.endpointInput.View(),
			"",
			mutedStyle.Render(keyLabel),
			v.apiKeyInput.View(),
			"",
			mutedStyle.Render("tab/shift+tab cycle · enter connect & fetch · esc cancel"),
		}
		return modalStyle.
			BorderForeground(accentAssistant).
			MaxWidth(maxWidth).
			Render(strings.Join(rows, "\n"))
	}
}

func (v *providerPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch v.state {
	case providerStateFetching:
		if message.String() == "esc" || message.String() == "ctrl+c" {
			m.bottom.remove(providerViewID)
			return true, nil
		}
		return true, nil

	case providerStateSelectModel:
		models := v.currentModels()
		switch message.String() {
		case "esc", "ctrl+c":
			v.state = providerStateInput
			v.focusIndex = 2
			v.apiKeyInput.Focus()
			return true, nil
		case "f":
			if v.isOpenCode() {
				v.filterFreeOnly = !v.filterFreeOnly
				v.selectedIndex = 0
				v.scrollOffset = 0
			}
			return true, nil
		case "up", "k":
			if len(models) > 0 {
				v.selectedIndex = (v.selectedIndex - 1 + len(models)) % len(models)
			}
			return true, nil
		case "down", "j":
			if len(models) > 0 {
				v.selectedIndex = (v.selectedIndex + 1) % len(models)
			}
			return true, nil
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			num := int(message.Runes[0] - '1')
			if num >= 0 && num < len(models) {
				v.selectedIndex = num
			}
			return true, nil
		case "enter":
			if len(models) > 0 && v.selectedIndex >= 0 && v.selectedIndex < len(models) {
				selected := models[v.selectedIndex]
				cmd := saveProviderCmd(
					v.nameInput.Value(),
					v.endpointInput.Value(),
					v.apiKeyInput.Value(),
					selected.ID,
				)
				m.bottom.remove(providerViewID)
				return true, cmd
			}
			return true, nil
		default:
			return true, nil
		}

	case providerStateError:
		switch message.String() {
		case "enter", "esc":
			v.state = providerStateInput
			v.focusIndex = 2
			v.apiKeyInput.Focus()
			return true, nil
		default:
			return true, nil
		}

	case providerStateInput:
		fallthrough
	default:
		switch message.String() {
		case "esc", "ctrl+c":
			m.bottom.remove(providerViewID)
			return true, nil
		case "alt+1", "alt+p":
			v.applyPreset(model.DefaultProtonmanName)
			return true, nil
		case "alt+2", "alt+o":
			v.applyPreset(model.DefaultOpenCodeName)
			return true, nil
		case "alt+3", "alt+l":
			v.applyPreset("ollama")
			return true, nil
		case "alt+4":
			v.applyPreset("openai")
			return true, nil
		case "tab", "down":
			v.focusIndex = (v.focusIndex + 1) % 3
			v.syncInputFocus()
			return true, nil
		case "shift+tab", "up":
			v.focusIndex = (v.focusIndex + 2) % 3
			v.syncInputFocus()
			return true, nil
		case "enter":
			key := strings.TrimSpace(v.apiKeyInput.Value())
			if key == "" && !v.isZeroKeyAllowed() {
				v.focusIndex = 2
				v.syncInputFocus()
				return true, nil
			}
			v.state = providerStateFetching
			return true, fetchModelsCmd(
				strings.TrimSpace(v.nameInput.Value()),
				strings.TrimSpace(v.endpointInput.Value()),
				key,
			)
		default:
			var cmd tea.Cmd
			switch v.focusIndex {
			case 0:
				v.nameInput, cmd = v.nameInput.Update(message)
			case 1:
				v.endpointInput, cmd = v.endpointInput.Update(message)
			case 2:
				v.apiKeyInput, cmd = v.apiKeyInput.Update(message)
			}
			return true, cmd
		}
	}
}

func (v *providerPaneView) syncInputFocus() {
	v.nameInput.Blur()
	v.endpointInput.Blur()
	v.apiKeyInput.Blur()
	switch v.focusIndex {
	case 0:
		v.nameInput.Focus()
	case 1:
		v.endpointInput.Focus()
	case 2:
		v.apiKeyInput.Focus()
	}
}

func fetchModelsCmd(providerName, baseURL, apiKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		models, err := model.FetchProviderModels(ctx, baseURL, apiKey)
		return modelsFetchedMsg{
			providerName: providerName,
			baseURL:      baseURL,
			apiKey:       apiKey,
			models:       models,
			err:          err,
		}
	}
}

func saveProviderCmd(providerName, baseURL, apiKey, defaultModel string) tea.Cmd {
	return func() tea.Msg {
		homeDir := strings.TrimSpace(os.Getenv("PROTON_HOME"))
		if homeDir == "" {
			if h, err := os.UserHomeDir(); err == nil {
				homeDir = h
			}
		}

		prov := config.ProviderConfig{
			Name:    providerName,
			Type:    "openai",
			BaseURL: baseURL,
			APIKey:  apiKey,
		}

		err := config.SaveUserProviderConfig(homeDir, prov, defaultModel)
		return providerSavedMsg{
			providerName: providerName,
			baseURL:      baseURL,
			apiKey:       apiKey,
			modelID:      defaultModel,
			err:          err,
		}
	}
}

func formatContextTokens(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
