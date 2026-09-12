package runtime

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelpicker"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelsetup"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	providerdomain "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const modelSetupViewID = "model_setup"
const maxModelSetupRows = 6

type modelSetupAppliedMsg struct {
	operationID  asyncOperationID
	providerName string
	modelID      string
	reasoning    sdk.ReasoningEffort
	unverified   bool
	err          error
}

type modelSetupPaneView struct {
	picker              list.Model
	pickerReady         bool
	models              []model.RemoteModel
	allModels           []model.RemoteModel
	providerNames       []string
	providerIndex       int
	fetchRequestID      asyncOperationID
	fetchCancel         context.CancelFunc
	loading             bool
	err                 error
	reasoningChoices    []sdk.ReasoningEffort
	reasoningIndex      int
	reasoningPreference sdk.ReasoningEffort
	layoutWidth         int
	layoutHeight        int
}

func newModelSetupPaneView(m *bubbleModel) *modelSetupPaneView {
	if m != nil {
		m.activeModelSetup = 0
		m.configMutationGate.invalidate()
	}
	providers, providerIdx := modelpicker.ProviderNames(m.providers, m.activeProvider)
	var modelsList []model.RemoteModel
	hasFreshCatalog := false
	if m != nil && providerIdx < len(providers) {
		providerName := providers[providerIdx]
		modelsList, hasFreshCatalog = m.modelCatalogs.FreshModels(providerName, time.Now(), m.runtimeConfig.ModelCatalogTTL)
		if cfg, ok := m.providers[modelcatalog.NormalizeProviderKey(providerName)]; ok {
			modelsList = modelcatalog.VisibleForAccess(providerName, cfg.BaseURL, cfg.APIKey, modelsList)
		}
	}
	if !hasFreshCatalog {
		modelsList = nil
	}
	view := &modelSetupPaneView{providerNames: providers, providerIndex: providerIdx}
	if m != nil {
		view.reasoningPreference = m.reasoningPreferenceValue()
	}
	view.initPicker()
	activeModel := ""
	if m != nil {
		activeModel = m.activeModel
	}
	view.setModels(modelsList, m.activeProvider, activeModel)
	view.syncReasoningForSelection(view.reasoningPreference)
	if m != nil {
		view.resize(m.layout.width, m.layout.height)
	}
	return view
}

type modelListItem struct {
	model        model.RemoteModel
	providerName string
	current      bool
}

func (i modelListItem) projection() modelsetup.Projection {
	return modelsetup.Projection{Model: i.model, ProviderName: i.providerName, Current: i.current}
}

func (i modelListItem) FilterValue() string { return modelsetup.FilterValue(i.projection()) }
func (i modelListItem) Title() string       { return modelsetup.Title(i.projection()) }
func (i modelListItem) Metadata() []string  { return modelsetup.Metadata(i.projection()) }

type modelSetupDelegate struct{}

func (modelSetupDelegate) Height() int                         { return 1 }
func (modelSetupDelegate) Spacing() int                        { return 0 }
func (modelSetupDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (modelSetupDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(modelListItem)
	if !ok {
		return
	}
	prefix := "  "
	style := bodyStyle
	if index == m.Index() {
		prefix = glyphPrompt
		style = brandStyle
	}
	width := maxInt(1, m.Width()-2)
	label := truncateWithEllipsis(entry.Title(), width)
	if entry.current {
		const marker = "(current)"
		markerWidth := len(marker)
		labelWidth := len([]rune(label))
		if gap := width - labelWidth - markerWidth; gap >= 2 {
			label += strings.Repeat(" ", gap) + mutedStyle.Render(marker)
		} else {
			label = truncateWithEllipsis(label, maxInt(1, width-markerWidth-2)) + "  " + mutedStyle.Render(marker)
		}
	}
	_, _ = fmt.Fprint(w, prefix+style.Render(label))
}

func (i modelListItem) Description() string { return modelsetup.Description(i.projection()) }

func (v *modelSetupPaneView) initPicker() {
	if v == nil || v.pickerReady {
		return
	}
	v.picker = paneutil.NewMinimalList(nil, modelSetupDelegate{}, defaultBubbleWidth-8, maxModelSetupRows)
	v.picker.SetStatusBarItemName("model", "models")
	v.picker.FilterInput.Prompt = "Search: "
	v.picker.SetShowFilter(false)
	v.picker.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "provider")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		}
	}
	v.pickerReady = true
}

