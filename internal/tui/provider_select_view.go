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

const providerSelectViewID = "provider_select"
const maxProviderListRows = 5

type providerActiveSelectedMsg struct {
	providerName string
	err          error
}

type providerDeletedMsg struct {
	providerName string
	err          error
}

type providerItemKind int

const (
	providerItemConfigured providerItemKind = iota
	providerItemPreset
	providerItemCustom
)

type providerSelectItem struct {
	kind         providerItemKind
	name         string
	displayName  string
	baseURL      string
	apiKey       string
	description  string
	presetID     string
	isConfigured bool
	isActive     bool
	isFree       bool
}

type providerSelectPaneView struct {
	index  int
	offset int
	items  []providerSelectItem
}

func newProviderSelectPaneView(m *bubbleModel) *providerSelectPaneView {
	items := make([]providerSelectItem, 0)
	configuredMap := make(map[string]bool)

	// 1. Configured providers
	if m != nil && len(m.providers) > 0 {
		names := make([]string, 0, len(m.providers))
		for name := range m.providers {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			cfg := m.providers[name]
			isActive := strings.EqualFold(name, m.activeProvider)
			isFree := strings.EqualFold(name, model.DefaultOpenCodeName) || strings.Contains(strings.ToLower(cfg.BaseURL), "opencode.ai")
			dispName := cfg.Name
			if dispName == "" {
				dispName = name
			}
			if p := model.LookupPreset(name); p != nil {
				dispName = p.Name
			}

			items = append(items, providerSelectItem{
				kind:         providerItemConfigured,
				name:         name,
				displayName:  dispName,
				baseURL:      cfg.BaseURL,
				apiKey:       cfg.APIKey,
				isConfigured: true,
				isActive:     isActive,
				isFree:       isFree,
				presetID:     name,
			})
			configuredMap[strings.ToLower(name)] = true
		}
	}

	// 2. Available supported presets not yet configured
	for _, preset := range model.SupportedPresets {
		if !configuredMap[strings.ToLower(preset.ID)] {
			items = append(items, providerSelectItem{
				kind:         providerItemPreset,
				name:         preset.ID,
				displayName:  preset.Name,
				baseURL:      preset.BaseURL,
				description:  preset.Description,
				presetID:     preset.ID,
				isConfigured: false,
				isActive:     false,
				isFree:       !preset.RequiresKey,
			})
		}
	}

	// 3. Custom Gateway entry
	items = append(items, providerSelectItem{
		kind:         providerItemCustom,
		name:         "custom",
		displayName:  "+ Custom Gateway / Proxy",
		description:  "Any OpenAI-compatible base URL",
		isConfigured: false,
		isActive:     false,
	})

	selectedIndex := 0
	for i, it := range items {
		if it.isActive {
			selectedIndex = i
			break
		}
	}

	return &providerSelectPaneView{
		index:  selectedIndex,
		offset: 0,
		items:  items,
	}
}

func (*providerSelectPaneView) ID() string             { return providerSelectViewID }
func (*providerSelectPaneView) ReplacesComposer() bool { return true }

func (v *providerSelectPaneView) Render(m *bubbleModel) string {
	maxWidth := maxInt(1, m.width-4)
	if len(v.items) == 0 {
		rows := []string{
			brandStyle.Render("✓ Model Providers"),
			"",
			mutedStyle.Render("No providers or presets available."),
			"",
			mutedStyle.Render("a add provider · esc close"),
		}
		return modalStyle.
			BorderForeground(accentAssistant).
			MaxWidth(maxWidth).
			Render(strings.Join(rows, "\n"))
	}

	if v.index >= len(v.items) {
		v.index = len(v.items) - 1
	}
	if v.index < 0 {
		v.index = 0
	}

	// Dynamic scroll windowing
	if v.index < v.offset {
		v.offset = v.index
	}
	if v.index >= v.offset+maxProviderListRows {
		v.offset = v.index - maxProviderListRows + 1
	}
	if v.offset > len(v.items)-maxProviderListRows {
		v.offset = len(v.items) - maxProviderListRows
	}
	if v.offset < 0 {
		v.offset = 0
	}

	visibleEnd := v.offset + maxProviderListRows
	if visibleEnd > len(v.items) {
		visibleEnd = len(v.items)
	}
	visible := v.items[v.offset:visibleEnd]

	numConfigured := 0
	numAvailable := 0
	for _, it := range v.items {
		if it.isConfigured {
			numConfigured++
		} else {
			numAvailable++
		}
	}

	activeName := "none"
	if m != nil && m.activeProvider != "" {
		activeName = m.activeProvider
	}

	title := fmt.Sprintf("✓ Model Providers (%d configured · %d available · active: %s) [%d/%d]",
		numConfigured, numAvailable, activeName, v.index+1, len(v.items))

	rows := make([]string, 0, len(visible)*2+6)
	rows = append(rows, brandStyle.Render(title), "")

	if v.offset > 0 {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▲ %d more above", v.offset)))
	}

	for i, item := range visible {
		idx := v.offset + i
		radio := "( )"
		if item.isActive {
			radio = "(●)"
		}

		statusBadge := ""
		if item.isActive {
			statusBadge = " " + successStyle.Render("[active]")
		} else if item.isConfigured {
			statusBadge = " " + mutedStyle.Render("[configured]")
		} else if item.kind == providerItemPreset {
			statusBadge = " " + brandStyle.Render("[available preset]")
		} else if item.kind == providerItemCustom {
			statusBadge = " " + mutedStyle.Render("[custom]")
		}

		freeBadge := ""
		if item.isFree {
			freeBadge = " " + successStyle.Render("[free models]")
		}

		var line1 string
		var line2 string
		if item.kind == providerItemCustom {
			line1 = fmt.Sprintf("%s %d. %s%s", radio, idx+1, item.displayName, statusBadge)
			line2 = fmt.Sprintf("      %s", mutedStyle.Render(item.description))
		} else if item.isConfigured {
			keyStr := "Key: [masked]"
			if strings.TrimSpace(item.apiKey) == "" {
				keyStr = "Key: [none required]"
			}
			line1 = fmt.Sprintf("%s %d. %s%s%s", radio, idx+1, item.displayName, statusBadge, freeBadge)
			line2 = fmt.Sprintf("      %s · %s", mutedStyle.Render(item.baseURL), mutedStyle.Render(keyStr))
		} else {
			line1 = fmt.Sprintf("%s %d. %s%s%s", radio, idx+1, item.displayName, statusBadge, freeBadge)
			line2 = fmt.Sprintf("      %s · %s", mutedStyle.Render(item.baseURL), mutedStyle.Render(item.description))
		}

		if idx == v.index {
			rows = append(rows, brandStyle.Render("  ❯ ")+brandStyle.Render(line1))
			rows = append(rows, line2)
		} else {
			rows = append(rows, "    "+mutedStyle.Render(line1))
			rows = append(rows, line2)
		}
	}

	if visibleEnd < len(v.items) {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▼ %d more below", len(v.items)-visibleEnd)))
	}

	rows = append(rows, "", mutedStyle.Render("↑/↓ move · 1-9 jump · enter switch/setup · e edit/create details · m models · d remove · esc close"))
	return modalStyle.
		BorderForeground(accentAssistant).
		MaxWidth(maxWidth).
		Render(strings.Join(rows, "\n"))
}

