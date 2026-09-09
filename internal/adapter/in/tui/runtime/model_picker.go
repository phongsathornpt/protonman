package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

const modelSelectViewID = "model_select"

const maxModelSelectRows = 6

type modelSelectedMsg struct {
	providerName string
	modelID      string
	unverified   bool
	err          error
}

type modelSelectPaneView struct {
	picker         list.Model
	pickerReady    bool
	index          int
	offset         int
	models         []model.RemoteModel
	allModels      []model.RemoteModel
	filter         string
	filtering      bool
	providerNames  []string
	providerIndex  int
	fetchRequestID uint64
	fetchCancel    context.CancelFunc
	loading        bool
	err            error
}

func newModelSelectPaneView(m *bubbleModel) *modelSelectPaneView {
	providers := make([]string, 0)
	seen := make(map[string]bool)
	if m != nil && len(m.providers) > 0 {
		for name := range m.providers {
			providers = append(providers, name)
			seen[strings.ToLower(name)] = true
		}
		sort.Strings(providers)
	}
	if m != nil && m.activeProvider != "" {
		if !seen[strings.ToLower(m.activeProvider)] {
			providers = append([]string{m.activeProvider}, providers...)
			seen[strings.ToLower(m.activeProvider)] = true
		}
	} else if len(providers) == 0 {
		providers = append(providers, model.DefaultProtonmanName)
	}
	providerIdx := 0
	if m != nil && m.activeProvider != "" {
		for i, name := range providers {
			if strings.EqualFold(name, m.activeProvider) {
				providerIdx = i
				break
			}
		}
	}
	var modelsList []model.RemoteModel
	hasFreshCatalog := false
	if m != nil && providerIdx < len(providers) {
		modelsList, hasFreshCatalog = m.modelCatalogs.freshModels(providers[providerIdx], time.Now(), m.runtimeConfig.ModelCatalogTTL)
	}
	if !hasFreshCatalog {
		modelsList = nil
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	view := &modelSelectPaneView{providerNames: providers, providerIndex: providerIdx}
	view.initPicker(delegate)
	activeModel := ""
	if m != nil {
		activeModel = m.activeModel
	}
	view.setModels(modelsList, activeModel)
	return view
}

type modelListItem struct {
	model        model.RemoteModel
	providerName string
	current      bool
}

func (i modelListItem) FilterValue() string {
	return strings.Join([]string{i.model.ID, i.model.Name, i.model.Provider, strings.Join(i.model.Features, " ")}, " ")
}

func (i modelListItem) Title() string {
	label := strings.TrimSpace(i.model.Name)
	if label == "" {
		label = i.model.ID
	}
	if model.IsFreeModel(i.model.ID) {
		label += " · FREE"
	}
	if i.current {
		label = "✓ " + label
	}
	return label
}

func (i modelListItem) Description() string {
	parts := make([]string, 0, 4)
	if name := strings.TrimSpace(i.model.Name); name != "" && !strings.EqualFold(name, i.model.ID) {
		parts = append(parts, i.model.ID)
	}
	resolved := model.ResolveRemoteMetadata(i.providerName, i.model)
	if limits := formatModelTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens); limits != "" {
		parts = append(parts, limits)
	}
	if len(resolved.Features) > 0 {
		parts = append(parts, strings.Join(resolved.Features, " · "))
	}
	if reasoning := remoteModelReasoningSummary(i.providerName, i.model, true); reasoning != "" {
		parts = append(parts, reasoning)
	}
	return strings.Join(parts, " · ")
}

func (v *modelSelectPaneView) initPicker(delegates ...list.DefaultDelegate) {
	if v == nil || v.pickerReady {
		return
	}
	delegate := list.NewDefaultDelegate()
	if len(delegates) > 0 {
		delegate = delegates[0]
	}
	delegate.SetSpacing(0)
	v.picker = list.New(nil, delegate, defaultBubbleWidth-8, defaultBubbleHeight-8)
	v.picker.DisableQuitKeybindings()
	v.picker.SetStatusBarItemName("model", "models")
	v.picker.FilterInput.Prompt = "Search: "
	v.pickerReady = true
}

func (*modelSelectPaneView) ID() string {
	return modelSelectViewID
}

func (*modelSelectPaneView) ReplacesComposer() bool {
	return true
}

