package runtime

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelpicker"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	providerdomain "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
	providerpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
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

type providerField = providerdomain.Field

const (
	providerFieldName     = providerdomain.FieldName
	providerFieldEndpoint = providerdomain.FieldEndpoint
	providerFieldAPIKey   = providerdomain.FieldAPIKey
	providerFieldCount    = providerdomain.FieldCount
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
	modelPicker    list.Model
	modelPickerSet bool
	filterFreeOnly bool
	errorMessage   string
	fieldErrors    [providerFieldCount]string
	fetchRequestID asyncOperationID
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
	pv.filterFreeOnly = pv.isOpenCode() && strings.TrimSpace(cfg.APIKey) == ""
	return pv
}

func newProviderPaneViewWithPreset(preset string) *providerPaneView {
	draft := providerdomain.NewEditorDraft(preset)
	nameIn := textinput.New()
	nameIn.Prompt = glyphPrompt
	nameIn.Placeholder = "provider name…"
	if draft.Name != "" {
		nameIn.SetValue(draft.Name)
	}
	nameIn.CharLimit = 64
	endpointIn := textinput.New()
	endpointIn.Prompt = glyphPrompt
	endpointIn.Placeholder = "https://api.example.com/v1"
	if draft.Endpoint != "" {
		endpointIn.SetValue(draft.Endpoint)
	}
	endpointIn.CharLimit = 2048
	keyIn := textinput.New()
	keyIn.Prompt = glyphPrompt
	keyIn.Placeholder = draft.KeyPlaceholder
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.EchoCharacter = '•'
	keyIn.CharLimit = 4096
	pv := &providerPaneView{
		state: providerStateInput, focusIndex: int(draft.FocusField), presetID: draft.PresetID,
		providerType: draft.ProviderType, requiresAPIKey: draft.RequiresAPIKey,
		nameInput: nameIn, endpointInput: endpointIn, apiKeyInput: keyIn,
		filterFreeOnly: draft.FilterFreeOnly, activateOnSave: true,
	}
	pv.syncInputFocus()
	return pv
}

func (*providerPaneView) ID() string {
	return providerViewID
}

func (*providerPaneView) PresentationMode() panePresentationMode {
	return paneBlocking
}

func (v *providerPaneView) isOpenCode() bool {
	return model.IsProvider(model.DefaultOpenCodeName, v.nameInput.Value(), v.endpointInput.Value())
}

func (v *providerPaneView) applyPreset(preset string) {
	preset = providerdomain.NormalizePresetID(preset)
	if p := model.LookupPreset(preset); p != nil {
		previousName := strings.TrimSpace(v.nameInput.Value())
		v.nameInput.SetValue(p.ID)
		v.endpointInput.SetValue(p.BaseURL)
		if !strings.EqualFold(previousName, p.ID) {
			v.apiKeyInput.SetValue("")
		}
		v.apiKeyInput.Placeholder = providerdomain.KeyPlaceholder(*p)
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
	return providerdomain.ProtocolLabel(v.providerType, v.presetID)
}

func (v *providerPaneView) toggleProtocol() {
	v.providerType = providerdomain.ToggleProtocol(v.providerType, v.presetID)
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
	validation := providerdomain.Validate(v.nameInput.Value(), v.endpointInput.Value(), v.apiKeyInput.Value(), v.requiresAPIKey)
	v.fieldErrors = validation.Errors
	if !validation.Valid {
		v.focusIndex = int(validation.FirstError)
		v.syncInputFocus()
		return false
	}
	v.nameInput.SetValue(validation.Name)
	v.endpointInput.SetValue(validation.Endpoint)
	v.apiKeyInput.SetValue(validation.APIKey)
	return true
}

func (v *providerPaneView) hasNameConflict(ctx paneRenderContext) bool {
	return providerdomain.HasNameConflict(ctx.providers, v.nameInput.Value(), v.originalName)
}

func (v *providerPaneView) setFetchedModels(models []model.RemoteModel) {
	v.models, _ = providerdomain.SortFetchedModels(models, v.isOpenCode())
	v.filterFreeOnly = v.isOpenCode() && strings.TrimSpace(v.apiKeyInput.Value()) == ""
	v.modelPickerSet = false
}

func (v *providerPaneView) currentModels() []model.RemoteModel {
	return providerdomain.CurrentModels(v.models, v.filterFreeOnly)
}

var providerEditorKeys = struct {
	ToggleFree, Protocol, NextField, PreviousField key.Binding
	Protonman, OpenCode, Ollama, OpenAI, Anthropic key.Binding
}{
	ToggleFree:    key.NewBinding(key.WithKeys("f")),
	Protocol:      key.NewBinding(key.WithKeys("ctrl+r")),
	NextField:     key.NewBinding(key.WithKeys("tab", "down")),
	PreviousField: key.NewBinding(key.WithKeys("up")),
	Protonman:     key.NewBinding(key.WithKeys("alt+1", "alt+p")),
	OpenCode:      key.NewBinding(key.WithKeys("alt+2", "alt+o")),
	Ollama:        key.NewBinding(key.WithKeys("alt+3", "alt+l")),
	OpenAI:        key.NewBinding(key.WithKeys("alt+4")),
	Anthropic:     key.NewBinding(key.WithKeys("alt+5")),
}

func (v *providerPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch v.state {
	case providerStateFetching:
		if key.Matches(message, paneutil.Keys.Escape) {
			v.cancelFetch()
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: providerViewID}}
		}
		return paneKeyResult{handled: true}
	case providerStateSelectModel:
		return v.handleModelSelectKey(ctx, message)
	case providerStateSaving:
		return paneKeyResult{handled: true}
	case providerStateSaveError:
		return v.handleSaveErrorKey(message)
	case providerStateConfirmOverwrite:
		return v.handleOverwriteKey(message)
	case providerStateError:
		return v.handleProviderErrorKey(message)
	default:
		return v.handleInputKey(ctx, message)
	}
}

