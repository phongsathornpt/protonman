package runtime

import (
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	providerdomain "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app"
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

func (i providerSelectItem) selectionEntry() providerdomain.SelectionEntry {
	return providerdomain.SelectionEntry{
		Kind: providerdomain.SelectionKind(i.kind), Name: i.name, DisplayName: i.displayName,
		BaseURL: i.baseURL, APIKey: i.apiKey, Description: i.description, PresetID: i.presetID,
		IsConfigured: i.isConfigured, IsActive: i.isActive, IsFree: i.isFree,
	}
}

func (i providerSelectItem) FilterValue() string {
	return providerdomain.FilterValue(i.selectionEntry())
}
func (i providerSelectItem) Title() string { return providerdomain.Title(i.selectionEntry()) }
func (i providerSelectItem) Description() string {
	return providerdomain.Description(i.selectionEntry())
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
	prefix := "  "
	textStyle := bodyStyle
	if index == m.Index() {
		prefix = brandStyle.Render(glyphPrompt)
		textStyle = bodyStyle.Bold(true)
	}
	label := entry.displayName
	if entry.isFree {
		label += "  FREE"
	}
	marker := providerdomain.Marker(entry.selectionEntry())
	width := maxInt(1, m.Width()-2)
	if marker != "" {
		markerWidth := len([]rune(marker))
		label = truncateWithEllipsis(label, maxInt(1, width-markerWidth-2))
		gap := maxInt(2, width-len([]rune(label))-markerWidth)
		_, _ = fmt.Fprint(w, prefix+textStyle.Render(label)+strings.Repeat(" ", gap)+mutedStyle.Render(marker))
	} else {
		label = truncateWithEllipsis(label, width)
		_, _ = fmt.Fprint(w, prefix+textStyle.Render(label))
	}
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
	v.picker = paneutil.NewMinimalList(items, providerSelectDelegate{}, defaultBubbleWidth-8, defaultBubbleHeight-8)
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
		m.configMutationGate.invalidate()
	}
	providers := map[string]config.ProviderConfig(nil)
	activeProvider := ""
	if m != nil {
		providers = m.providers
		activeProvider = m.activeProvider
	}
	entries := providerdomain.BuildSelectionEntries(providers, activeProvider)
	items := make([]providerSelectItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, providerSelectItem{
			kind: providerItemKind(entry.Kind), name: entry.Name, displayName: entry.DisplayName,
			baseURL: entry.BaseURL, apiKey: entry.APIKey, description: entry.Description,
			presetID: entry.PresetID, isConfigured: entry.IsConfigured,
			isActive: entry.IsActive, isFree: entry.IsFree,
		})
	}
	view := &providerSelectPaneView{items: items}
	view.initPicker()
	view.picker.Select(providerdomain.ActiveSelectionIndex(entries))
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
	if len(items) > end-start {
		if status != "" {
			status = fmt.Sprintf("%d-%d of %d · %s", start+1, end, len(items), status)
		} else {
			status = fmt.Sprintf("%d-%d of %d", start+1, end, len(items))
		}
	}
	listRows := make([]string, 0, end-start+1)
	if v.picker.SettingFilter() || v.picker.IsFiltered() {
		listRows = append(listRows, mutedStyle.Render("Search: ")+userStyle.Render(v.picker.FilterValue()))
	}
	for index := start; index < end; index++ {
		item, ok := items[index].(providerSelectItem)
		if !ok {
			continue
		}
		prefix := "  "
		textStyle := bodyStyle
		if index == v.picker.Index() {
			prefix = brandStyle.Render(glyphPrompt)
			textStyle = bodyStyle.Bold(true)
		}
		label := item.displayName
		if item.isFree {
			label += "  FREE"
		}
		marker := providerdomain.Marker(item.selectionEntry())
		lineWidth := maxInt(1, providerModalContentWidth(ctx)-2)
		if marker != "" {
			markerWidth := len([]rune(marker))
			label = truncateWithEllipsis(label, maxInt(1, lineWidth-markerWidth-2))
			gap := maxInt(2, lineWidth-len([]rune(label))-markerWidth)
			listRows = append(listRows, prefix+textStyle.Render(label)+strings.Repeat(" ", gap)+mutedStyle.Render(marker))
		} else {
			listRows = append(listRows, prefix+textStyle.Render(truncateWithEllipsis(label, lineWidth)))
		}
	}
	return renderProviderModal(ctx, accentAssistant, paneSection("Providers", listRows, help, status, providerModalContentWidth(ctx)+4))
}

