package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func (v *providerPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch v.state {
	case providerStateFetching:
		if message.String() == "esc" {
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
	switch message.String() {
	case "esc":
		v.state = providerStateInput
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	case "f":
		if v.isOpenCode() {
			v.filterFreeOnly = !v.filterFreeOnly
			v.modelPickerSet = false
			v.ensureModelPicker(ctx)
		}
		return paneKeyResult{handled: true}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		pageOffset := v.modelPicker.Paginator.Page * v.modelPicker.Paginator.PerPage
		idx := pageOffset + int(message.String()[0]-'1')
		if idx >= 0 && idx < len(v.currentModels()) {
			v.modelPicker.Select(idx)
		}
		return paneKeyResult{handled: true}
	case "enter":
		item, ok := v.modelPicker.SelectedItem().(providerEditorModelItem)
		if !ok {
			return paneKeyResult{handled: true}
		}
		v.selectedModel = item.model.ID
		v.state = providerStateSaving
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderSave, providerSave: v.providerSaveRequest(item.model.ID)}}
	case "up", "k", "down", "j", "home", "g", "end", "G", "pgup", "pgdown":
		updated, cmd := v.modelPicker.Update(message)
		v.modelPicker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleSaveErrorKey(message tea.KeyPressMsg) paneKeyResult {
	switch message.String() {
	case "enter":
		v.state = providerStateSaving
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderSave, providerSave: v.providerSaveRequest(v.selectedModel)}}
	case "esc":
		v.state = providerStateSelectModel
		v.errorMessage = ""
		return paneKeyResult{handled: true}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleOverwriteKey(message tea.KeyPressMsg) paneKeyResult {
	switch message.String() {
	case "enter":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderFetch}}
	case "esc":
		v.state = providerStateInput
		v.focusIndex = int(providerFieldName)
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleProviderErrorKey(message tea.KeyPressMsg) paneKeyResult {
	switch message.String() {
	case "enter", "esc":
		v.state = providerStateInput
		v.clearValidation()
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *providerPaneView) handleInputKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch message.String() {
	case "esc":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: providerViewID}}
	case "alt+1", "alt+p":
		v.applyPreset(model.DefaultProtonmanName)
		return paneKeyResult{handled: true}
	case "alt+2", "alt+o":
		v.applyPreset(model.DefaultOpenCodeName)
		return paneKeyResult{handled: true}
	case "alt+3", "alt+l":
		v.applyPreset(model.DefaultOllamaName)
		return paneKeyResult{handled: true}
	case "alt+4":
		v.applyPreset(model.DefaultOpenAIName)
		return paneKeyResult{handled: true}
	case "alt+5":
		v.applyPreset(model.DefaultAnthropicName)
		return paneKeyResult{handled: true}
	case "ctrl+r":
		v.toggleProtocol()
		return paneKeyResult{handled: true}
	case "tab", "down":
		v.focusIndex = (v.focusIndex + 1) % 3
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	case "shift+tab", "up":
		v.focusIndex = (v.focusIndex + 2) % 3
		v.syncInputFocus()
		return paneKeyResult{handled: true}
	case "enter":
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