func (v *modelSelectPaneView) setModels(models []model.RemoteModel, activeModel string) {
	if v == nil {
		return
	}
	v.initPicker()
	v.allModels = append([]model.RemoteModel(nil), models...)
	items := make([]list.Item, 0, len(v.allModels))
	for _, md := range v.allModels {
		items = append(items, modelListItem{model: md, providerName: v.activeProviderName(), current: strings.EqualFold(md.ID, activeModel)})
	}
	_ = v.picker.SetItems(items)
	v.applyFilter(activeModel)
}

func (v *modelSelectPaneView) applyFilter(activeModel string) {
	if v == nil {
		return
	}
	if strings.TrimSpace(v.filter) == "" {
		v.picker.ResetFilter()
	} else {
		v.picker.SetFilterText(v.filter)
		if v.filtering {
			v.picker.SetFilterState(list.Filtering)
		}
	}
	v.resetSelection(activeModel)
}

func (v *modelSelectPaneView) resetSelection(activeModel string) {
	if v == nil {
		return
	}
	if !v.pickerReady {
		source := v.allModels
		if len(source) == 0 {
			source = v.models
		}
		v.initPicker()
		items := make([]list.Item, 0, len(source))
		for _, md := range source {
			items = append(items, modelListItem{model: md, providerName: v.activeProviderName(), current: strings.EqualFold(md.ID, activeModel)})
		}
		_ = v.picker.SetItems(items)
	}
	v.picker.GoToStart()
	for i, item := range v.picker.VisibleItems() {
		md, ok := item.(modelListItem)
		if ok && strings.EqualFold(md.model.ID, activeModel) {
			v.picker.Select(i)
			break
		}
	}
	v.syncPickerProjection()
}

func (v *modelSelectPaneView) syncPickerProjection() {
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
	v.index = v.picker.Index()
	v.offset = v.picker.Paginator.Page * v.picker.Paginator.PerPage
	v.filter = v.picker.FilterValue()
	v.filtering = v.picker.SettingFilter()
}

func (v *modelSelectPaneView) activeProviderName() string {
	if v == nil || v.providerIndex < 0 || v.providerIndex >= len(v.providerNames) {
		return model.DefaultProtonmanName
	}
	return v.providerNames[v.providerIndex]
}

func (v *modelSelectPaneView) beginFetch(parent context.Context, providerName string, cfg config.ProviderConfig, timeouts ...time.Duration) tea.Cmd {
	discoveryTimeout := runtimepolicy.ModelDiscoveryTimeout
	if len(timeouts) > 0 && timeouts[0] > 0 {
		discoveryTimeout = timeouts[0]
	}
	v.cancelFetch()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	v.fetchCancel = cancel
	v.fetchRequestID++
	v.loading = true
	v.err = nil
	v.models = nil
	v.allModels = nil
	v.index = 0
	v.offset = 0
	return fetchProviderModelsCmd(providerFetchRequest{ctx: ctx, requestID: v.fetchRequestID, providerName: providerName, providerType: cfg.Type, baseURL: cfg.BaseURL, apiKey: cfg.APIKey, discoveryTimeout: discoveryTimeout})
}

func (v *modelSelectPaneView) cancelFetch() {
	if v == nil || v.fetchCancel == nil {
		return
	}
	v.fetchCancel()
	v.fetchCancel = nil
}

func (v *modelSelectPaneView) loadProvider(m *bubbleModel, force bool) tea.Cmd {
	if v == nil || m == nil {
		return nil
	}
	providerName := v.activeProviderName()
	v.cancelFetch()
	v.loading = false
	v.err = nil
	if !force {
		if models, ok := m.modelCatalogs.freshModels(providerName, time.Now(), m.runtimeConfig.ModelCatalogTTL); ok {
			v.setModels(models, m.activeModel)
			return nil
		}
	}
	cfg, configured := m.providers[normalizeProviderKey(providerName)]
	if configured {
		if model.ProviderHasUsableAuth(providerName, cfg.BaseURL, cfg.APIKey) {
			return v.beginFetch(m.ctx, providerName, cfg, m.runtimeConfig.ModelDiscoveryTimeout)
		}
	}
	v.setModels(nil, m.activeModel)
	return nil
}

func (m *bubbleModel) openModelSelectPane() tea.Cmd {
	if m == nil || m.bottom.has(modelSelectViewID) {
		return nil
	}
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	m.relayout()
	return view.loadProvider(m, false)
}

