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
	mode := layoutModeForHeight(ctx.height)
	v.picker.SetShowTitle(false)
	v.picker.SetShowFilter(v.picker.SettingFilter())
	v.picker.SetShowStatusBar(false)
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(false)

	rows := []string{brandStyle.Render("Switch Model")}
	if len(v.providerNames) > 1 {
		rows = append(rows, mutedStyle.Render("Provider: "+v.activeProviderName()+" · tab switch"))
	}
	rows = append(rows, "")
	showSelectionStatus := false

	switch {
	case v.loading:
		rows = append(rows, mutedStyle.Render("Loading models…"))
	case v.err != nil:
		rows = append(rows, errorStyle.Render("Failed to load models"), mutedStyle.Render(truncateWithEllipsis(v.err.Error(), maxInt(8, ctx.width-8))))
	case len(v.picker.Items()) == 0 && !v.picker.SettingFilter() && !v.picker.IsFiltered():
		rows = append(rows, mutedStyle.Render("No models available."))
	case len(v.picker.VisibleItems()) == 0 && strings.TrimSpace(v.picker.FilterValue()) != "":
		rows = append(rows, mutedStyle.Render("Search: "+v.picker.FilterValue()), mutedStyle.Render("No matches."))
	default:
		rows = append(rows, v.modelRows(ctx)...)
		showSelectionStatus = true
	}

	rows = append(rows, "", v.effortRow())
	if labels := v.effortLabels(); labels != "" {
		rows = append(rows, labels)
	}
	if mode != layoutTiny {
		rows = append(rows, "", modelSetupHelp(ctx.width, len(v.reasoningChoices) > 1))
	}
	if showSelectionStatus {
		if status := v.selectionStatus(ctx.width); status != "" {
			rows = append(rows, status)
		}
	}
	return renderModalRows(ctx, accentAssistant, rows)
}

func (v *modelSetupPaneView) modelRows(ctx paneRenderContext) []string {
	items := v.picker.VisibleItems()
	if len(items) == 0 {
		return nil
	}
	start, end := paneWindow(len(items), v.picker.Index(), maxModelSetupRows, layoutModeForHeight(ctx.height))
	rows := make([]string, 0, end-start+1)
	if v.picker.SettingFilter() || v.picker.IsFiltered() {
		rows = append(rows, mutedStyle.Render("Search: ")+userStyle.Render(v.picker.FilterValue()))
	}
	width := maxInt(1, ctx.width-8)
	for i := start; i < end; i++ {
		entry, ok := items[i].(modelListItem)
		if !ok {
			continue
		}
		prefix := "  "
		style := bodyStyle
		if i == v.picker.Index() {
			prefix = glyphPrompt
			style = brandStyle
		}
		label := truncateWithEllipsis(entry.Title(), maxInt(1, width-2))
		if entry.current {
			const marker = "(current)"
			markerWidth := len(marker)
			labelWidth := len([]rune(label))
			if gap := width - labelWidth - markerWidth - 2; gap >= 2 {
				label += strings.Repeat(" ", gap) + mutedStyle.Render(marker)
			} else {
				label = truncateWithEllipsis(label, maxInt(1, width-markerWidth-4)) + "  " + mutedStyle.Render(marker)
			}
		}
		rows = append(rows, prefix+style.Render(label))
	}
	return rows
}

func modelSetupHelp(width int, adjustableEffort bool) string {
	if adjustableEffort {
		return paneKeyboardHelp(width, "↑/↓", "Navigate", "←/→", "Effort", "enter", "Select", "esc", "Go Back")
	}
	return paneKeyboardHelp(width, "↑/↓", "Navigate", "enter", "Select", "esc", "Go Back")
}

func (v *modelSetupPaneView) effortRow() string {
	choices := v.reasoningChoices
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	if len(choices) <= 1 {
		return "Effort    " + brandStyle.Render(reasoningEffortLabel(choices[0]))
	}
	parts := make([]string, 0, len(choices)*2-1)
	for i := range choices {
		dot := mutedStyle.Render("●")
		if i == v.reasoningIndex {
			dot = brandStyle.Render("●")
		}
		parts = append(parts, dot)
		if i+1 < len(choices) {
			parts = append(parts, mutedStyle.Render("──────"))
		}
	}
	return "Effort    " + userStyle.Render("◀") + "   " + strings.Join(parts, "") + "   " + userStyle.Render("▶")
}

func (v *modelSetupPaneView) effortLabels() string {
	choices := v.reasoningChoices
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	if len(choices) <= 1 {
		return ""
	}
	labels := make([]string, 0, len(choices))
	for i, effort := range choices {
		label := reasoningEffortLabel(effort)
		if i == v.reasoningIndex {
			label = brandStyle.Render(label)
		} else {
			label = mutedStyle.Render(label)
		}
		labels = append(labels, label)
	}
	return "          " + strings.Join(labels, "     ")
}

func (v *modelSetupPaneView) selectionStatus(width int) string {
	md, ok := v.selectedRemoteModel()
	if !ok {
		return ""
	}
	return paneRightStatus(width, modelDisplayName(md)+" · "+reasoningEffortLabel(v.selectedReasoning()))
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
		v.resize(v.layoutWidth, v.layoutHeight)
		v.syncReasoningForSelection(v.reasoningPreference)
		return paneKeyResult{handled: true, cmd: cmd}
	}
	switch message.String() {
	case "/":
		v.picker.SetFilterState(list.Filtering)
		v.syncPickerProjection()
		v.resize(v.layoutWidth, v.layoutHeight)
		return paneKeyResult{handled: true}
	case "ctrl+u":
		v.picker.ResetFilter()
		v.syncPickerProjection()
		v.resize(v.layoutWidth, v.layoutHeight)
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
			v.resize(v.layoutWidth, v.layoutHeight)
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
