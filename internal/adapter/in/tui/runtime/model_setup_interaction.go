package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/providerio"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
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
		selected := i == v.picker.Index()
		rows = append(rows, renderModelRow(entry, selected, width))
	}
	return rows
}

func renderModelRow(entry modelListItem, selected bool, width int) string {
	const (
		markerWidth   = 2
		freeWidth     = 4
		currentWidth  = 9
		metadataGap   = 2
		nameMetaGap   = 3
		metadataWidth = freeWidth + metadataGap + currentWidth
	)
	prefix := "  "
	nameStyle := bodyStyle
	if selected {
		prefix = "> "
		nameStyle = brandStyle
	}

	available := maxInt(1, width-markerWidth)
	showMetadata := available-metadataWidth-nameMetaGap >= 8
	nameWidth := available
	if showMetadata {
		nameWidth = available - metadataWidth - nameMetaGap
	}
	name := truncateWithEllipsis(entry.Title(), nameWidth)
	row := prefix + nameStyle.Render(name)
	if !showMetadata {
		return row
	}

	free := ""
	current := ""
	if model.IsFreeModel(entry.model.ID) {
		free = "FREE"
	}
	if entry.current {
		current = "(current)"
	}
	gap := nameWidth - len([]rune(name)) + nameMetaGap
	metadata := strings.TrimRight(padRight(free, freeWidth)+strings.Repeat(" ", metadataGap)+padRight(current, currentWidth), " ")
	return row + strings.Repeat(" ", gap) + mutedStyle.Render(metadata)
}

func padRight(value string, width int) string {
	valueWidth := len([]rune(value))
	if valueWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-valueWidth)
}

func modelSetupHelp(width int, adjustableEffort bool) string {
	if adjustableEffort {
		return paneKeyboardHelp(width, "↑/↓", "Navigate", "←/→", "Effort", "enter", "Select", "esc", "Go Back")
	}
	return paneKeyboardHelp(width, "↑/↓", "Navigate", "enter", "Select", "esc", "Go Back")
}

func (v *modelSetupPaneView) effortLayout() (string, string) {
	choices := v.reasoningChoices
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	if len(choices) <= 1 {
		return "Effort    " + brandStyle.Render(reasoningEffortLabel(choices[0])), ""
	}

	labels := make([]string, len(choices))
	slotWidth := 5
	for i, effort := range choices {
		labels[i] = reasoningEffortLabel(effort)
		if width := len([]rune(labels[i])) + 2; width > slotWidth {
			slotWidth = width
		}
	}

	dots := make([]string, len(choices))
	styledLabels := make([]string, len(choices))
	for i, label := range labels {
		left := (slotWidth - 1) / 2
		right := slotWidth - left - 1
		dot := mutedStyle.Render("●")
		if i == v.reasoningIndex {
			dot = brandStyle.Render("●")
		}
		dots[i] = strings.Repeat(" ", left) + dot + strings.Repeat(" ", right)

		labelLeft := (slotWidth - len([]rune(label))) / 2
		labelRight := slotWidth - labelLeft - len([]rune(label))
		styled := mutedStyle.Render(label)
		if i == v.reasoningIndex {
			styled = brandStyle.Render(label)
		}
		styledLabels[i] = strings.Repeat(" ", labelLeft) + styled + strings.Repeat(" ", labelRight)
	}

	connector := mutedStyle.Render(strings.Repeat("─", 2))
	track := strings.Join(dots, connector)
	labelRow := strings.Join(styledLabels, "  ")
	const effortPrefix = "Effort    "
	controlPrefix := effortPrefix + userStyle.Render("◀") + " "
	labelPrefix := strings.Repeat(" ", len([]rune(effortPrefix))+2)
	return controlPrefix + track + " " + userStyle.Render("▶"), labelPrefix + labelRow
}

func (v *modelSetupPaneView) effortRow() string {
	row, _ := v.effortLayout()
	return row
}

func (v *modelSetupPaneView) effortLabels() string {
	_, labels := v.effortLayout()
	return labels
}

func (v *modelSetupPaneView) selectionStatus(width int) string {
	md, ok := v.selectedRemoteModel()
	if !ok {
		return ""
	}
	return paneRightStatus(width, modelDisplayName(md)+" · "+reasoningEffortLabel(v.selectedReasoning()))
}

func (v *modelSetupPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
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
