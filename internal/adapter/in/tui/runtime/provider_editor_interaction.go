package runtime

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelpicker"
	providerpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"image/color"
)

func (v *providerPaneView) Render(m *bubbleModel) string {
	if m == nil {
		return ""
	}
	v.resizeInputs(m.width)
	rows, tone := providerpane.ProviderEditorRows(providerEditorSnapshot(m, v))
	return renderProviderModal(m, paneToneColor(tone), rows)
}

func (v *providerPaneView) resizeInputs(width int) {
	inputWidth := maxInt(8, width-18)
	v.nameInput.SetWidth(inputWidth)
	v.endpointInput.SetWidth(inputWidth)
	v.apiKeyInput.SetWidth(inputWidth)
}

func providerEditorSnapshot(m *bubbleModel, v *providerPaneView) providerpane.ProviderEditorSnapshot {
	if m == nil || v == nil {
		return providerpane.ProviderEditorSnapshot{}
	}
	models := v.currentModels()
	items := make([]providerpane.ProviderEditorModel, 0, len(models))
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
		items = append(items, providerpane.ProviderEditorModel{Label: label, Free: model.IsFreeModel(md.ID), Limits: modelpicker.FormatTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens), Features: strings.Join(resolved.Features, ", "), Reasoning: remoteModelReasoningSummary(providerName, md, false)})
	}
	hasFreeModels := false
	for _, md := range v.models {
		if model.IsFreeModel(md.ID) {
			hasFreeModels = true
			break
		}
	}
	fieldErrors := [3]string{v.fieldErrors[providerFieldName], v.fieldErrors[providerFieldEndpoint], v.fieldErrors[providerFieldAPIKey]}
	return providerpane.ProviderEditorSnapshot{Width: m.width, Height: m.height, State: providerEditorPaneState(v.state), Name: v.nameInput.Value(), Endpoint: v.endpointInput.Value(), Spinner: m.spinner.View(), UserConfigPath: appdirs.UserConfigDisplay(), ErrorMessage: v.errorMessage, IsEditing: v.isEditing, ActivateOnSave: v.activateOnSave, ProviderType: v.providerType, ProtocolLabel: v.protocolLabel(), RequiresAPIKey: v.requiresAPIKey, NameInput: v.nameInput.View(), EndpointInput: v.endpointInput.View(), APIKeyInput: v.apiKeyInput.View(), FieldErrors: fieldErrors, Models: items, SelectedIndex: v.selectedIndex, ScrollOffset: v.scrollOffset, FilterFreeOnly: v.filterFreeOnly, HasFreeModels: hasFreeModels, TotalModels: len(v.models)}
}

func providerEditorPaneState(state providerPaneState) providerpane.ProviderEditorState {
	switch state {
	case providerStateFetching:
		return providerpane.ProviderEditorFetching
	case providerStateSelectModel:
		return providerpane.ProviderEditorSelectModel
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
	snapshot.State = providerpane.ProviderEditorInput
	rows, tone := providerpane.ProviderEditorRows(snapshot)
	return renderProviderModal(m, paneToneColor(tone), rows)
}

func renderProviderModal(m *bubbleModel, border color.Color, rows []string) string {
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

func (v *providerPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
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

func (v *providerPaneView) normalizeModelSelection() {
	models := v.currentModels()
	v.selectedIndex, v.scrollOffset, _ = normalizedPickerWindow(v.selectedIndex, v.scrollOffset, len(models), maxProviderSelectRows)
}

func (v *providerPaneView) handleFetchingKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	if message.String() == "esc" {
		v.cancelFetch()
		m.bottom.remove(providerViewID)
	}
	return true, nil
}

func (v *providerPaneView) handleModelSelectKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
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
		num := v.scrollOffset + int(message.String()[0]-'1')
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

func (v *providerPaneView) handleSaveErrorKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
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

func (v *providerPaneView) handleOverwriteKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
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

func (v *providerPaneView) handleProviderErrorKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
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

func (v *providerPaneView) handleInputKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
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

func (v *providerPaneView) saveSelectedModelCmd(modelID string) tea.Cmd {
	return saveProviderCmd(providerSaveRequest{providerName: strings.TrimSpace(v.nameInput.Value()), providerType: v.providerType, previousName: v.originalName, baseURL: strings.TrimSpace(v.endpointInput.Value()), apiKey: strings.TrimSpace(v.apiKeyInput.Value()), defaultModel: modelID, activate: v.activateOnSave})
}
