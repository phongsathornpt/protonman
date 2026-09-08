package runtime

import (
	"context"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"net/url"
	"sort"
	"strings"
)

const providerViewID = "add_provider"

type providerPaneState int

const (
	providerStateInput providerPaneState = iota
	providerStateFetching
	providerStateSelectModel
	providerStateConfirmOverwrite
	providerStateSaving
	providerStateSaveError
	providerStateError
)

type providerField int

const (
	providerFieldName providerField = iota
	providerFieldEndpoint
	providerFieldAPIKey
	providerFieldCount
)

const maxProviderSelectRows = 8

type providerPaneView struct {
	state          providerPaneState
	providerType   string
	focusIndex     int
	isEditing      bool
	originalName   string
	presetID       string
	requiresAPIKey bool
	nameInput      textinput.Model
	endpointInput  textinput.Model
	apiKeyInput    textinput.Model
	models         []model.RemoteModel
	selectedIndex  int
	scrollOffset   int
	filterFreeOnly bool
	errorMessage   string
	fieldErrors    [providerFieldCount]string
	fetchRequestID uint64
	fetchCancel    context.CancelFunc
	selectedModel  string
	activateOnSave bool
}

func newProviderPaneView() *providerPaneView {
	return newProviderPaneViewWithPreset("")
}

func newProviderPaneViewWithConfig(cfg config.ProviderConfig) *providerPaneView {
	pv := newProviderPaneViewWithPreset(cfg.Name)
	pv.isEditing = true
	pv.originalName = strings.TrimSpace(cfg.Name)
	if strings.TrimSpace(cfg.Type) != "" {
		pv.providerType = strings.ToLower(strings.TrimSpace(cfg.Type))
	}
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
	name := ""
	endpoint := ""
	keyPlaceholder := "API key (optional)…"
	presetID := ""
	requiresAPIKey := false
	filterFree := false
	providerType := string(model.ProviderProtocolOpenAI)
	if p := model.LookupPreset(preset); p != nil {
		presetID = p.ID
		name = p.ID
		endpoint = p.BaseURL
		requiresAPIKey = p.RequiresKey
		providerType = string(p.Protocol)
		keyPlaceholder = providerKeyPlaceholder(*p)
		if p.ID == model.DefaultOpenCodeName {
			filterFree = true
		}
	} else if preset == "opencode-free" || preset == "free" {
		presetID = model.DefaultOpenCodeName
		name = model.DefaultOpenCodeName
		endpoint = model.DefaultOpenCodeEndpoint
		keyPlaceholder = "API key (optional)…"
		filterFree = true
	} else if preset != "" {
		name = preset
	}
	nameIn := textinput.New()
	nameIn.Prompt = glyphPrompt
	nameIn.Placeholder = "provider name…"
	if name != "" {
		nameIn.SetValue(name)
	}
	nameIn.CharLimit = 64
	endpointIn := textinput.New()
	endpointIn.Prompt = glyphPrompt
	endpointIn.Placeholder = "https://api.example.com/v1"
	if endpoint != "" {
		endpointIn.SetValue(endpoint)
	}
	endpointIn.CharLimit = 256
	keyIn := textinput.New()
	keyIn.Prompt = glyphPrompt
	keyIn.Placeholder = keyPlaceholder
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.EchoCharacter = '•'
	keyIn.CharLimit = 256
	focusIndex := providerFieldName
	if name != "" && endpoint != "" {
		focusIndex = providerFieldAPIKey
	}
	pv := &providerPaneView{state: providerStateInput, focusIndex: int(focusIndex), presetID: presetID, providerType: providerType, requiresAPIKey: requiresAPIKey, nameInput: nameIn, endpointInput: endpointIn, apiKeyInput: keyIn, filterFreeOnly: filterFree, activateOnSave: true}
	pv.syncInputFocus()
	return pv
}

func (*providerPaneView) ID() string {
	return providerViewID
}

