package runtime

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelpicker"
	providerpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"image/color"
)

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
		if reasoning := remoteModelReasoningSummary(providerName, md, false); reasoning != "" {
			parts = append(parts, reasoning)
		}
		items = append(items, providerEditorModelItem{model: md, title: title, description: strings.Join(parts, " · ")})
	}
	return items
}

func (v *providerPaneView) ensureModelPicker(m *bubbleModel) {
	if v == nil || m == nil {
		return
	}
	items := providerEditorListItems(v)
	if !v.modelPickerSet {
		delegate := list.NewDefaultDelegate()
		delegate.SetSpacing(0)
		v.modelPicker = list.New(items, delegate, maxInt(20, m.layout.width-8), maxInt(6, minInt(20, m.layout.height-4)))
		v.modelPicker.DisableQuitKeybindings()
		v.modelPicker.SetFilteringEnabled(false)
		v.modelPicker.SetShowStatusBar(false)
		v.modelPicker.SetStatusBarItemName("model", "models")
		v.modelPicker.InfiniteScrolling = true
		v.modelPickerSet = true
	} else {
		_ = v.modelPicker.SetItems(items)
	}
	v.modelPicker.Title = "Select model"
	if v.filterFreeOnly {
		v.modelPicker.Title += " · free"
	}
	v.modelPicker.SetSize(maxInt(20, m.layout.width-8), maxInt(6, minInt(20, m.layout.height-4)))
	mode := layoutModeForHeight(m.layout.height)
	v.modelPicker.SetShowHelp(mode != layoutTiny)
	v.modelPicker.SetShowPagination(mode == layoutNormal)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.modelPicker.SetDelegate(delegate)
}

func (v *providerPaneView) Render(m *bubbleModel) string {
	if m == nil {
		return ""
	}
	v.resizeInputs(m.layout.width)
	if v.state == providerStateSelectModel {
		v.ensureModelPicker(m)
		return renderProviderModal(m, accentAssistant, strings.Split(v.modelPicker.View(), "\n"))
	}
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
	fieldErrors := [3]string{v.fieldErrors[providerFieldName], v.fieldErrors[providerFieldEndpoint], v.fieldErrors[providerFieldAPIKey]}
	return providerpane.ProviderEditorSnapshot{Width: m.layout.width, Height: m.layout.height, State: providerEditorPaneState(v.state), Name: v.nameInput.Value(), Endpoint: v.endpointInput.Value(), Spinner: m.spinner.View(), UserConfigPath: appdirs.UserConfigDisplay(), ErrorMessage: v.errorMessage, IsEditing: v.isEditing, ActivateOnSave: v.activateOnSave, ProviderType: v.providerType, ProtocolLabel: v.protocolLabel(), RequiresAPIKey: v.requiresAPIKey, NameInput: v.nameInput.View(), EndpointInput: v.endpointInput.View(), APIKeyInput: v.apiKeyInput.View(), FieldErrors: fieldErrors}
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

func renderProviderInput(m *bubbleModel) string {
	if m == nil || m.bottom == nil {
		return ""
	}
	view, ok := m.bottom.find(providerViewID).(*providerPaneView)
	if !ok || view == nil {
		return ""
	}
	view.resizeInputs(m.layout.width)
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
	return maxInt(1, maxInt(1, m.layout.width-4)-6)
}
