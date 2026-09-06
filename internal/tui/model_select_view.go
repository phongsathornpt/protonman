package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
)

const modelSelectViewID = "model_select"
const maxModelSelectRows = 6

type modelSelectedMsg struct {
	providerName string
	modelID      string
	err          error
}

type modelSelectPaneView struct {
	index         int
	offset        int
	models        []model.RemoteModel
	providerNames []string
	providerIndex int
}

func newModelSelectPaneView(m *bubbleModel) *modelSelectPaneView {
	providers := make([]string, 0)
	if m != nil && len(m.providers) > 0 {
		for name := range m.providers {
			providers = append(providers, name)
		}
		sort.Strings(providers)
	} else if m != nil && m.activeProvider != "" {
		providers = append(providers, m.activeProvider)
	} else {
		providers = append(providers, model.DefaultProtonmanName)
	}

	providerIdx := 0
	if m != nil && m.activeProvider != "" {
		for i, name := range providers {
			if strings.EqualFold(name, m.activeProvider) {
				providerIdx = i
				break
			}
		}
	}

	// Resolve models catalog
	var modelsList []model.RemoteModel
	if m != nil && len(m.modelsCatalog) > 0 {
		modelsList = append([]model.RemoteModel{}, m.modelsCatalog...)
	} else {
		modelsList = append([]model.RemoteModel{}, model.DefaultProtonmanModels...)
	}

	// Focus currently active model if present
	selectedIndex := 0
	if m != nil && m.activeModel != "" {
		for i, md := range modelsList {
			if strings.EqualFold(md.ID, m.activeModel) {
				selectedIndex = i
				break
			}
		}
	}

	return &modelSelectPaneView{
		index:         selectedIndex,
		offset:        0,
		models:        modelsList,
		providerNames: providers,
		providerIndex: providerIdx,
	}
}

func (*modelSelectPaneView) ID() string             { return modelSelectViewID }
func (*modelSelectPaneView) ReplacesComposer() bool { return true }

func (v *modelSelectPaneView) Render(m *bubbleModel) string {
	maxWidth := maxInt(1, m.width-4)
	if len(v.models) == 0 {
		rows := []string{
			brandStyle.Render("✓ Select Model"),
			"",
			mutedStyle.Render("No models available for the selected provider."),
			"",
			mutedStyle.Render("a add provider credentials · esc close"),
		}
		return modalStyle.
			BorderForeground(accentAssistant).
			MaxWidth(maxWidth).
			Render(strings.Join(rows, "\n"))
	}

	activeProv := "default"
	if v.providerIndex >= 0 && v.providerIndex < len(v.providerNames) {
		activeProv = v.providerNames[v.providerIndex]
	}

	if v.index >= len(v.models) {
		v.index = len(v.models) - 1
	}
	if v.index < 0 {
		v.index = 0
	}

	visibleRows := pickerVisibleRows(m.height, maxModelSelectRows)

	// Dynamic scroll windowing
	if v.index < v.offset {
		v.offset = v.index
	}
	if v.index >= v.offset+visibleRows {
		v.offset = v.index - visibleRows + 1
	}
	if v.offset > len(v.models)-visibleRows {
		v.offset = len(v.models) - visibleRows
	}
	if v.offset < 0 {
		v.offset = 0
	}

	visibleEnd := v.offset + visibleRows
	if visibleEnd > len(v.models) {
		visibleEnd = len(v.models)
	}
	visible := v.models[v.offset:visibleEnd]

	title := fmt.Sprintf("Select Model · %s · %d/%d", activeProv, v.index+1, len(v.models))
	if len(v.providerNames) > 1 {
		title += " · tab provider"
	}

	rows := make([]string, 0, len(visible)*2+6)
	rows = append(rows, brandStyle.Render(title), "")

	if v.offset > 0 {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ↑ %d more", v.offset)))
	}

	contentWidth := maxInt(8, maxWidth-6)
	showDetails := layoutModeForHeight(m.height) == layoutNormal
	for i, md := range visible {
		idx := v.offset + i
		isCurrent := m != nil && strings.EqualFold(md.ID, m.activeModel)
		focus := "  "
		if idx == v.index {
			focus = "❯ "
		}
		active := " "
		if isCurrent {
			active = "✓"
		}
		label := strings.TrimSpace(md.Name)
		if label == "" {
			label = md.ID
		}
		line := fmt.Sprintf("%s%s %s", focus, active, label)
		if model.IsFreeModel(md.ID) {
			line += " · FREE"
		}
		line = truncateWithEllipsis(line, contentWidth)
		if idx == v.index {
			rows = append(rows, brandStyle.Render(line))
		} else if isCurrent {
			rows = append(rows, successStyle.Render(line))
		} else {
			rows = append(rows, mutedStyle.Render(line))
		}

		if showDetails {
			details := make([]string, 0, 3)
			if label != md.ID && strings.TrimSpace(md.ID) != "" {
				details = append(details, md.ID)
			}
			if md.ContextWindow > 0 {
				details = append(details, formatContextTokens(md.ContextWindow)+" context")
			}
			if len(md.Features) > 0 {
				details = append(details, strings.Join(md.Features, " · "))
			}
			if len(details) > 0 {
				rows = append(rows, mutedStyle.Render("    "+truncateWithEllipsis(strings.Join(details, " · "), maxInt(4, contentWidth-4))))
			}
		}
	}

	if visibleEnd < len(v.models) {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ↓ %d more", len(v.models)-visibleEnd)))
	}

	rows = append(rows, "", mutedStyle.Render("↑/↓ move · pgup/pgdn page · home/end · enter select · p providers · esc close"))
	return modalStyle.
		BorderForeground(accentAssistant).
		MaxWidth(maxWidth).
		Render(strings.Join(rows, "\n"))
}

