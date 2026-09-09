package runtime

import (
	"fmt"
	"strings"

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