func (v *providerPaneView) HandlePanePaste(_ paneRenderContext, message tea.PasteMsg) paneKeyResult {
	if v.state != providerStateInput {
		return paneKeyResult{}
	}
	clean := tea.PasteMsg{Content: strings.TrimSpace(message.Content)}
	if clean.Content == "" {
		return paneKeyResult{handled: true}
	}
	cmd := v.updateFocusedInput(clean)
	return paneKeyResult{handled: true, cmd: cmd}
}

func (v *providerPaneView) HandlePaneMsg(_ paneRenderContext, msg tea.Msg) paneKeyResult {
	if v.state != providerStateInput {
		return paneKeyResult{}
	}
	cmd := v.updateFocusedInput(msg)
	return paneKeyResult{handled: true, cmd: cmd}
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

func (v *providerPaneView) handleModelSelectKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.ensureModelPicker(ctx)
	switch {
	case key.Matches(message, paneutil.Keys.Escape):
		v.state = providerStateInput
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	case key.Matches(message, providerEditorKeys.ToggleFree):
		if v.isOpenCode() && strings.TrimSpace(v.apiKeyInput.Value()) != "" {
			v.filterFreeOnly = !v.filterFreeOnly
			v.modelPickerSet = false
			v.ensureModelPicker(ctx)
		}
		return paneKeyResult{handled: true}
	case message.Text >= "1" && message.Text <= "9":
		pageOffset := v.modelPicker.Paginator.Page * v.modelPicker.Paginator.PerPage
		idx := pageOffset + int(message.Text[0]-'1')
		if idx >= 0 && idx < len(v.currentModels()) {
			v.modelPicker.Select(idx)
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Confirm):
		item, ok := v.modelPicker.SelectedItem().(providerEditorModelItem)
		if !ok {
			return paneKeyResult{handled: true}
		}
		v.selectedModel = item.model.ID
		v.state = providerStateSaving
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderSave, providerSave: v.providerSaveRequest(item.model.ID)}}
	case key.Matches(message, paneutil.Keys.Nav):
		updated, cmd := v.modelPicker.Update(message)
		v.modelPicker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleSaveErrorKey(message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneutil.Keys.Confirm):
		v.state = providerStateSaving
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderSave, providerSave: v.providerSaveRequest(v.selectedModel)}}
	case key.Matches(message, paneutil.Keys.Escape):
		v.state = providerStateSelectModel
		v.errorMessage = ""
		return paneKeyResult{handled: true}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleOverwriteKey(message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneutil.Keys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderFetch}}
	case key.Matches(message, paneutil.Keys.Escape):
		v.state = providerStateInput
		v.focusIndex = int(providerFieldName)
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleProviderErrorKey(message tea.KeyPressMsg) paneKeyResult {
	if key.Matches(message, paneutil.Keys.Confirm, paneutil.Keys.Escape) {
		v.state = providerStateInput
		v.clearValidation()
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
	}
	return paneKeyResult{handled: true}
}

