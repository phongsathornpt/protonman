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
	modelID      string
	err          error
}

type providerPaneView struct {
	state         providerPaneState
	focusIndex    int
	nameInput     textinput.Model
	endpointInput textinput.Model
	apiKeyInput   textinput.Model
	models        []model.RemoteModel
	selectedIndex int
	errorMessage  string
}

func newProviderPaneView() *providerPaneView {
	nameIn := textinput.New()
	nameIn.Prompt = glyphPrompt
	nameIn.Placeholder = model.DefaultProtonmanName
	nameIn.SetValue(model.DefaultProtonmanName)
	nameIn.CharLimit = 64

	endpointIn := textinput.New()
	endpointIn.Prompt = glyphPrompt
	endpointIn.Placeholder = model.DefaultProtonmanEndpoint
	endpointIn.SetValue(model.DefaultProtonmanEndpoint)
	endpointIn.CharLimit = 256

	keyIn := textinput.New()
	keyIn.Prompt = glyphPrompt
	keyIn.Placeholder = "plk_live_..."
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.EchoCharacter = '•'
	keyIn.CharLimit = 256
	keyIn.Focus()

	return &providerPaneView{
		state:         providerStateInput,
		focusIndex:    2, // Start with focus on API key
		nameInput:     nameIn,
		endpointInput: endpointIn,
		apiKeyInput:   keyIn,
	}
}

func (*providerPaneView) ID() string             { return providerViewID }
func (*providerPaneView) ReplacesComposer() bool { return true }

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
		rows := []string{
			brandStyle.Render(fmt.Sprintf("✓ Select Active Model (%d discovered) [Step 2/2]", len(v.models))),
			"",
		}
		for idx, md := range v.models {
			prefix := "    "
			if idx == v.selectedIndex {
				prefix = brandStyle.Render("  ❯ ")
			}
			line := fmt.Sprintf("%d. %s", idx+1, md.ID)
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
		rows = append(rows, "", mutedStyle.Render("↑/↓ or j/k move · 1-9 select · enter confirm & save · esc back"))
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
		rows := []string{
			brandStyle.Render("◆ Add Model Provider [Step 1/2: Credentials]"),
			"",
			mutedStyle.Render("Provider Name:"),
			v.nameInput.View(),
			"",
			mutedStyle.Render("Endpoint (Base URL):"),
			v.endpointInput.View(),
			"",
			mutedStyle.Render("API Key (shown masked):"),
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
		switch message.String() {
		case "esc":
			v.state = providerStateInput
			return true, nil
		case "up", "k":
			if len(v.models) > 0 {
				v.selectedIndex = (v.selectedIndex - 1 + len(v.models)) % len(v.models)
			}
			return true, nil
		case "down", "j":
			if len(v.models) > 0 {
				v.selectedIndex = (v.selectedIndex + 1) % len(v.models)
			}
			return true, nil
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			num := int(message.Runes[0] - '1')
			if num >= 0 && num < len(v.models) {
				v.selectedIndex = num
			}
			return true, nil
		case "enter":
			if len(v.models) > 0 && v.selectedIndex >= 0 && v.selectedIndex < len(v.models) {
				selected := v.models[v.selectedIndex]
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
			if key == "" {
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