func (*modelSetupPaneView) ID() string {
	return modelSetupViewID
}

func (*modelSetupPaneView) PresentationMode() panePresentationMode {
	return paneBelowComposer
}

func (v *modelSetupPaneView) resize(width, height int) {
	if v == nil {
		return
	}
	v.initPicker()
	v.layoutWidth = width
	v.layoutHeight = height
	visibleRows := len(v.picker.VisibleItems())
	if visibleRows == 0 {
		visibleRows = 1
	}
	visibleRows = minInt(maxModelSetupRows, visibleRows)
	if v.picker.SettingFilter() {
		visibleRows++
	}
	v.picker.SetSize(maxInt(12, width-8), visibleRows)
}

func (v *modelSetupPaneView) setModels(models []model.RemoteModel, activeProvider, activeModel string) {
	if v == nil {
		return
	}
	v.initPicker()
	v.allModels = append([]model.RemoteModel(nil), models...)
	providerName := v.activeProviderName()
	projection := modelsetup.Project(v.allModels, providerName, activeProvider, activeModel)
	items := make([]list.Item, 0, len(projection))
	for _, item := range projection {
		items = append(items, modelListItem{model: item.Model, providerName: item.ProviderName, current: item.Current})
	}
	filterValue := v.picker.FilterValue()
	filtering := v.picker.SettingFilter()
	_ = v.picker.SetItems(items)
	if strings.TrimSpace(filterValue) != "" {
		v.picker.SetFilterText(filterValue)
		if filtering {
			v.picker.SetFilterState(list.Filtering)
		}
	}
	if strings.EqualFold(providerName, activeProvider) {
		v.resetSelection(activeModel)
	} else {
		v.resetSelection("")
	}
	if v.layoutWidth > 0 && v.layoutHeight > 0 {
		v.resize(v.layoutWidth, v.layoutHeight)
	}
}

func (v *modelSetupPaneView) resetSelection(activeModel string) {
	if v == nil {
		return
	}
	if !v.pickerReady {
		source := v.allModels
		if len(source) == 0 {
			source = v.models
		}
		v.initPicker()
		providerName := v.activeProviderName()
		projection := modelsetup.Project(source, providerName, providerName, activeModel)
		items := make([]list.Item, 0, len(projection))
		for _, item := range projection {
			items = append(items, modelListItem{model: item.Model, providerName: item.ProviderName, current: item.Current})
		}
		_ = v.picker.SetItems(items)
	}
	v.picker.GoToStart()
	models := make([]model.RemoteModel, 0, len(v.picker.VisibleItems()))
	for _, item := range v.picker.VisibleItems() {
		if md, ok := item.(modelListItem); ok {
			models = append(models, md.model)
		}
	}
	v.picker.Select(modelpicker.ActiveModelIndex(models, activeModel))
	v.syncPickerProjection()
}

func (v *modelSetupPaneView) syncPickerProjection() {
	if v == nil {
		return
	}
	visible := v.picker.VisibleItems()
	v.models = make([]model.RemoteModel, 0, len(visible))
	for _, item := range visible {
		if md, ok := item.(modelListItem); ok {
			v.models = append(v.models, md.model)
		}
	}
}

func (v *modelSetupPaneView) activeProviderName() string {
	if v == nil {
		return model.DefaultOpenCodeName
	}
	return modelsetup.ActiveProviderName(v.providerNames, v.providerIndex)
}

func (v *modelSetupPaneView) selectedRemoteModel() (model.RemoteModel, bool) {
	if v == nil {
		return model.RemoteModel{}, false
	}
	item, ok := v.picker.SelectedItem().(modelListItem)
	if !ok {
		return model.RemoteModel{}, false
	}
	return item.model, true
}

func (v *modelSetupPaneView) syncReasoningForSelection(desired sdk.ReasoningEffort) {
	if v == nil {
		return
	}
	md, ok := v.selectedRemoteModel()
	if !ok {
		v.reasoningChoices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
		v.reasoningIndex = 0
		return
	}
	choices := modelsetup.ReasoningChoices(v.activeProviderName(), md)
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	v.reasoningChoices = choices
	v.reasoningIndex = modelsetup.ReasoningIndex(choices, desired)
}

func (v *modelSetupPaneView) selectedReasoning() sdk.ReasoningEffort {
	if v == nil {
		return sdk.ReasoningDefault
	}
	return modelsetup.SelectedReasoning(v.reasoningChoices, v.reasoningIndex)
}

