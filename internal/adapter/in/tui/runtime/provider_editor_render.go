package runtime

import (
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
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
		v.modelPicker = newMinimalList(items, delegate, maxInt(20, ctx.width-8), maxInt(6, minInt(20, ctx.height-4)))
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
			prefix, style := "  ", bodyStyle
			if index == v.modelPicker.Index() {
				prefix, style = "> ", brandStyle
			}
			listRows = append(listRows, prefix+style.Render(truncateWithEllipsis(item.title, maxInt(1, providerModalContentWidth(ctx)-4))))
		}
		help := paneKeyboardHelp(providerModalContentWidth(ctx), "↑/↓", "Navigate", "enter", "Select", "esc", "Go Back")
		status := ""
		if item, ok := v.modelPicker.SelectedItem().(providerEditorModelItem); ok {
			status = item.title
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
