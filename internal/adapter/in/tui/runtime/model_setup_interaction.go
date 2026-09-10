package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/providerio"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"strings"
)

func (v *modelSetupPaneView) Render(ctx paneRenderContext) string {
	v.initPicker()
	providerName := v.activeProviderName()
	v.picker.SetSize(maxInt(12, ctx.width-8), maxInt(4, min(8, ctx.height-10)))
	mode := layoutModeForHeight(ctx.height)
	v.picker.SetShowTitle(false)
	v.picker.SetShowStatusBar(false)
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(false)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)

	rows := []string{brandStyle.Render("Switch Model")}
	if len(v.providerNames) > 1 {
		rows = append(rows, mutedStyle.Render("Provider: "+providerName+" · tab switch"))
	} else {
		rows = append(rows, mutedStyle.Render("Provider: "+providerName))
	}

	switch {
	case v.loading:
		rows = append(rows, "", mutedStyle.Render("Loading models…"))
	case v.err != nil:
		rows = append(rows, "", errorStyle.Render("Failed to load models"), mutedStyle.Render(truncateWithEllipsis(v.err.Error(), maxInt(8, ctx.width-8))))
	case len(v.picker.Items()) == 0 && !v.picker.SettingFilter() && !v.picker.IsFiltered():
		rows = append(rows, "", mutedStyle.Render("No models available."))
	case len(v.picker.VisibleItems()) == 0 && strings.TrimSpace(v.picker.FilterValue()) != "":
		rows = append(rows, "", mutedStyle.Render("Search: "+v.picker.FilterValue()), mutedStyle.Render("No matches."))
	default:
		rows = append(rows, "")
		rows = append(rows, strings.Split(v.picker.View(), "\n")...)
	}

	rows = append(rows, "", v.reasoningRow())
	if mode != layoutTiny {
		rows = append(rows, mutedStyle.Render("↑↓ model · ←→ thinking · tab provider · enter apply · esc back"))
	}
	return renderModalRows(ctx, accentAssistant, rows)
}

func (v *modelSetupPaneView) reasoningRow() string {
	if len(v.reasoningChoices) == 0 {
		return mutedStyle.Render("Thinking  auto")
	}
	parts := make([]string, 0, len(v.reasoningChoices))
	for i, effort := range v.reasoningChoices {
		label := reasoningEffortLabel(effort)
		if i == v.reasoningIndex {
			label = brandStyle.Render(label)
		} else {
			label = mutedStyle.Render(label)
		}
		parts = append(parts, label)
	}
	return "Thinking  " + mutedStyle.Render("‹") + "  " + strings.Join(parts, "  ") + "  " + mutedStyle.Render("›")
}

func (v *modelSetupPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	if message.String() == "ctrl+p" || message.String() == "alt+m" {
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: modelSetupViewID}}
	}
	v.initPicker()
	if v.picker.SettingFilter() {
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		v.syncReasoningForSelection(v.reasoningPreference)
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
		v.syncReasoningForSelection(v.reasoningPreference)
		return paneKeyResult{handled: true}
	case "left", "h":
		v.moveReasoning(-1)
		return paneKeyResult{handled: true}
	case "right", "l":
		v.moveReasoning(1)
		return paneKeyResult{handled: true}
	case "esc":
		if v.picker.IsFiltered() {
			v.picker.ResetFilter()
			v.syncPickerProjection()
			v.syncReasoningForSelection(v.reasoningPreference)
			return paneKeyResult{handled: true}
		}
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: modelSetupViewID}}
	case "q":
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: modelSetupViewID}}
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
			v.reasoningPreference = v.selectedReasoning()
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionReloadModels}}
		}
		return paneKeyResult{handled: true}
	case "shift+tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex - 1 + len(v.providerNames)) % len(v.providerNames)
			v.reasoningPreference = v.selectedReasoning()
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionReloadModels}}
		}
		return paneKeyResult{handled: true}
	case "pgup", "pgdown", "up", "k", "down", "j", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		v.syncReasoningForSelection(v.reasoningPreference)
		return paneKeyResult{handled: true, cmd: cmd}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		pageOffset := v.picker.Paginator.Page * v.picker.Paginator.PerPage
		targetIdx := int(message.String()[0]-'1') + pageOffset
		if targetIdx >= 0 && targetIdx < len(v.models) {
			selected := v.models[targetIdx]
			reasoning := reasoningForModel(v.activeProviderName(), selected, v.reasoningPreference)
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionApplyModelSetup, providerName: v.activeProviderName(), modelID: selected.ID, reasoning: reasoning}}
		}
		return paneKeyResult{handled: true}
	case "enter":
		item, ok := v.picker.SelectedItem().(modelListItem)
		if !ok {
			return paneKeyResult{handled: true}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionApplyModelSetup, providerName: v.activeProviderName(), modelID: item.model.ID, reasoning: v.selectedReasoning()}}
	default:
		return paneKeyResult{}
	}
}

func persistModelSetupCmd(operationID asyncOperationID, gate *asyncOperationGate, providerName, modelID string, reasoning sdk.ReasoningEffort, unverified bool) tea.Cmd {
	return func() tea.Msg {
		if gate != nil && !gate.current(operationID) {
			return modelSetupAppliedMsg{operationID: operationID, providerName: providerName, modelID: modelID, reasoning: reasoning, unverified: unverified, err: errStaleConfigMutation}
		}
		err := providerio.SelectModel(providerName, modelID)
		return modelSetupAppliedMsg{operationID: operationID, providerName: providerName, modelID: modelID, reasoning: reasoning, unverified: unverified, err: err}
	}
}