func (v *modelSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "esc", "ctrl+c", "ctrl+p", "alt+m", "q":
		m.bottom.remove(modelSelectViewID)
		return true, nil

	case "p":
		// Switch to provider select view
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerSelectViewID) {
			m.bottom.push(newProviderSelectPaneView(m))
		}
		return true, nil

	case "a":
		// Switch to add provider view
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneView())
		}
		return true, nil

	case "tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex + 1) % len(v.providerNames)
			currentProv := v.providerNames[v.providerIndex]
			if cfg, ok := m.providers[currentProv]; ok {
				isOpenCode := strings.Contains(strings.ToLower(cfg.BaseURL), "opencode.ai") || strings.EqualFold(currentProv, model.DefaultOpenCodeName)
				if cfg.APIKey != "" || isOpenCode {
					// Fetch models for newly focused provider
					return true, fetchModelsCmd(currentProv, cfg.BaseURL, cfg.APIKey)
				}
			}
		}
		return true, nil

	case "up", "k":
		if v.index > 0 {
			v.index--
		}
		return true, nil

	case "down", "j":
		if v.index < len(v.models)-1 {
			v.index++
		}
		return true, nil

	case "pgup":
		v.index -= pickerVisibleRows(m.height, maxModelSelectRows)
		if v.index < 0 {
			v.index = 0
		}
		return true, nil

	case "pgdown":
		v.index += pickerVisibleRows(m.height, maxModelSelectRows)
		if v.index >= len(v.models) {
			v.index = len(v.models) - 1
		}
		return true, nil

	case "home", "g":
		v.index = 0
		return true, nil

	case "end", "G":
		if len(v.models) > 0 {
			v.index = len(v.models) - 1
		}
		return true, nil

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return true, nil

	case "enter":
		if len(v.models) == 0 {
			m.bottom.remove(modelSelectViewID)
			if !m.bottom.has(providerViewID) {
				m.bottom.push(newProviderPaneView())
			}
			return true, nil
		}
		if v.index >= 0 && v.index < len(v.models) {
			selected := v.models[v.index]
			provName := model.DefaultProtonmanName
			if v.providerIndex >= 0 && v.providerIndex < len(v.providerNames) {
				provName = v.providerNames[v.providerIndex]
			}
			cmd := saveDefaultModelCmd(provName, selected.ID)
			m.bottom.remove(modelSelectViewID)
			return true, cmd
		}
		return true, nil

	default:
		return false, nil
	}
}

func saveDefaultModelCmd(providerName, modelID string) tea.Cmd {
	return func() tea.Msg {
		homeDir := strings.TrimSpace(os.Getenv("PROTON_HOME"))
		if homeDir == "" {
			if h, err := os.UserHomeDir(); err == nil {
				homeDir = h
			}
		}

		err := config.SaveUserDefaultModel(homeDir, providerName, modelID)
		return modelSelectedMsg{
			providerName: providerName,
			modelID:      modelID,
			err:          err,
		}
	}
}
