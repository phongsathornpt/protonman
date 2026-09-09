package runtime

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
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

func (i providerSelectItem) FilterValue() string {
	return strings.Join([]string{i.name, i.displayName, i.baseURL, i.description}, " ")
}

func (i providerSelectItem) Title() string {
	label := i.displayName
	status := "setup"
	if i.isActive {
		status = "active"
	} else if i.isConfigured {
		status = "saved"
	} else if i.kind == providerItemCustom {
		status = "custom"
	}
	label += " · " + status
	if i.isFree {
		label += " · free"
	}
	if i.isActive {
		label = "✓ " + label
	}
	return label
}

func (i providerSelectItem) Description() string {
	if strings.TrimSpace(i.baseURL) != "" {
		return i.baseURL
	}
	return i.description
}

type providerSelectPaneView struct {
	index         int
	offset        int
	picker        list.Model
	pickerReady   bool
	items         []providerSelectItem
	deleteConfirm bool
}

func (v *providerSelectPaneView) initPicker() {
	if v == nil || v.pickerReady {
		return
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	items := make([]list.Item, 0, len(v.items))
	for _, item := range v.items {
		items = append(items, item)
	}
	v.picker = list.New(items, delegate, defaultBubbleWidth-8, defaultBubbleHeight-8)
	v.picker.DisableQuitKeybindings()
	v.picker.SetStatusBarItemName("provider", "providers")
	v.picker.FilterInput.Prompt = "Search: "
	v.picker.InfiniteScrolling = false
	v.picker.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
			key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
			key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "models")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove")),
		}
	}
	v.pickerReady = true
}

func (v *providerSelectPaneView) syncPickerProjection() {
	if v == nil || !v.pickerReady {
		return
	}
	v.index = v.picker.Index()
	v.offset = v.picker.Paginator.Page * v.picker.Paginator.PerPage
}

func newProviderSelectPaneView(m *bubbleModel) *providerSelectPaneView {
	items := make([]providerSelectItem, 0)
	configuredMap := make(map[string]bool)
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
			items = append(items, providerSelectItem{kind: providerItemConfigured, name: name, displayName: dispName, baseURL: cfg.BaseURL, apiKey: cfg.APIKey, isConfigured: true, isActive: isActive, isFree: isFree, presetID: name})
			configuredMap[strings.ToLower(name)] = true
		}
	}
	for _, preset := range model.SupportedPresets {
		if !configuredMap[strings.ToLower(preset.ID)] {
			isActive := m != nil && strings.EqualFold(preset.ID, m.activeProvider)
			items = append(items, providerSelectItem{kind: providerItemPreset, name: preset.ID, displayName: preset.Name, baseURL: preset.BaseURL, description: preset.Description, presetID: preset.ID, isConfigured: false, isActive: isActive, isFree: !preset.RequiresKey})
		}
	}
	items = append(items, providerSelectItem{kind: providerItemCustom, name: "custom", displayName: "+ Custom Gateway / Proxy", description: "Any OpenAI-compatible or Anthropic Messages base URL", isConfigured: false, isActive: false})
	selectedIndex := 0
	for i, it := range items {
		if it.isActive {
			selectedIndex = i
			break
		}
	}
	view := &providerSelectPaneView{items: items}
	view.initPicker()
	view.picker.Select(selectedIndex)
	view.syncPickerProjection()
	return view
}

func (*providerSelectPaneView) ID() string {
	return providerSelectViewID
}

func (*providerSelectPaneView) ReplacesComposer() bool {
	return true
}

func (v *providerSelectPaneView) selectedItem() (providerSelectItem, bool) {
	if v == nil || !v.pickerReady {
		return providerSelectItem{}, false
	}
	item, ok := v.picker.SelectedItem().(providerSelectItem)
	return item, ok
}

func (v *providerSelectPaneView) Render(m *bubbleModel) string {
	v.initPicker()
	if m == nil {
		return ""
	}
	mode := layoutModeForHeight(m.height)
	v.picker.SetSize(maxInt(12, m.width-8), maxInt(5, minInt(14, m.height-4)))
	v.picker.Title = "Providers"
	v.picker.SetShowStatusBar(mode != layoutTiny)
	v.picker.SetShowPagination(mode != layoutTiny)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	if v.deleteConfirm {
		item, ok := v.selectedItem()
		if ok && item.isConfigured {
			rows := []string{warningStyle.Render("Remove Provider?"), "", item.displayName, mutedStyle.Render(item.baseURL)}
			if item.isActive {
				rows = append(rows, warningStyle.Render("This is the active provider."))
			}
			rows = append(rows, "", mutedStyle.Render("enter remove permanently · esc cancel"))
			return renderProviderModal(m, warningColor, rows)
		}
	}
	return renderProviderModal(m, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *providerSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	v.initPicker()
	if v.index != v.picker.Index() {
		v.picker.Select(v.index)
		v.syncPickerProjection()
	}
	if v.deleteConfirm {
		item, ok := v.selectedItem()
		if !ok || !item.isConfigured {
			v.deleteConfirm = false
		}
	}
	if v.picker.SettingFilter() {
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return true, cmd
	}
	if v.deleteConfirm {
		switch message.String() {
		case "enter":
			item, ok := v.selectedItem()
			v.deleteConfirm = false
			if !ok {
				return true, nil
			}
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
		if item, ok := v.selectedItem(); ok {
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
		if item, ok := v.selectedItem(); ok {
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
		if item, ok := v.selectedItem(); ok {
			if item.isConfigured {
				v.deleteConfirm = true
			}
		}
		return true, nil
	case "/":
		v.picker.SetFilterState(list.Filtering)
		v.syncPickerProjection()
		return true, nil
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return true, cmd
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return true, nil
	case "enter":
		if item, ok := v.selectedItem(); ok {
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
		return providerActiveSelectedMsg{providerName: providerName, err: err}
	}
}

func deleteProviderCmd(providerName string) tea.Cmd {
	return func() tea.Msg {
		err := (app.Providers{}).Delete(providerName)
		return providerDeletedMsg{providerName: providerName, err: err}
	}
}