func (*providerPaneView) ReplacesComposer() bool {
	return true
}

func providerKeyPlaceholder(p model.SupportedProviderPreset) string {
	if strings.TrimSpace(p.KeyPlaceholder) != "" {
		return p.KeyPlaceholder
	}
	if !p.RequiresKey {
		return "API key (optional)…"
	}
	return "API key…"
}

func (v *providerPaneView) isOpenCode() bool {
	return model.IsProvider(model.DefaultOpenCodeName, v.nameInput.Value(), v.endpointInput.Value())
}

func (v *providerPaneView) applyPreset(preset string) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	if preset == "1" {
		preset = model.DefaultProtonmanName
	} else if preset == "2" {
		preset = model.DefaultOpenCodeName
	} else if preset == "3" {
		preset = model.DefaultOllamaName
	} else if preset == "4" {
		preset = model.DefaultOpenAIName
	} else if preset == "5" {
		preset = model.DefaultAnthropicName
	}
	if p := model.LookupPreset(preset); p != nil {
		previousName := strings.TrimSpace(v.nameInput.Value())
		v.nameInput.SetValue(p.ID)
		v.endpointInput.SetValue(p.BaseURL)
		if !strings.EqualFold(previousName, p.ID) {
			v.apiKeyInput.SetValue("")
		}
		v.apiKeyInput.Placeholder = providerKeyPlaceholder(*p)
		v.presetID = p.ID
		v.requiresAPIKey = p.RequiresKey
		v.providerType = string(p.Protocol)
		v.filterFreeOnly = (p.ID == model.DefaultOpenCodeName)
		v.clearValidation()
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
	}
}

func (v *providerPaneView) protocolLabel() string {
	label := strings.ToLower(strings.TrimSpace(v.providerType))
	if label == "" {
		label = string(model.ProviderProtocolOpenAI)
	}
	if v.presetID == "" {
		return label + " · ctrl+r to switch"
	}
	return label
}

func (v *providerPaneView) toggleProtocol() {
	if v.presetID != "" {
		return
	}
	switch model.ProviderProtocol(strings.ToLower(strings.TrimSpace(v.providerType))) {
	case model.ProviderProtocolAnthropic:
		v.providerType = string(model.ProviderProtocolOpenAI)
	default:
		v.providerType = string(model.ProviderProtocolAnthropic)
	}
}

func (v *providerPaneView) clearValidation() {
	v.fieldErrors = [providerFieldCount]string{}
	v.errorMessage = ""
}

func (v *providerPaneView) clearFieldError(field providerField) {
	if field >= 0 && field < providerFieldCount {
		v.fieldErrors[field] = ""
	}
}

func (v *providerPaneView) validateDraft() bool {
	v.clearValidation()
	firstInvalid := providerFieldCount
	name := strings.TrimSpace(v.nameInput.Value())
	if name == "" {
		v.fieldErrors[providerFieldName] = "required"
		firstInvalid = providerFieldName
	}
	endpoint := strings.TrimSpace(v.endpointInput.Value())
	if endpoint == "" {
		v.fieldErrors[providerFieldEndpoint] = "required"
		if firstInvalid == providerFieldCount {
			firstInvalid = providerFieldEndpoint
		}
	} else if !isValidProviderEndpoint(endpoint) {
		v.fieldErrors[providerFieldEndpoint] = "use an HTTP(S) URL"
		if firstInvalid == providerFieldCount {
			firstInvalid = providerFieldEndpoint
		}
	}
	key := strings.TrimSpace(v.apiKeyInput.Value())
	if v.requiresAPIKey && key == "" {
		v.fieldErrors[providerFieldAPIKey] = "required for this provider"
		if firstInvalid == providerFieldCount {
			firstInvalid = providerFieldAPIKey
		}
	}
	if firstInvalid != providerFieldCount {
		v.focusIndex = int(firstInvalid)
		v.syncInputFocus()
		return false
	}
	v.nameInput.SetValue(name)
	v.endpointInput.SetValue(strings.TrimRight(endpoint, "/"))
	v.apiKeyInput.SetValue(key)
	return true
}

func isValidProviderEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

func (v *providerPaneView) hasNameConflict(m *bubbleModel) bool {
	if m == nil {
		return false
	}
	name := strings.TrimSpace(v.nameInput.Value())
	original := strings.TrimSpace(v.originalName)
	for providerKey, cfg := range m.providers {
		existingName := strings.TrimSpace(cfg.Name)
		if existingName == "" {
			existingName = providerKey
		}
		if strings.EqualFold(existingName, name) && !strings.EqualFold(existingName, original) {
			return true
		}
	}
	return false
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
	if m == nil {
		return ""
	}
	v.resizeInputs(m.width)
	rows, tone := pane.ProviderEditorRows(providerEditorSnapshot(m, v))
	return renderProviderModal(m, paneToneColor(tone), rows)
}

func (v *providerPaneView) resizeInputs(width int) {
	inputWidth := maxInt(8, width-18)
	v.nameInput.Width = inputWidth
	v.endpointInput.Width = inputWidth
	v.apiKeyInput.Width = inputWidth
}

func providerEditorSnapshot(m *bubbleModel, v *providerPaneView) pane.ProviderEditorSnapshot {
	if m == nil || v == nil {
		return pane.ProviderEditorSnapshot{}
	}
	models := v.currentModels()
	items := make([]pane.ProviderEditorModel, 0, len(models))
	providerName := strings.TrimSpace(v.nameInput.Value())
	for _, md := range models {
		resolved := model.ResolveRemoteMetadata(providerName, md)
		label := strings.TrimSpace(md.ID)
		if name := strings.TrimSpace(md.Name); name != "" && !strings.EqualFold(name, label) {
			if label == "" {
				label = name
			} else {
				label = fmt.Sprintf("%s (%s)", name, label)
			}
		}
		items = append(items, pane.ProviderEditorModel{Label: label, Free: model.IsFreeModel(md.ID), Limits: formatModelTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens), Features: strings.Join(resolved.Features, ", "), Reasoning: remoteModelReasoningSummary(providerName, md, false)})
	}
	hasFreeModels := false
	for _, md := range v.models {
		if model.IsFreeModel(md.ID) {
			hasFreeModels = true
			break
		}
	}
	fieldErrors := [3]string{v.fieldErrors[providerFieldName], v.fieldErrors[providerFieldEndpoint], v.fieldErrors[providerFieldAPIKey]}
	return pane.ProviderEditorSnapshot{Width: m.width, Height: m.height, State: providerEditorPaneState(v.state), Name: v.nameInput.Value(), Endpoint: v.endpointInput.Value(), Spinner: m.spinner.View(), UserConfigPath: appdirs.UserConfigDisplay(), ErrorMessage: v.errorMessage, IsEditing: v.isEditing, ActivateOnSave: v.activateOnSave, ProviderType: v.providerType, ProtocolLabel: v.protocolLabel(), RequiresAPIKey: v.requiresAPIKey, NameInput: v.nameInput.View(), EndpointInput: v.endpointInput.View(), APIKeyInput: v.apiKeyInput.View(), FieldErrors: fieldErrors, Models: items, SelectedIndex: v.selectedIndex, ScrollOffset: v.scrollOffset, FilterFreeOnly: v.filterFreeOnly, HasFreeModels: hasFreeModels, TotalModels: len(v.models)}
}

func providerEditorPaneState(state providerPaneState) pane.ProviderEditorState {
	switch state {
	case providerStateFetching:
		return pane.ProviderEditorFetching
	case providerStateSelectModel:
		return pane.ProviderEditorSelectModel
	case providerStateConfirmOverwrite:
		return pane.ProviderEditorConfirmOverwrite
	case providerStateSaving:
		return pane.ProviderEditorSaving
	case providerStateSaveError:
		return pane.ProviderEditorSaveError
	case providerStateError:
		return pane.ProviderEditorError
	default:
		return pane.ProviderEditorInput
	}
}

