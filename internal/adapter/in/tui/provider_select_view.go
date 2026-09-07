package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/adapter/out/model"
	"github.com/projectTHORN/proton/internal/app"
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
	index         int
	offset        int
	items         []providerSelectItem
	deleteConfirm bool
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
			isFree := model.IsProvider(model.DefaultOpenCodeName, name, cfg.BaseURL)
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
		description:  "Any OpenAI-compatible or Anthropic Messages base URL",
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
	compact := m.height <= 20
	visibleRows := pickerVisibleRows(m.height, maxProviderListRows)
	if len(v.items) == 0 {
		rows := []string{
			brandStyle.Render("✓ Model Providers"),
			"",
			mutedStyle.Render("No providers or presets available."),
			"",
			mutedStyle.Render("a add provider · esc close"),
		}
		return renderProviderModal(m, accentAssistant, rows)
	}

	index, offset, visibleEnd := normalizedPickerWindow(v.index, v.offset, len(v.items), visibleRows)
	if v.deleteConfirm {
		item := v.items[index]
		if item.isConfigured {
			rows := []string{
				warningStyle.Render("Remove Provider?"),
				"",
				fmt.Sprintf("  %s", item.displayName),
				mutedStyle.Render("  " + item.baseURL),
			}
			if item.isActive {
				rows = append(rows, warningStyle.Render("  This is the active provider."))
				rows = append(rows, mutedStyle.Render("  Proton will select another saved provider."))
			}
			rows = append(rows, "", mutedStyle.Render("enter remove permanently · esc cancel"))
			return renderProviderModal(m, warningColor, rows)
		}
	}

	visible := v.items[offset:visibleEnd]

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

	title := fmt.Sprintf("Providers · active: %s · %d/%d", activeName, v.index+1, len(v.items))
	title = truncateWithEllipsis(title, providerModalContentWidth(m))

	rows := make([]string, 0, len(visible)*2+6)
	rows = append(rows, brandStyle.Render(title), "")

	if offset > 0 {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ↑ %d more", v.offset)))
	}

	contentWidth := maxInt(8, providerModalContentWidth(m)-2)
	showDetails := layoutModeForHeight(m.height) == layoutNormal
	for i, item := range visible {
		idx := offset + i
		focus := "  "
		if idx == index {
			focus = "❯ "
		}
		active := " "
		if item.isActive {
			active = "✓"
		}
		status := "setup"
		switch {
		case item.isActive:
			status = "active"
		case item.isConfigured:
			status = "saved"
		case item.kind == providerItemCustom:
			status = "custom"
		}
		line := fmt.Sprintf("%s%s %s · %s", focus, active, item.displayName, status)
		if item.isFree {
			line += " · free"
		}
		line = truncateWithEllipsis(line, contentWidth)
		if idx == v.index {
			rows = append(rows, brandStyle.Render(line))
		} else if item.isActive {
			rows = append(rows, successStyle.Render(line))
		} else {
			rows = append(rows, mutedStyle.Render(line))
		}

		if showDetails {
			detail := item.baseURL
			if item.kind == providerItemCustom || detail == "" {
				detail = item.description
			}
			if detail != "" {
				rows = append(rows, mutedStyle.Render("    "+truncateWithEllipsis(detail, maxInt(4, contentWidth-4))))
			}
		}
	}

	if visibleEnd < len(v.items) {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ↓ %d more", len(v.items)-visibleEnd)))
	}

	footer := "↑/↓ move · enter activate/setup · e edit · m models · d remove · esc"
	if compact {
		footer = "↑/↓ move · enter activate/setup · e edit · esc"
		if m.width <= 30 {
			footer = "↑/↓ · enter · esc"
		}
	}
	rows = append(rows, "", mutedStyle.Render(footer))
	return renderProviderModal(m, accentAssistant, rows)
}

func (v *providerSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	defer func() {
		visible := pickerVisibleRows(m.height, maxProviderListRows)
		v.index, v.offset, _ = normalizedPickerWindow(v.index, v.offset, len(v.items), visible)
		if v.deleteConfirm && (len(v.items) == 0 || !v.items[v.index].isConfigured) {
			v.deleteConfirm = false
		}
	}()
	if v.deleteConfirm {
		switch message.String() {
		case "enter":
			v.deleteConfirm = false
			item := v.items[v.index]
			m.bottom.remove(providerSelectViewID)
			return true, deleteProviderCmd(item.name)
		case "esc":
			v.deleteConfirm = false
			return true, nil
		case "ctrl+c":
			v.deleteConfirm = false
			m.bottom.remove(providerSelectViewID)
			return true, nil
		default:
			return !m.matchesGlobalShortcut(message), nil
		}
	}

	switch message.String() {
	case "esc", "q":
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
					if _, ok := m.providers[strings.ToLower(item.name)]; ok {
						return true, mv.loadProvider(m, false)
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
						pv := newProviderPaneViewWithConfig(cfg)
						pv.activateOnSave = item.isActive
						m.bottom.push(pv)
					} else {
						pv := newProviderPaneViewWithPreset(item.name)
						pv.activateOnSave = item.isActive
						m.bottom.push(pv)
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
				v.deleteConfirm = true
			}
		}
		return true, nil

	case "up", "k":
		if v.index > 0 {
			v.index--
		}
		return true, nil

	case "down", "j":
		if v.index < len(v.items)-1 {
			v.index++
		}
		return true, nil

	case "pgup":
		v.index -= pickerVisibleRows(m.height, maxProviderListRows)
		if v.index < 0 {
			v.index = 0
		}
		return true, nil

	case "pgdown":
		v.index += pickerVisibleRows(m.height, maxProviderListRows)
		if v.index >= len(v.items) {
			v.index = len(v.items) - 1
		}
		return true, nil

	case "home", "g":
		v.index = 0
		return true, nil

	case "end", "G":
		if len(v.items) > 0 {
			v.index = len(v.items) - 1
		}
		return true, nil

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
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
		err := (app.Providers{}).Select(providerName)
		return providerActiveSelectedMsg{
			providerName: providerName,
			err:          err,
		}
	}
}

func deleteProviderCmd(providerName string) tea.Cmd {
	return func() tea.Msg {
		err := (app.Providers{}).Delete(providerName)
		return providerDeletedMsg{
			providerName: providerName,
			err:          err,
		}
	}
}
