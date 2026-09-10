package runtime

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelpicker"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const modelSetupViewID = "model_setup"

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
		modelsList, hasFreshCatalog = m.modelCatalogs.FreshModels(providers[providerIdx], time.Now(), m.runtimeConfig.ModelCatalogTTL)
	}
	if !hasFreshCatalog {
		modelsList = nil
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	view := &modelSetupPaneView{providerNames: providers, providerIndex: providerIdx}
	if m != nil {
		view.reasoningPreference = m.reasoningPreferenceValue()
	}
	view.initPicker(delegate)
	activeModel := ""
	if m != nil {
		activeModel = m.activeModel
	}
	view.setModels(modelsList, m.activeProvider, activeModel)
	view.syncReasoningForSelection(view.reasoningPreference)
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

func (v *modelSetupPaneView) initPicker(delegates ...list.DefaultDelegate) {
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

func (*modelSetupPaneView) ID() string {
	return modelSetupViewID
}

func (*modelSetupPaneView) ReplacesComposer() bool {
	return true
}

func (v *modelSetupPaneView) setModels(models []model.RemoteModel, activeProvider, activeModel string) {
	if v == nil {
		return
	}
	v.initPicker()
	v.allModels = append([]model.RemoteModel(nil), models...)
	providerIsActive := strings.EqualFold(v.activeProviderName(), activeProvider)
	items := make([]list.Item, 0, len(v.allModels))
	for _, md := range v.allModels {
		items = append(items, modelListItem{model: md, providerName: v.activeProviderName(), current: providerIsActive && strings.EqualFold(md.ID, activeModel)})
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
	if providerIsActive {
		v.resetSelection(activeModel)
	} else {
		v.resetSelection("")
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
	if v == nil || v.providerIndex < 0 || v.providerIndex >= len(v.providerNames) {
		return model.DefaultOpenCodeName
	}
	return v.providerNames[v.providerIndex]
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

func reasoningChoicesForModel(providerName string, md model.RemoteModel) []sdk.ReasoningEffort {
	profile := model.ResolveModelProfile(providerName, md.ID, &md)
	return reasoningChoices(profile)
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
	choices := reasoningChoicesForModel(v.activeProviderName(), md)
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	idx := 0
	for i, effort := range choices {
		if effort == desired {
			idx = i
			break
		}
	}
	v.reasoningChoices = choices
	v.reasoningIndex = idx
}

func (v *modelSetupPaneView) selectedReasoning() sdk.ReasoningEffort {
	if v == nil || len(v.reasoningChoices) == 0 {
		return sdk.ReasoningDefault
	}
	if v.reasoningIndex < 0 || v.reasoningIndex >= len(v.reasoningChoices) {
		return sdk.ReasoningDefault
	}
	return v.reasoningChoices[v.reasoningIndex]
}

func (v *modelSetupPaneView) moveReasoning(delta int) {
	if v == nil || len(v.reasoningChoices) == 0 {
		return
	}
	v.reasoningIndex = (v.reasoningIndex + delta + len(v.reasoningChoices)) % len(v.reasoningChoices)
	v.reasoningPreference = v.selectedReasoning()
}

func reasoningForModel(providerName string, md model.RemoteModel, desired sdk.ReasoningEffort) sdk.ReasoningEffort {
	choices := reasoningChoicesForModel(providerName, md)
	for _, effort := range choices {
		if effort == desired {
			return desired
		}
	}
	return sdk.ReasoningDefault
}