func renderProviderInput(m *bubbleModel) string {
	if m == nil || m.bottom == nil {
		return ""
	}
	view, ok := m.bottom.find(providerViewID).(*providerPaneView)
	if !ok || view == nil {
		return ""
	}
	view.resizeInputs(m.width)
	snapshot := providerEditorSnapshot(m, view)
	snapshot.State = pane.ProviderEditorInput
	rows, tone := pane.ProviderEditorRows(snapshot)
	return renderProviderModal(m, paneToneColor(tone), rows)
}

func renderProviderModal(m *bubbleModel, border lipgloss.TerminalColor, rows []string) string {
	contentWidth := providerModalContentWidth(m)
	wrappedRows := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == "" || lipgloss.Width(row) <= contentWidth {
			wrappedRows = append(wrappedRows, row)
			continue
		}
		wrappedRows = append(wrappedRows, strings.Split(wrapWords(row, contentWidth), "\n")...)
	}
	return renderModalRows(m, border, wrappedRows)
}

func providerModalContentWidth(m *bubbleModel) int {
	if m == nil {
		return maxInt(1, defaultBubbleWidth-10)
	}
	return maxInt(1, maxInt(1, m.width-4)-6)
}

func (v *providerPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	defer v.normalizeModelSelection()
	switch v.state {
	case providerStateFetching:
		return v.handleFetchingKey(m, message)
	case providerStateSelectModel:
		return v.handleModelSelectKey(m, message)
	case providerStateSaving:
		return true, nil
	case providerStateSaveError:
		return v.handleSaveErrorKey(m, message)
	case providerStateConfirmOverwrite:
		return v.handleOverwriteKey(m, message)
	case providerStateError:
		return v.handleProviderErrorKey(m, message)
	default:
		return v.handleInputKey(m, message)
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
	return &providerSelectPaneView{index: selectedIndex, offset: 0, items: items}
}

func (*providerSelectPaneView) ID() string {
	return providerSelectViewID
}

func (*providerSelectPaneView) ReplacesComposer() bool {
	return true
}

func (v *providerSelectPaneView) Render(m *bubbleModel) string {
	items := make([]pane.ProviderItem, 0, len(v.items))
	for _, item := range v.items {
		items = append(items, pane.ProviderItem{DisplayName: item.displayName, BaseURL: item.baseURL, Description: item.description, Configured: item.isConfigured, Active: item.isActive, Free: item.isFree, Custom: item.kind == providerItemCustom})
	}
	activeName := ""
	if m != nil {
		activeName = m.activeProvider
	}
	rows, warning := pane.ProviderRows(pane.ProviderSnapshot{Width: m.width, Height: m.height, ContentWidth: providerModalContentWidth(m), Index: v.index, Offset: v.offset, ActiveName: activeName, DeleteConfirm: v.deleteConfirm, Items: items})
	border := accentAssistant
	if warning {
		border = warningColor
	}
	return renderProviderModal(m, border, rows)
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

func (v *providerPaneView) normalizeModelSelection() {
	models := v.currentModels()
	v.selectedIndex, v.scrollOffset, _ = normalizedPickerWindow(v.selectedIndex, v.scrollOffset, len(models), maxProviderSelectRows)
}

func (v *providerPaneView) handleFetchingKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	if message.String() == "esc" {
		v.cancelFetch()
		m.bottom.remove(providerViewID)
	}
	return true, nil
}

func (v *providerPaneView) handleModelSelectKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	models := v.currentModels()
	switch message.String() {
	case "esc":
		v.state = providerStateInput
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
		return true, nil
	case "f":
		if v.isOpenCode() {
			v.filterFreeOnly = !v.filterFreeOnly
			v.selectedIndex, v.scrollOffset = 0, 0
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
		num := v.scrollOffset + int(message.Runes[0]-'1')
		if num >= 0 && num < len(models) {
			v.selectedIndex = num
		}
		return true, nil
	case "enter":
		if len(models) == 0 || v.selectedIndex < 0 || v.selectedIndex >= len(models) {
			return true, nil
		}
		selected := models[v.selectedIndex]
		v.selectedModel = selected.ID
		v.state = providerStateSaving
		return true, v.saveSelectedModelCmd(selected.ID)
	default:
		return !m.matchesGlobalShortcut(message), nil
	}
}

func (v *providerPaneView) handleSaveErrorKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "enter":
		v.state = providerStateSaving
		return true, v.saveSelectedModelCmd(v.selectedModel)
	case "esc":
		v.state = providerStateSelectModel
		v.errorMessage = ""
		return true, nil
	default:
		return !m.matchesGlobalShortcut(message), nil
	}
}