func (v *providerPaneView) handleInputKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneutil.Keys.Escape):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: providerViewID}}
	case key.Matches(message, providerEditorKeys.Protonman):
		v.applyPreset(model.DefaultProtonmanName)
		return paneKeyResult{handled: true}
	case key.Matches(message, providerEditorKeys.OpenCode):
		v.applyPreset(model.DefaultOpenCodeName)
		return paneKeyResult{handled: true}
	case key.Matches(message, providerEditorKeys.Ollama):
		v.applyPreset(model.DefaultOllamaName)
		return paneKeyResult{handled: true}
	case key.Matches(message, providerEditorKeys.OpenAI):
		v.applyPreset(model.DefaultOpenAIName)
		return paneKeyResult{handled: true}
	case key.Matches(message, providerEditorKeys.Anthropic):
		v.applyPreset(model.DefaultAnthropicName)
		return paneKeyResult{handled: true}
	case key.Matches(message, providerEditorKeys.Protocol):
		v.toggleProtocol()
		return paneKeyResult{handled: true}
	case key.Matches(message, providerEditorKeys.NextField):
		v.focusIndex = (v.focusIndex + 1) % 3
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	case key.Matches(message, providerEditorKeys.PreviousField):
		v.focusIndex = (v.focusIndex + 2) % 3
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Confirm):
		if !v.validateDraft() {
			return paneKeyResult{handled: true}
		}
		if v.hasNameConflict(ctx) {
			v.state = providerStateConfirmOverwrite
			return paneKeyResult{handled: true}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderFetch}}
	default:
		return paneKeyResult{handled: true, cmd: v.updateFocusedInput(message)}
	}
}

func (v *providerPaneView) updateFocusedInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch v.focusIndex {
	case int(providerFieldName):
		v.clearFieldError(providerFieldName)
		v.nameInput, cmd = v.nameInput.Update(msg)
		if err, ok := msg.(error); ok && err != nil {
			v.fieldErrors[providerFieldName] = "clipboard unavailable"
		} else if v.nameInput.Err != nil {
			v.fieldErrors[providerFieldName] = "clipboard unavailable"
			v.nameInput.Err = nil
		}
	case int(providerFieldEndpoint):
		v.clearFieldError(providerFieldEndpoint)
		v.endpointInput, cmd = v.endpointInput.Update(msg)
		if err, ok := msg.(error); ok && err != nil {
			v.fieldErrors[providerFieldEndpoint] = "clipboard unavailable"
		} else if v.endpointInput.Err != nil {
			v.fieldErrors[providerFieldEndpoint] = "clipboard unavailable"
			v.endpointInput.Err = nil
		}
	case int(providerFieldAPIKey):
		v.clearFieldError(providerFieldAPIKey)
		v.apiKeyInput, cmd = v.apiKeyInput.Update(msg)
		if strings.ContainsAny(v.apiKeyInput.Value(), " \t\r\n") {
			v.apiKeyInput.SetValue(strings.TrimSpace(v.apiKeyInput.Value()))
		}
		if err, ok := msg.(error); ok && err != nil {
			v.fieldErrors[providerFieldAPIKey] = "clipboard unavailable (use terminal paste: Ctrl+Shift+V)"
		} else if v.apiKeyInput.Err != nil {
			v.fieldErrors[providerFieldAPIKey] = "clipboard unavailable (use terminal paste: Ctrl+Shift+V)"
			v.apiKeyInput.Err = nil
		}
	}
	return cmd
}

type modelsFetchedMsg struct {
	providerName string
	baseURL      string
	apiKey       string
	models       []model.RemoteModel
	requestID    asyncOperationID
	err          error
}

