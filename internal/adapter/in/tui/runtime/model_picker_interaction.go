package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/providerio"
	"strings"
)

func (v *modelSelectPaneView) Render(ctx paneRenderContext) string {
	v.initPicker()
	providerName := v.activeProviderName()
	v.picker.Title = "Models · " + providerName
	if len(v.providerNames) > 1 {
		v.picker.Title += " · tab switch"
	}
	v.picker.SetSize(maxInt(12, ctx.width-8), maxInt(4, min(8, ctx.height-6)))
	mode := layoutModeForHeight(ctx.height)
	v.picker.SetShowStatusBar(false)
	// Pagination stays hidden: resizing Bubbles list during Render can recompute paginator state.
	// Navigation still belongs to list.Update; this only suppresses the mutable presentation row.
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	if v.loading {
		rows := []string{brandStyle.Render("Models · " + providerName), mutedStyle.Render("Loading…"), mutedStyle.Render("esc close")}
		return renderModalRows(ctx, accentAssistant, rows)
	}
	if v.err != nil {
		rows := []string{brandStyle.Render("Models · " + providerName), errorStyle.Render("Failed to load models"), mutedStyle.Render(truncateWithEllipsis(v.err.Error(), maxInt(8, ctx.width-8))), mutedStyle.Render("r retry · p providers · esc close")}
		return renderModalRows(ctx, accentAssistant, rows)
	}
	if len(v.picker.Items()) == 0 && !v.picker.SettingFilter() && !v.picker.IsFiltered() {
		rows := []string{brandStyle.Render("Models · " + providerName), mutedStyle.Render("No models available."), mutedStyle.Render("a add provider · r retry · esc close")}
		return renderModalRows(ctx, accentAssistant, rows)
	}
	if len(v.picker.VisibleItems()) == 0 && strings.TrimSpace(v.picker.FilterValue()) != "" {
		rows := []string{brandStyle.Render("Models · " + providerName), mutedStyle.Render("Search: " + v.picker.FilterValue()), mutedStyle.Render("No matches."), mutedStyle.Render("esc clear filter")}
		return renderModalRows(ctx, accentAssistant, rows)
	}
	return renderModalRows(ctx, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *modelSelectPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	if message.String() == "ctrl+p" || message.String() == "alt+m" {
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: modelSelectViewID}}
	}
	v.initPicker()
	if v.picker.SettingFilter() {
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return paneKeyResult{handled: true, cmd: cmd}
	}
	switch message.String() {
	case "/":
		v.picker.SetFilterState(list.Filtering)
		v.syncPickerProjection()
		return paneKeyResult{handled: true}
	case "ctrl+u":
		v.picker.ResetFilter()
		v.syncPickerProjection()
		return paneKeyResult{handled: true}
	case "esc":
		if v.picker.IsFiltered() {
			v.picker.ResetFilter()
			v.syncPickerProjection()
			return paneKeyResult{handled: true}
		}
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: modelSelectViewID}}
	case "q":
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: modelSelectViewID}}
	case "p":
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionOpenProviderSelect}}
	case "a":
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionOpenProviderEditor}}
	case "r":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionReloadModels, runSlash: true}}
	case "tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex + 1) % len(v.providerNames)
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionReloadModels}}
		}
		return paneKeyResult{handled: true}
	case "shift+tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex - 1 + len(v.providerNames)) % len(v.providerNames)
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionReloadModels}}
		}
		return paneKeyResult{handled: true}
	case "pgup", "pgdown", "up", "k", "down", "j", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return paneKeyResult{handled: true, cmd: cmd}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		pageOffset := v.picker.Paginator.Page * v.picker.Paginator.PerPage
		targetIdx := int(message.String()[0]-'1') + pageOffset
		if targetIdx >= 0 && targetIdx < len(v.models) {
			selected := v.models[targetIdx]
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionSelectModel, providerName: v.activeProviderName(), modelID: selected.ID}}
		}
		return paneKeyResult{handled: true}
	case "enter":
		item, ok := v.picker.SelectedItem().(modelListItem)
		if !ok {
			return paneKeyResult{handled: true}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionSelectModel, providerName: v.activeProviderName(), modelID: item.model.ID}}
	default:
		return paneKeyResult{}
	}
}

func saveDefaultModelCmd(operationID asyncOperationID, providerName, modelID string) tea.Cmd {
	return saveModelSelectionCmd(operationID, nil, providerName, modelID, false)
}

func saveModelSelectionCmd(operationID asyncOperationID, gate *asyncOperationGate, providerName, modelID string, unverified bool) tea.Cmd {
	return func() tea.Msg {
		if gate != nil && !gate.current(operationID) {
			return modelSelectedMsg{operationID: operationID, providerName: providerName, modelID: modelID, unverified: unverified, err: errStaleConfigMutation}
		}
		err := providerio.SelectModel(providerName, modelID)
		return modelSelectedMsg{operationID: operationID, providerName: providerName, modelID: modelID, unverified: unverified, err: err}
	}
}