func (v *providerPaneView) handleOverwriteKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "enter":
		return true, v.beginFetch(m.ctx, m.runtimeConfig.ModelDiscoveryTimeout)
	case "esc":
		v.state = providerStateInput
		v.focusIndex = int(providerFieldName)
		v.syncInputFocus()
		return true, nil
	default:
		return !m.matchesGlobalShortcut(message), nil
	}
}

func (v *providerPaneView) handleProviderErrorKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "enter", "esc":
		v.state = providerStateInput
		v.clearValidation()
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
		return true, nil
	default:
		return !m.matchesGlobalShortcut(message), nil
	}
}

func (v *providerPaneView) handleInputKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "esc":
		m.bottom.remove(providerViewID)
		return true, nil
	case "alt+1", "alt+p":
		v.applyPreset(model.DefaultProtonmanName)
		return true, nil
	case "alt+2", "alt+o":
		v.applyPreset(model.DefaultOpenCodeName)
		return true, nil
	case "alt+3", "alt+l":
		v.applyPreset(model.DefaultOllamaName)
		return true, nil
	case "alt+4":
		v.applyPreset(model.DefaultOpenAIName)
		return true, nil
	case "alt+5":
		v.applyPreset(model.DefaultAnthropicName)
		return true, nil
	case "ctrl+r":
		v.toggleProtocol()
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
		if !v.validateDraft() {
			return true, nil
		}
		if v.hasNameConflict(m) {
			v.state = providerStateConfirmOverwrite
			return true, nil
		}
		return true, v.beginFetch(m.ctx, m.runtimeConfig.ModelDiscoveryTimeout)
	default:
		if m.matchesGlobalShortcut(message) {
			return false, nil
		}
		return true, v.updateFocusedInput(message)
	}
}

func (v *providerPaneView) updateFocusedInput(message tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	switch v.focusIndex {
	case int(providerFieldName):
		v.clearFieldError(providerFieldName)
		v.nameInput, cmd = v.nameInput.Update(message)
	case int(providerFieldEndpoint):
		v.clearFieldError(providerFieldEndpoint)
		v.endpointInput, cmd = v.endpointInput.Update(message)
	case int(providerFieldAPIKey):
		v.clearFieldError(providerFieldAPIKey)
		v.apiKeyInput, cmd = v.apiKeyInput.Update(message)
	}
	return cmd
}

func (v *providerPaneView) saveSelectedModelCmd(modelID string) tea.Cmd {
	return saveProviderCmd(providerSaveRequest{providerName: strings.TrimSpace(v.nameInput.Value()), providerType: v.providerType, previousName: v.originalName, baseURL: strings.TrimSpace(v.endpointInput.Value()), apiKey: strings.TrimSpace(v.apiKeyInput.Value()), defaultModel: modelID, activate: v.activateOnSave})
}