func (v *modelSetupPaneView) moveReasoning(delta int) {
	if v == nil {
		return
	}
	v.reasoningIndex, v.reasoningPreference = modelsetup.MoveReasoning(v.reasoningChoices, v.reasoningIndex, delta)
}

var modelSetupKeys = struct {
	Filter, ClearFilter, ReasoningLeft, ReasoningRight         key.Binding
	Escape, Quit, Providers, AddProvider, Reload, NextProvider key.Binding
}{
	Filter:         key.NewBinding(key.WithKeys("/")),
	ClearFilter:    key.NewBinding(key.WithKeys("ctrl+u")),
	ReasoningLeft:  key.NewBinding(key.WithKeys("left", "h")),
	ReasoningRight: key.NewBinding(key.WithKeys("right", "l")),
	Escape:         key.NewBinding(key.WithKeys("esc")),
	Quit:           key.NewBinding(key.WithKeys("q")),
	Providers:      key.NewBinding(key.WithKeys("p")),
	AddProvider:    key.NewBinding(key.WithKeys("a")),
	Reload:         key.NewBinding(key.WithKeys("r")),
	NextProvider:   key.NewBinding(key.WithKeys("tab")),
}

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

	rows = appendPaneGroup(rows, v.effortRow())
	if labels := v.effortLabels(); labels != "" {
		rows = append(rows, labels)
	}
	if mode != layoutTiny {
		rows = appendPaneGroup(rows, modelSetupHelp(maxInt(1, ctx.width-6), len(v.reasoningChoices) > 1))
	}
	if showSelectionStatus {
		if status := v.selectionStatus(maxInt(1, ctx.width-6)); status != "" {
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
		return "Effort    " + brandStyle.Render(reasoningpolicy.EffortLabel(choices[0])), ""
	}

	labels := make([]string, len(choices))
	slotWidth := 5
	for i, effort := range choices {
		labels[i] = reasoningpolicy.EffortLabel(effort)
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
	return paneRightStatus(width, modelpicker.DisplayName(md)+" · "+reasoningpolicy.EffortLabel(v.selectedReasoning()))
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
	switch {
	case key.Matches(message, modelSetupKeys.Filter):
		v.picker.SetFilterState(list.Filtering)
		v.syncPickerProjection()
		v.resize(v.layoutWidth, v.layoutHeight)
		return paneKeyResult{handled: true}
	case key.Matches(message, modelSetupKeys.ClearFilter):
		v.picker.ResetFilter()
		v.syncPickerProjection()
		v.resize(v.layoutWidth, v.layoutHeight)
		v.syncReasoningForSelection(v.reasoningPreference)
		return paneKeyResult{handled: true}
	case key.Matches(message, modelSetupKeys.ReasoningLeft):
		v.moveReasoning(-1)
		return paneKeyResult{handled: true}
	case key.Matches(message, modelSetupKeys.ReasoningRight):
		v.moveReasoning(1)
		return paneKeyResult{handled: true}
	case key.Matches(message, modelSetupKeys.Escape):
		if v.picker.IsFiltered() {
			v.picker.ResetFilter()
			v.syncPickerProjection()
			v.resize(v.layoutWidth, v.layoutHeight)
			v.syncReasoningForSelection(v.reasoningPreference)
			return paneKeyResult{handled: true}
		}
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: modelSetupViewID}}
	case key.Matches(message, modelSetupKeys.Quit):
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: modelSetupViewID}}
	case key.Matches(message, modelSetupKeys.Providers):
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionOpenProviderSelect}}
	case key.Matches(message, modelSetupKeys.AddProvider):
		v.cancelFetch()
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionOpenProviderEditor}}
	case key.Matches(message, modelSetupKeys.Reload):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionReloadModels, runSlash: true}}
	case key.Matches(message, modelSetupKeys.NextProvider):
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex + 1) % len(v.providerNames)
			v.reasoningPreference = v.selectedReasoning()
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionReloadModels}}
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Nav):
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		v.syncReasoningForSelection(v.reasoningPreference)
		return paneKeyResult{handled: true, cmd: cmd}
	case message.Text >= "1" && message.Text <= "9":
		pageOffset := v.picker.Paginator.Page * v.picker.Paginator.PerPage
		targetIdx := int(message.Text[0]-'1') + pageOffset
		if targetIdx >= 0 && targetIdx < len(v.models) {
			selected := v.models[targetIdx]
			reasoning := modelsetup.CompatibleReasoning(v.activeProviderName(), selected, v.reasoningPreference)
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionApplyModelSetup, providerName: v.activeProviderName(), modelID: selected.ID, reasoning: reasoning}}
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Confirm):
		item, ok := v.picker.SelectedItem().(modelListItem)
		if !ok {
			return paneKeyResult{handled: true}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionApplyModelSetup, providerName: v.activeProviderName(), modelID: item.model.ID, reasoning: v.selectedReasoning()}}
	default:
		return paneKeyResult{}
	}
}