func (v *modelSelectPaneView) Render(m *bubbleModel) string {
	v.initPicker()
	if m == nil {
		return ""
	}
	providerName := v.activeProviderName()
	v.picker.Title = "Select Model · " + providerName
	if len(v.providerNames) > 1 {
		v.picker.Title += " · tab provider"
	}
	v.picker.SetSize(maxInt(12, m.width-8), maxInt(5, min(16, m.height-4)))
	mode := layoutModeForHeight(m.height)
	v.picker.SetShowStatusBar(mode == layoutNormal)
	v.picker.SetShowPagination(mode != layoutTiny)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	if v.loading {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", mutedStyle.Render("Loading models..."), "", mutedStyle.Render("esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if v.err != nil {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", errorStyle.Render("Failed to load models"), mutedStyle.Render(truncateWithEllipsis(v.err.Error(), maxInt(8, m.width-8))), "", mutedStyle.Render("r retry · p providers · esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if len(v.picker.Items()) == 0 && !v.picker.SettingFilter() && !v.picker.IsFiltered() {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", mutedStyle.Render("No models available for the selected provider."), "", mutedStyle.Render("a add provider · r retry · esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if len(v.picker.VisibleItems()) == 0 && strings.TrimSpace(v.picker.FilterValue()) != "" {
		rows := []string{brandStyle.Render("Select Model · " + providerName), mutedStyle.Render("Search: " + v.picker.FilterValue()), "", mutedStyle.Render("No models match the current search."), "", mutedStyle.Render("esc clear filter")}
		return renderModalRows(m, accentAssistant, rows)
	}
	return renderModalRows(m, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *modelSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	if key.Matches(message, m.keys.ToggleModel) {
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	}
	v.initPicker()
	if v.picker.SettingFilter() {
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return true, cmd
	}
	switch message.String() {
	case "/":
		v.picker.SetFilterState(list.Filtering)
		v.syncPickerProjection()
		return true, nil
	case "ctrl+u":
		v.picker.ResetFilter()
		v.syncPickerProjection()
		return true, nil
	case "esc":
		if v.picker.IsFiltered() {
			v.picker.ResetFilter()
			v.syncPickerProjection()
			return true, nil
		}
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	case "q":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	case "p":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerSelectViewID) {
			m.bottom.push(newProviderSelectPaneView(m))
		}
		return true, nil
	case "a":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneView())
		}
		return true, nil
	case "r":
		return true, v.loadProvider(m, true)
	case "tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex + 1) % len(v.providerNames)
			return true, v.loadProvider(m, false)
		}
		return true, nil
	case "shift+tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex - 1 + len(v.providerNames)) % len(v.providerNames)
			return true, v.loadProvider(m, false)
		}
		return true, nil
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return true, cmd
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		targetIdx := int(message.String()[0]-'1') + v.offset
		if targetIdx >= 0 && targetIdx < len(v.models) {
			selected := v.models[targetIdx]
			provName := model.DefaultProtonmanName
			if v.providerIndex >= 0 && v.providerIndex < len(v.providerNames) {
				provName = v.providerNames[v.providerIndex]
			}
			cmd := saveDefaultModelCmd(provName, selected.ID)
			m.bottom.remove(modelSelectViewID)
			return true, cmd
		}
		return true, nil
	case "enter":
		if len(v.models) == 0 {
			return true, nil
		}
		if v.index >= 0 && v.index < len(v.models) {
			selected := v.models[v.index]
			provName := model.DefaultProtonmanName
			if v.providerIndex >= 0 && v.providerIndex < len(v.providerNames) {
				provName = v.providerNames[v.providerIndex]
			}
			cmd := saveDefaultModelCmd(provName, selected.ID)
			m.bottom.remove(modelSelectViewID)
			return true, cmd
		}
		return true, nil
	default:
		return false, nil
	}
}

func formatContextTokens(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

func formatModelTokenLimits(contextWindow, maxInput, maxOutput int) string {
	parts := make([]string, 0, 3)
	if contextWindow > 0 {
		parts = append(parts, formatContextTokens(contextWindow)+" context")
	}
	if maxInput > 0 {
		parts = append(parts, formatContextTokens(maxInput)+" input")
	}
	if maxOutput > 0 {
		parts = append(parts, formatContextTokens(maxOutput)+" output")
	}
	return strings.Join(parts, " · ")
}

func saveDefaultModelCmd(providerName, modelID string) tea.Cmd {
	return saveModelSelectionCmd(providerName, modelID, false)
}

func saveModelSelectionCmd(providerName, modelID string, unverified bool) tea.Cmd {
	return func() tea.Msg {
		err := (app.Providers{}).SelectModel(providerName, modelID)
		return modelSelectedMsg{providerName: providerName, modelID: modelID, unverified: unverified, err: err}
	}
}