type providerSavedMsg struct {
	operationID  asyncOperationID
	providerName string
	providerType string
	previousName string
	baseURL      string
	apiKey       string
	modelID      string
	activated    bool
	err          error
}

type providerFetchRequest struct {
	ctx              context.Context
	requestID        asyncOperationID
	providerName     string
	providerType     string
	baseURL          string
	apiKey           string
	discoveryTimeout time.Duration
}

func (v *providerPaneView) beginFetch(parent context.Context, models app.Models, timeouts ...time.Duration) tea.Cmd {
	discoveryTimeout := runtimepolicy.ModelDiscoveryTimeout
	if len(timeouts) > 0 && timeouts[0] > 0 {
		discoveryTimeout = timeouts[0]
	}
	if v.fetchCancel != nil {
		v.fetchCancel()
	}
	if parent == nil {
		v.fetchRequestID = 0
		v.state = providerStateError
		v.errorMessage = errMissingRuntimeContext.Error()
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	v.fetchCancel = cancel
	v.fetchRequestID = nextAsyncOperationID()
	v.state = providerStateFetching
	return fetchProviderModelsCmd(models, providerFetchRequest{
		ctx:              ctx,
		requestID:        v.fetchRequestID,
		providerName:     strings.TrimSpace(v.nameInput.Value()),
		providerType:     v.providerType,
		baseURL:          strings.TrimSpace(v.endpointInput.Value()),
		apiKey:           strings.TrimSpace(v.apiKeyInput.Value()),
		discoveryTimeout: discoveryTimeout,
	})
}

func (v *providerPaneView) cancelFetch() {
	if v.fetchCancel == nil {
		return
	}
	v.fetchCancel()
	v.fetchCancel = nil
}

func fetchProviderModelsCmd(models app.Models, request providerFetchRequest) tea.Cmd {
	return func() tea.Msg {
		discovered, err := providerdomain.Discover(request.ctx, models, providerdomain.FetchRequest{
			ProviderName: request.providerName,
			ProviderType: request.providerType,
			BaseURL:      request.baseURL,
			APIKey:       request.apiKey,
			Timeout:      request.discoveryTimeout,
		})
		return modelsFetchedMsg{
			providerName: request.providerName,
			baseURL:      request.baseURL,
			apiKey:       request.apiKey,
			models:       discovered,
			requestID:    request.requestID,
			err:          err,
		}
	}
}

type providerSaveRequest struct {
	providerName string
	providerType string
	previousName string
	baseURL      string
	apiKey       string
	defaultModel string
	activate     bool
}

func saveProviderCmd(providers app.Providers, operationID asyncOperationID, gate *asyncOperationGate, request providerSaveRequest) tea.Cmd {
	return func() tea.Msg {
		if !gate.current(operationID) {
			return providerSavedMsg{operationID: operationID, err: errStaleConfigMutation}
		}
		err := providerdomain.Save(providers, providerdomain.SaveRequest{
			ProviderName: request.providerName,
			ProviderType: request.providerType,
			PreviousName: request.previousName,
			BaseURL:      request.baseURL,
			APIKey:       request.apiKey,
			DefaultModel: request.defaultModel,
			Activate:     request.activate,
		})
		return providerSavedMsg{
			operationID:  operationID,
			providerName: request.providerName,
			providerType: request.providerType,
			previousName: request.previousName,
			baseURL:      request.baseURL,
			apiKey:       request.apiKey,
			modelID:      request.defaultModel,
			activated:    request.activate,
			err:          err,
		}
	}
}

func (v *providerPaneView) providerSaveRequest(modelID string) providerSaveRequest {
	return providerSaveRequest{
		providerName: strings.TrimSpace(v.nameInput.Value()),
		providerType: v.providerType,
		previousName: v.originalName,
		baseURL:      strings.TrimSpace(v.endpointInput.Value()),
		apiKey:       strings.TrimSpace(v.apiKeyInput.Value()),
		defaultModel: modelID,
		activate:     v.activateOnSave,
	}
}

type providerEditorModelItem struct {
	model       model.RemoteModel
	title       string
	description string
}

func (i providerEditorModelItem) FilterValue() string { return i.title + " " + i.description }
func (i providerEditorModelItem) Title() string       { return i.title }
func (i providerEditorModelItem) Description() string { return i.description }

func providerEditorListItems(v *providerPaneView) []list.Item {
	if v == nil {
		return nil
	}
	models := v.currentModels()
	items := make([]list.Item, 0, len(models))
	providerName := strings.TrimSpace(v.nameInput.Value())
	for _, md := range models {
		resolved := model.ResolveRemoteMetadata(providerName, md)
		title := strings.TrimSpace(md.ID)
		if name := strings.TrimSpace(md.Name); name != "" && !strings.EqualFold(name, title) {
			if title == "" {
				title = name
			} else {
				title = fmt.Sprintf("%s (%s)", name, title)
			}
		}
		parts := make([]string, 0, 4)
		if model.IsFreeModel(md.ID) {
			parts = append(parts, "free")
		}
		if limits := modelpicker.FormatTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens); limits != "" {
			parts = append(parts, limits)
		}
		if len(resolved.Features) > 0 {
			parts = append(parts, strings.Join(resolved.Features, ", "))
		}
		if reasoning := reasoningpolicy.Summary(providerName, md, false); reasoning != "" {
			parts = append(parts, reasoning)
		}
		items = append(items, providerEditorModelItem{model: md, title: title, description: strings.Join(parts, " · ")})
	}
	return items
}

