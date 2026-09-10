package runtime

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
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

type providerSelectDelegate struct{}

func (providerSelectDelegate) Height() int                         { return 1 }
func (providerSelectDelegate) Spacing() int                        { return 0 }
func (providerSelectDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (providerSelectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(providerSelectItem)
	if !ok {
		return
	}
	prefix, style := "  ", bodyStyle
	if index == m.Index() {
		prefix, style = "> ", brandStyle
	}
	label := entry.displayName
	if entry.isFree {
		label += "  FREE"
	}
	marker := ""
	if entry.isActive {
		marker = "(current)"
	} else if entry.isConfigured {
		marker = "saved"
	}
	width := maxInt(1, m.Width()-2)
	if marker != "" {
		markerWidth := len([]rune(marker))
		label = truncateWithEllipsis(label, maxInt(1, width-markerWidth-2))
		gap := maxInt(2, width-len([]rune(label))-markerWidth)
		label += strings.Repeat(" ", gap) + mutedStyle.Render(marker)
	} else {
		label = truncateWithEllipsis(label, width)
	}
	_, _ = fmt.Fprint(w, prefix+style.Render(label))
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
	items := make([]list.Item, 0, len(v.items))
	for _, item := range v.items {
		items = append(items, item)
	}
	v.picker = list.New(items, providerSelectDelegate{}, defaultBubbleWidth-8, defaultBubbleHeight-8)
	v.picker.DisableQuitKeybindings()
	v.picker.SetStatusBarItemName("provider", "providers")
	v.picker.SetShowTitle(false)
	v.picker.SetShowStatusBar(false)
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(false)
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
		m.configMutationGate.invalidate()
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

func (*providerSelectPaneView) PresentationMode() panePresentationMode {
	return paneBelowComposer
}

func (v *providerSelectPaneView) selectedItem() (providerSelectItem, bool) {
	if v == nil || !v.pickerReady {
		return providerSelectItem{}, false
	}
	item, ok := v.picker.SelectedItem().(providerSelectItem)
	return item, ok
}

func (v *providerSelectPaneView) Render(ctx paneRenderContext) string {
	v.initPicker()
	v.picker.SetSize(maxInt(12, ctx.width-8), maxInt(4, minInt(maxProviderListRows, ctx.height-6)))
	if v.deleteConfirm {
		item, ok := v.selectedItem()
		if ok && item.isConfigured {
			rows := []string{item.displayName, mutedStyle.Render(item.baseURL)}
			if item.isActive {
				rows = append(rows, warningStyle.Render("This is the active provider."))
			}
			help := paneKeyboardHelp(ctx.width-4, "enter", "Remove", "esc", "Cancel")
			return renderProviderModal(ctx, warningColor, paneSection("Remove Provider?", rows, help, "", ctx.width))
		}
	}
	help := ""
	if layoutModeForHeight(ctx.height) != layoutTiny {
		help = paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "enter", "Select", "e", "Edit", "m", "Models", "d", "Remove", "esc", "Go Back")
	}
	status := ""
	if item, ok := v.selectedItem(); ok {
		status = item.displayName
		if item.isActive {
			status += " · current"
		} else if item.isConfigured {
			status += " · saved"
		}
	}
	items := v.picker.VisibleItems()
	start, end := paneWindow(len(items), v.picker.Index(), maxProviderListRows, layoutModeForHeight(ctx.height))
	listRows := make([]string, 0, end-start+1)
	if v.picker.SettingFilter() || v.picker.IsFiltered() {
		listRows = append(listRows, mutedStyle.Render("Search: ")+userStyle.Render(v.picker.FilterValue()))
	}
	for index := start; index < end; index++ {
		item, ok := items[index].(providerSelectItem)
		if !ok {
			continue
		}
		prefix, style := "  ", bodyStyle
		if index == v.picker.Index() {
			prefix, style = "> ", brandStyle
		}
		label := item.displayName
		if item.isFree {
			label += "  FREE"
		}
		marker := ""
		if item.isActive {
			marker = "(current)"
		} else if item.isConfigured {
			marker = "saved"
		}
		lineWidth := maxInt(1, providerModalContentWidth(ctx)-2)
		if marker != "" {
			markerWidth := len([]rune(marker))
			label = truncateWithEllipsis(label, maxInt(1, lineWidth-markerWidth-2))
			gap := maxInt(2, lineWidth-len([]rune(label))-markerWidth)
			listRows = append(listRows, prefix+style.Render(label)+strings.Repeat(" ", gap)+mutedStyle.Render(marker))
		} else {
			listRows = append(listRows, prefix+style.Render(truncateWithEllipsis(label, lineWidth)))
		}
	}
	return renderProviderModal(ctx, accentAssistant, paneSection("Providers", listRows, help, status, providerModalContentWidth(ctx)+4))
}
