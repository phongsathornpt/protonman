package runtime

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelpicker"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

const modelSelectViewID = "model_select"

const maxModelSelectRows = 6

type modelSelectedMsg struct {
	operationID  asyncOperationID
	providerName string
	modelID      string
	unverified   bool
	err          error
}

type modelSelectPaneView struct {
	picker         list.Model
	pickerReady    bool
	models         []model.RemoteModel
	allModels      []model.RemoteModel
	providerNames  []string
	providerIndex  int
	fetchRequestID asyncOperationID
	fetchCancel    context.CancelFunc
	loading        bool
	err            error
}

func newModelSelectPaneView(m *bubbleModel) *modelSelectPaneView {
	if m != nil {
		m.activeModelSelect = 0
		m.configMutationGate.invalidate()
	}
	providers, providerIdx := modelpicker.ProviderNames(m.providers, m.activeProvider)
	var modelsList []model.RemoteModel
	hasFreshCatalog := false
	if m != nil && providerIdx < len(providers) {
		modelsList, hasFreshCatalog = m.modelCatalogs.FreshModels(providers[providerIdx], time.Now(), m.runtimeConfig.ModelCatalogTTL)
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
	if limits := modelpicker.FormatTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens); limits != "" {
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
	v.picker.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "provider")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		}
	}
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
	filterValue := v.picker.FilterValue()
	filtering := v.picker.SettingFilter()
	_ = v.picker.SetItems(items)
	if strings.TrimSpace(filterValue) != "" {
		v.picker.SetFilterText(filterValue)
		if filtering {
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
}

func (v *modelSelectPaneView) activeProviderName() string {
	if v == nil || v.providerIndex < 0 || v.providerIndex >= len(v.providerNames) {
		return model.DefaultProtonmanName
	}
	return v.providerNames[v.providerIndex]
}
