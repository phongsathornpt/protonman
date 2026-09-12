package runtime

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	providerdomain "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
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
	endpointIn.CharLimit = 256
	keyIn := textinput.New()
	keyIn.Prompt = glyphPrompt
	keyIn.Placeholder = draft.KeyPlaceholder
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.EchoCharacter = '•'
	keyIn.CharLimit = 256
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
		if key.Matches(message, paneKeys.Escape) {
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
	case key.Matches(message, paneKeys.Escape):
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
	case key.Matches(message, paneKeys.Confirm):
		item, ok := v.modelPicker.SelectedItem().(providerEditorModelItem)
		if !ok {
			return paneKeyResult{handled: true}
		}
		v.selectedModel = item.model.ID
		v.state = providerStateSaving
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderSave, providerSave: v.providerSaveRequest(item.model.ID)}}
	case key.Matches(message, paneKeys.Nav):
		updated, cmd := v.modelPicker.Update(message)
		v.modelPicker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleSaveErrorKey(message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneKeys.Confirm):
		v.state = providerStateSaving
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderSave, providerSave: v.providerSaveRequest(v.selectedModel)}}
	case key.Matches(message, paneKeys.Escape):
		v.state = providerStateSelectModel
		v.errorMessage = ""
		return paneKeyResult{handled: true}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleOverwriteKey(message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneKeys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderFetch}}
	case key.Matches(message, paneKeys.Escape):
		v.state = providerStateInput
		v.focusIndex = int(providerFieldName)
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleProviderErrorKey(message tea.KeyPressMsg) paneKeyResult {
	if key.Matches(message, paneKeys.Confirm, paneKeys.Escape) {
		v.state = providerStateInput
		v.clearValidation()
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
	}
	return paneKeyResult{handled: true}
}

func (v *providerPaneView) handleInputKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneKeys.Escape):
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
	case key.Matches(message, paneKeys.Confirm):
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

func (v *providerPaneView) updateFocusedInput(message tea.KeyPressMsg) tea.Cmd {
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