var providerSelectKeys = struct {
	Add, Models, Edit, Delete, Filter key.Binding
}{
	Add:    key.NewBinding(key.WithKeys("a", "c")),
	Models: key.NewBinding(key.WithKeys("m")),
	Edit:   key.NewBinding(key.WithKeys("e")),
	Delete: key.NewBinding(key.WithKeys("d")),
	Filter: key.NewBinding(key.WithKeys("/")),
}

func (v *providerSelectPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.initPicker()
	if v.deleteConfirm {
		item, ok := v.selectedItem()
		if !ok || !item.isConfigured {
			v.deleteConfirm = false
		}
	}
	if v.picker.SettingFilter() {
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	}
	if v.deleteConfirm {
		switch {
		case key.Matches(message, paneutil.Keys.Confirm):
			item, ok := v.selectedItem()
			v.deleteConfirm = false
			if !ok {
				return paneKeyResult{handled: true}
			}
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderDelete, providerItem: item}}
		case key.Matches(message, paneutil.Keys.Escape):
			v.deleteConfirm = false
			return paneKeyResult{handled: true}
		default:
			return paneKeyResult{handled: true}
		}
	}
	switch {
	case key.Matches(message, paneutil.Keys.Close):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: providerSelectViewID}}
	case key.Matches(message, providerSelectKeys.Add):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionOpenProviderEditor}}
	case key.Matches(message, providerSelectKeys.Models):
		if item, ok := v.selectedItem(); ok && item.isConfigured {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderModels, providerItem: item}}
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, providerSelectKeys.Edit):
		if item, ok := v.selectedItem(); ok {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderEdit, providerItem: item}}
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, providerSelectKeys.Delete):
		if item, ok := v.selectedItem(); ok && item.isConfigured {
			v.deleteConfirm = true
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, providerSelectKeys.Filter):
		v.picker.SetFilterState(list.Filtering)
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Nav):
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	case message.Text >= "1" && message.Text <= "9":
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Confirm):
		item, ok := v.selectedItem()
		if !ok {
			return paneKeyResult{handled: true}
		}
		if item.isConfigured {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderActivate, providerItem: item}}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderEdit, providerItem: item}}
	default:
		return paneKeyResult{}
	}
}

func saveActiveProviderCmd(providers app.Providers, operationID asyncOperationID, gate *asyncOperationGate, providerName, reconciledModel string) tea.Cmd {
	return func() tea.Msg {
		if !gate.current(operationID) {
			return providerActiveSelectedMsg{operationID: operationID, providerName: providerName, reconciledModel: reconciledModel, err: errStaleConfigMutation}
		}
		err := providers.Activate(providerName, reconciledModel)
		return providerActiveSelectedMsg{operationID: operationID, providerName: providerName, reconciledModel: reconciledModel, err: err}
	}
}

func deleteProviderCmd(providers app.Providers, operationID asyncOperationID, gate *asyncOperationGate, providerName string) tea.Cmd {
	return func() tea.Msg {
		if !gate.current(operationID) {
			return providerDeletedMsg{operationID: operationID, providerName: providerName, err: errStaleConfigMutation}
		}
		err := providers.Delete(providerName)
		return providerDeletedMsg{operationID: operationID, providerName: providerName, err: err}
	}
}
