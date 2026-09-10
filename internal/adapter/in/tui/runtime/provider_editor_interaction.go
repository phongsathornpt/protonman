package runtime

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

var providerEditorKeys = struct {
	ToggleFree, Protocol, NextField, PreviousField key.Binding
	Protonman, OpenCode, Ollama, OpenAI, Anthropic key.Binding
}{
	ToggleFree:    key.NewBinding(key.WithKeys("f")),
	Protocol:      key.NewBinding(key.WithKeys("ctrl+r")),
	NextField:     key.NewBinding(key.WithKeys("tab", "down")),
	PreviousField: key.NewBinding(key.WithKeys("shift+tab", "up")),
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