func (v *providerPaneView) ensureModelPicker(ctx paneRenderContext) {
	if v == nil {
		return
	}
	items := providerEditorListItems(v)
	if !v.modelPickerSet {
		delegate := list.NewDefaultDelegate()
		delegate.SetSpacing(0)
		v.modelPicker = paneutil.NewMinimalList(items, delegate, maxInt(20, ctx.width-8), maxInt(6, minInt(20, ctx.height-4)))
		v.modelPicker.SetFilteringEnabled(false)
		v.modelPicker.SetStatusBarItemName("model", "models")
		v.modelPicker.InfiniteScrolling = true
		v.modelPickerSet = true
	} else {
		_ = v.modelPicker.SetItems(items)
	}
	visibleRows := 7
	switch layoutModeForHeight(ctx.height) {
	case layoutTiny:
		visibleRows = 2
	case layoutCompact:
		visibleRows = 4
	}
	v.modelPicker.SetSize(maxInt(20, ctx.width-8), visibleRows)
}

func (v *providerPaneView) Render(ctx paneRenderContext) string {
	v.resizeInputs(ctx.width)
	if v.state == providerStateSelectModel {
		v.ensureModelPicker(ctx)
		items := v.modelPicker.VisibleItems()
		start, end := paneWindow(len(items), v.modelPicker.Index(), 7, layoutModeForHeight(ctx.height))
		listRows := make([]string, 0, end-start)
		for index := start; index < end; index++ {
			item, ok := items[index].(providerEditorModelItem)
			if !ok {
				continue
			}
			prefix := "  "
			style := bodyStyle
			if index == v.modelPicker.Index() {
				prefix = brandStyle.Render(glyphPrompt)
				style = bodyStyle.Bold(true)
			}
			listRows = append(listRows, prefix+style.Render(truncateWithEllipsis(item.title, maxInt(1, providerModalContentWidth(ctx)-4))))
		}
		help := paneKeyboardHelp(providerModalContentWidth(ctx), "↑/↓", "Navigate", "enter", "Select", "esc", "Go Back")
		status := ""
		if item, ok := v.modelPicker.SelectedItem().(providerEditorModelItem); ok {
			status = item.title
		}
		if len(items) > end-start {
			if status != "" {
				status = fmt.Sprintf("%d-%d of %d · %s", start+1, end, len(items), status)
			} else {
				status = fmt.Sprintf("%d-%d of %d", start+1, end, len(items))
			}
		}
		title := "Select Model"
		if v.filterFreeOnly {
			title += " · Free"
		}
		return renderProviderModal(ctx, accentAssistant, paneSection(title, listRows, help, status, providerModalContentWidth(ctx)+4))
	}
	rows, tone := providerpane.ProviderEditorRows(providerEditorSnapshot(ctx, v))
	if len(rows) > 1 && layoutModeForHeight(ctx.height) != layoutTiny {
		rows = appendPaneGroup(rows[:1], rows[1:]...)
	}
	help := providerEditorKeyboardHelp(providerModalContentWidth(ctx), v.state, v.isEditing, v.activateOnSave)
	if help != "" {
		rows = appendPaneGroup(rows, help)
	}
	status := strings.TrimSpace(v.nameInput.Value())
	if v.isEditing && !v.activateOnSave && v.state == providerStateInput {
		status = strings.TrimSpace(status + " · active stays")
	}
	if status != "" {
		rows = append(rows, paneRightStatus(providerModalContentWidth(ctx)+4, status))
	}
	return renderProviderModal(ctx, paneToneColor(tone), rows)
}