func persistModelSetupCmd(providers app.Providers, operationID asyncOperationID, gate *asyncOperationGate, providerName, modelID string, reasoning sdk.ReasoningEffort, unverified bool) tea.Cmd {
	return func() tea.Msg {
		if gate != nil && !gate.current(operationID) {
			return modelSetupAppliedMsg{operationID: operationID, providerName: providerName, modelID: modelID, reasoning: reasoning, unverified: unverified, err: errStaleConfigMutation}
		}
		err := providerdomain.SelectModel(providers, providerName, modelID)
		return modelSetupAppliedMsg{operationID: operationID, providerName: providerName, modelID: modelID, reasoning: reasoning, unverified: unverified, err: err}
	}
}

func (v *modelSetupPaneView) beginFetch(parent context.Context, models app.Models, providerName string, cfg config.ProviderConfig, timeouts ...time.Duration) tea.Cmd {
	discoveryTimeout := runtimepolicy.ModelDiscoveryTimeout
	if len(timeouts) > 0 && timeouts[0] > 0 {
		discoveryTimeout = timeouts[0]
	}
	v.cancelFetch()
	if parent == nil {
		v.fetchRequestID = 0
		v.loading = false
		v.err = errMissingRuntimeContext
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	v.fetchCancel = cancel
	v.fetchRequestID = nextAsyncOperationID()
	v.loading = true
	v.err = nil
	v.models = nil
	v.allModels = nil
	v.picker.GoToStart()
	return fetchProviderModelsCmd(models, providerFetchRequest{ctx: ctx, requestID: v.fetchRequestID, providerName: providerName, providerType: cfg.Type, baseURL: cfg.BaseURL, apiKey: cfg.APIKey, discoveryTimeout: discoveryTimeout})
}

func (v *modelSetupPaneView) cancelFetch() {
	if v == nil || v.fetchCancel == nil {
		return
	}
	v.fetchCancel()
	v.fetchCancel = nil
}

func (v *modelSetupPaneView) loadProvider(m *bubbleModel, force bool) tea.Cmd {
	if v == nil || m == nil {
		return nil
	}
	providerName := v.activeProviderName()
	v.cancelFetch()
	v.loading = false
	v.err = nil
	if !force {
		if models, ok := m.modelCatalogs.FreshModels(providerName, time.Now(), m.runtimeConfig.ModelCatalogTTL); ok {
			if cfg, configured := m.providers[modelcatalog.NormalizeProviderKey(providerName)]; configured {
				models = modelcatalog.VisibleForAccess(providerName, cfg.BaseURL, cfg.APIKey, models)
			}
			v.setModels(models, m.activeProvider, m.activeModel)
			v.syncReasoningForSelection(v.reasoningPreference)
			return nil
		}
	}
	cfg, configured := m.providers[modelcatalog.NormalizeProviderKey(providerName)]
	if configured {
		if model.ProviderHasUsableAuth(providerName, cfg.BaseURL, cfg.APIKey) {
			return v.beginFetch(m.ctx, m.application.Models, providerName, cfg, m.runtimeConfig.ModelDiscoveryTimeout)
		}
	}
	v.setModels(nil, m.activeProvider, m.activeModel)
	v.syncReasoningForSelection(v.reasoningPreference)
	return nil
}

func (m *bubbleModel) openModelSetupPane() tea.Cmd {
	if m == nil || m.panes.bottom.has(modelSetupViewID) {
		return nil
	}
	view := newModelSetupPaneView(m)
	m.panes.bottom.push(view)
	m.requestRelayout()
	return view.loadProvider(m, false)
}

func (m *bubbleModel) toggleModelSetupPane() tea.Cmd {
	if m.panes.bottom.has(modelSetupViewID) {
		m.panes.bottom.remove(modelSetupViewID)
		m.requestRelayout()
		return nil
	}
	return m.openModelSetupPane()
}