func (v *providerSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "esc", "ctrl+c", "q":
		m.bottom.remove(providerSelectViewID)
		return true, nil

	case "a", "c":
		m.bottom.remove(providerSelectViewID)
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneView())
		}
		return true, nil

	case "m":
		if len(v.items) > 0 && v.index >= 0 && v.index < len(v.items) {
			item := v.items[v.index]
			if item.isConfigured {
				m.bottom.remove(providerSelectViewID)
				if !m.bottom.has(modelSelectViewID) {
					mv := newModelSelectPaneView(m)
					for i, name := range mv.providerNames {
						if strings.EqualFold(name, item.name) {
							mv.providerIndex = i
							break
						}
					}
					m.bottom.push(mv)
					if cfg, ok := m.providers[strings.ToLower(item.name)]; ok {
						return true, fetchModelsCmd(item.name, cfg.BaseURL, cfg.APIKey)
					}
				}
				return true, nil
			}
		}
		return true, nil

	case "e":
		if len(v.items) > 0 && v.index >= 0 && v.index < len(v.items) {
			item := v.items[v.index]
			m.bottom.remove(providerSelectViewID)
			if !m.bottom.has(providerViewID) {
				if item.isConfigured {
					if cfg, ok := m.providers[strings.ToLower(item.name)]; ok {
						m.bottom.push(newProviderPaneViewWithConfig(cfg))
					} else {
						m.bottom.push(newProviderPaneViewWithPreset(item.name))
					}
				} else if item.kind == providerItemPreset {
					m.bottom.push(newProviderPaneViewWithPreset(item.presetID))
				} else {
					m.bottom.push(newProviderPaneView())
				}
			}
			return true, nil
		}
		return true, nil

	case "d":
		if len(v.items) > 0 && v.index >= 0 && v.index < len(v.items) {
			item := v.items[v.index]
			if item.isConfigured {
				m.bottom.remove(providerSelectViewID)
				return true, deleteProviderCmd(item.name)
			}
		}
		return true, nil

	case "up", "k":
		if len(v.items) > 0 {
			v.index = (v.index - 1 + len(v.items)) % len(v.items)
		}
		return true, nil

	case "down", "j":
		if len(v.items) > 0 {
			v.index = (v.index + 1) % len(v.items)
		}
		return true, nil

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		num := int(message.Runes[0] - '1')
		if num >= 0 && num < len(v.items) {
			v.index = num
		}
		return true, nil

	case "enter":
		if len(v.items) > 0 && v.index >= 0 && v.index < len(v.items) {
			item := v.items[v.index]
			m.bottom.remove(providerSelectViewID)
			if item.isConfigured {
				return true, saveActiveProviderCmd(item.name)
			}
			if item.kind == providerItemPreset {
				if !m.bottom.has(providerViewID) {
					m.bottom.push(newProviderPaneViewWithPreset(item.presetID))
				}
				return true, nil
			}
			// Custom
			if !m.bottom.has(providerViewID) {
				m.bottom.push(newProviderPaneView())
			}
			return true, nil
		}
		return true, nil

	default:
		return false, nil
	}
}

func saveActiveProviderCmd(providerName string) tea.Cmd {
	return func() tea.Msg {
		homeDir := strings.TrimSpace(os.Getenv("PROTON_HOME"))
		if homeDir == "" {
			if h, err := os.UserHomeDir(); err == nil {
				homeDir = h
			}
		}

		err := config.SaveUserDefaultProvider(homeDir, providerName)
		return providerActiveSelectedMsg{
			providerName: providerName,
			err:          err,
		}
	}
}

func deleteProviderCmd(providerName string) tea.Cmd {
	return func() tea.Msg {
		homeDir := strings.TrimSpace(os.Getenv("PROTON_HOME"))
		if homeDir == "" {
			if h, err := os.UserHomeDir(); err == nil {
				homeDir = h
			}
		}

		err := config.DeleteUserProviderConfig(homeDir, providerName)
		return providerDeletedMsg{
			providerName: providerName,
			err:          err,
		}
	}
}