func providerEditorKeyboardHelp(width int, state providerPaneState, editing, activateOnSave bool) string {
	switch state {
	case providerStateFetching:
		return paneKeyboardHelp(width, "esc", "Cancel")
	case providerStateConfirmOverwrite:
		return paneKeyboardHelp(width, "enter", "Overwrite", "esc", "Go Back", "ctrl+c", "Cancel")
	case providerStateSaveError:
		return paneKeyboardHelp(width, "enter", "Retry", "esc", "Go Back", "ctrl+c", "Cancel")
	case providerStateError:
		return paneKeyboardHelp(width, "enter", "Go Back", "esc", "Go Back")
	case providerStateSaving:
		return ""
	default:
		action := "Connect"
		if editing && !activateOnSave {
			action = "Save"
		}
		return paneKeyboardHelp(width, "tab", "Fields", "ctrl+r", "Protocol", "enter", action, "esc", "Go Back")
	}
}

func (v *providerPaneView) resizeInputs(width int) {
	inputWidth := maxInt(8, width-18)
	v.nameInput.SetWidth(inputWidth)
	v.endpointInput.SetWidth(inputWidth)
	v.apiKeyInput.SetWidth(inputWidth)
}

func providerEditorSnapshot(ctx paneRenderContext, v *providerPaneView) providerpane.ProviderEditorSnapshot {
	if v == nil {
		return providerpane.ProviderEditorSnapshot{}
	}
	fieldErrors := [3]string{v.fieldErrors[providerFieldName], v.fieldErrors[providerFieldEndpoint], v.fieldErrors[providerFieldAPIKey]}
	return providerpane.ProviderEditorSnapshot{Width: ctx.width, Height: ctx.height, State: providerEditorPaneState(v.state), Name: v.nameInput.Value(), Endpoint: v.endpointInput.Value(), Spinner: ctx.spinner, UserConfigPath: appdirs.UserConfigDisplay(), ErrorMessage: v.errorMessage, IsEditing: v.isEditing, ActivateOnSave: v.activateOnSave, ProviderType: v.providerType, ProtocolLabel: v.protocolLabel(), RequiresAPIKey: v.requiresAPIKey, NameInput: v.nameInput.View(), EndpointInput: v.endpointInput.View(), APIKeyInput: v.apiKeyInput.View(), FieldErrors: fieldErrors}
}

func providerEditorPaneState(state providerPaneState) providerpane.ProviderEditorState {
	switch state {
	case providerStateFetching:
		return providerpane.ProviderEditorFetching
	case providerStateConfirmOverwrite:
		return providerpane.ProviderEditorConfirmOverwrite
	case providerStateSaving:
		return providerpane.ProviderEditorSaving
	case providerStateSaveError:
		return providerpane.ProviderEditorSaveError
	case providerStateError:
		return providerpane.ProviderEditorError
	default:
		return providerpane.ProviderEditorInput
	}
}

func renderProviderModal(ctx paneRenderContext, border color.Color, rows []string) string {
	contentWidth := providerModalContentWidth(ctx)
	wrappedRows := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == "" || lipgloss.Width(row) <= contentWidth {
			wrappedRows = append(wrappedRows, row)
			continue
		}
		wrappedRows = append(wrappedRows, strings.Split(wrapWords(row, contentWidth), "\n")...)
	}
	return renderModalRows(ctx, border, wrappedRows)
}

func providerModalContentWidth(ctx paneRenderContext) int {
	return maxInt(1, maxInt(1, ctx.width-4)-6)
}
