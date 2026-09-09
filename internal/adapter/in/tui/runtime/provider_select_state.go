package runtime

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

const providerSelectViewID = "provider_select"

const maxProviderListRows = 5

type providerActiveSelectedMsg struct {
	operationID     asyncOperationID
	providerName    string
	reconciledModel string
	err             error
}

type providerDeletedMsg struct {
	operationID  asyncOperationID
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

func newProviderSelectPaneView(m *bubbleModel) *providerSelectPaneView {
	if m != nil {
		m.activeProviderSelect = 0
		m.activeProviderDelete = 0
	}
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
	mode := layoutModeForHeight(m.layout.height)
	v.picker.SetSize(maxInt(12, m.layout.width-8), maxInt(4, minInt(8, m.layout.height-6)))
	v.picker.Title = "Providers"
	v.picker.SetShowStatusBar(false)
	// Keep pagination presentation hidden; Bubbles list still owns navigation and selection state.
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	if v.deleteConfirm {
		item, ok := v.selectedItem()
		if ok && item.isConfigured {
			rows := []string{warningStyle.Render("Remove provider?"), item.displayName, mutedStyle.Render(item.baseURL)}
			if item.isActive {
				rows = append(rows, warningStyle.Render("This is the active provider."))
			}
			rows = append(rows, mutedStyle.Render("enter remove · esc cancel"))
			return renderProviderModal(m, warningColor, rows)
		}
	}
	return renderProviderModal(m, accentAssistant, strings.Split(v.picker.View(), "\n"))
}
