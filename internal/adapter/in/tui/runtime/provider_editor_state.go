package runtime

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	providerdomain "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

const providerViewID = "add_provider"

type providerPaneState int

const (
	providerStateInput providerPaneState = iota
	providerStateFetching
	providerStateSelectModel
	providerStateConfirmOverwrite
	providerStateSaving
	providerStateSaveError
	providerStateError
)

type providerField = providerdomain.Field

const (
	providerFieldName     = providerdomain.FieldName
	providerFieldEndpoint = providerdomain.FieldEndpoint
	providerFieldAPIKey   = providerdomain.FieldAPIKey
	providerFieldCount    = providerdomain.FieldCount
)

const maxProviderSelectRows = 8

type providerPaneView struct {
	state          providerPaneState
	providerType   string
	focusIndex     int
	isEditing      bool
	originalName   string
	presetID       string
	requiresAPIKey bool
	nameInput      textinput.Model
	endpointInput  textinput.Model
	apiKeyInput    textinput.Model
	models         []model.RemoteModel
	selectedIndex  int
	scrollOffset   int
	filterFreeOnly bool
	errorMessage   string
	fieldErrors    [providerFieldCount]string
	fetchRequestID uint64
	fetchCancel    context.CancelFunc
	selectedModel  string
	activateOnSave bool
}

func newProviderPaneView() *providerPaneView {
	return newProviderPaneViewWithPreset("")
}

func newProviderPaneViewWithConfig(cfg config.ProviderConfig) *providerPaneView {
	pv := newProviderPaneViewWithPreset(cfg.Name)
	pv.isEditing = true
	pv.originalName = strings.TrimSpace(cfg.Name)
	if strings.TrimSpace(cfg.Type) != "" {
		pv.providerType = strings.ToLower(strings.TrimSpace(cfg.Type))
	}
	if cfg.Name != "" {
		pv.nameInput.SetValue(cfg.Name)
	}
	if cfg.BaseURL != "" {
		pv.endpointInput.SetValue(cfg.BaseURL)
	}
	if cfg.APIKey != "" {
		pv.apiKeyInput.SetValue(cfg.APIKey)
	}
	return pv
}

func newProviderPaneViewWithPreset(preset string) *providerPaneView {
	preset = strings.ToLower(strings.TrimSpace(preset))
	name := ""
	endpoint := ""
	keyPlaceholder := "API key (optional)…"
	presetID := ""
	requiresAPIKey := false
	filterFree := false
	providerType := string(model.ProviderProtocolOpenAI)
	if p := model.LookupPreset(preset); p != nil {
		presetID = p.ID
		name = p.ID
		endpoint = p.BaseURL
		requiresAPIKey = p.RequiresKey
		providerType = string(p.Protocol)
		keyPlaceholder = providerKeyPlaceholder(*p)
		if p.ID == model.DefaultOpenCodeName {
			filterFree = true
		}
	} else if preset == "opencode-free" || preset == "free" {
		presetID = model.DefaultOpenCodeName
		name = model.DefaultOpenCodeName
		endpoint = model.DefaultOpenCodeEndpoint
		keyPlaceholder = "API key (optional)…"
		filterFree = true
	} else if preset != "" {
		name = preset
	}
	nameIn := textinput.New()
	nameIn.Prompt = glyphPrompt
	nameIn.Placeholder = "provider name…"
	if name != "" {
		nameIn.SetValue(name)
	}
	nameIn.CharLimit = 64
	endpointIn := textinput.New()
	endpointIn.Prompt = glyphPrompt
	endpointIn.Placeholder = "https://api.example.com/v1"
	if endpoint != "" {
		endpointIn.SetValue(endpoint)
	}
	endpointIn.CharLimit = 256
	keyIn := textinput.New()
	keyIn.Prompt = glyphPrompt
	keyIn.Placeholder = keyPlaceholder
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.EchoCharacter = '•'
	keyIn.CharLimit = 256
	focusIndex := providerFieldName
	if name != "" && endpoint != "" {
		focusIndex = providerFieldAPIKey
	}
	pv := &providerPaneView{state: providerStateInput, focusIndex: int(focusIndex), presetID: presetID, providerType: providerType, requiresAPIKey: requiresAPIKey, nameInput: nameIn, endpointInput: endpointIn, apiKeyInput: keyIn, filterFreeOnly: filterFree, activateOnSave: true}
	pv.syncInputFocus()
	return pv
}

func (*providerPaneView) ID() string {
	return providerViewID
}

func (*providerPaneView) ReplacesComposer() bool {
	return true
}

func providerKeyPlaceholder(p model.SupportedProviderPreset) string {
	if strings.TrimSpace(p.KeyPlaceholder) != "" {
		return p.KeyPlaceholder
	}
	if !p.RequiresKey {
		return "API key (optional)…"
	}
	return "API key…"
}

func (v *providerPaneView) isOpenCode() bool {
	return model.IsProvider(model.DefaultOpenCodeName, v.nameInput.Value(), v.endpointInput.Value())
}

func (v *providerPaneView) applyPreset(preset string) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	if preset == "1" {
		preset = model.DefaultProtonmanName
	} else if preset == "2" {
		preset = model.DefaultOpenCodeName
	} else if preset == "3" {
		preset = model.DefaultOllamaName
	} else if preset == "4" {
		preset = model.DefaultOpenAIName
	} else if preset == "5" {
		preset = model.DefaultAnthropicName
	}
	if p := model.LookupPreset(preset); p != nil {
		previousName := strings.TrimSpace(v.nameInput.Value())
		v.nameInput.SetValue(p.ID)
		v.endpointInput.SetValue(p.BaseURL)
		if !strings.EqualFold(previousName, p.ID) {
			v.apiKeyInput.SetValue("")
		}
		v.apiKeyInput.Placeholder = providerKeyPlaceholder(*p)
		v.presetID = p.ID
		v.requiresAPIKey = p.RequiresKey
		v.providerType = string(p.Protocol)
		v.filterFreeOnly = (p.ID == model.DefaultOpenCodeName)
		v.clearValidation()
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
	}
}

func (v *providerPaneView) protocolLabel() string {
	label := strings.ToLower(strings.TrimSpace(v.providerType))
	if label == "" {
		label = string(model.ProviderProtocolOpenAI)
	}
	if v.presetID == "" {
		return label + " · ctrl+r to switch"
	}
	return label
}

func (v *providerPaneView) toggleProtocol() {
	if v.presetID != "" {
		return
	}
	switch model.ProviderProtocol(strings.ToLower(strings.TrimSpace(v.providerType))) {
	case model.ProviderProtocolAnthropic:
		v.providerType = string(model.ProviderProtocolOpenAI)
	default:
		v.providerType = string(model.ProviderProtocolAnthropic)
	}
}

func (v *providerPaneView) clearValidation() {
	v.fieldErrors = [providerFieldCount]string{}
	v.errorMessage = ""
}

func (v *providerPaneView) clearFieldError(field providerField) {
	if field >= 0 && field < providerFieldCount {
		v.fieldErrors[field] = ""
	}
}

func (v *providerPaneView) validateDraft() bool {
	v.clearValidation()
	validation := providerdomain.Validate(v.nameInput.Value(), v.endpointInput.Value(), v.apiKeyInput.Value(), v.requiresAPIKey)
	v.fieldErrors = validation.Errors
	if !validation.Valid {
		v.focusIndex = int(validation.FirstError)
		v.syncInputFocus()
		return false
	}
	v.nameInput.SetValue(validation.Name)
	v.endpointInput.SetValue(validation.Endpoint)
	v.apiKeyInput.SetValue(validation.APIKey)
	return true
}

func (v *providerPaneView) hasNameConflict(m *bubbleModel) bool {
	if m == nil {
		return false
	}
	return providerdomain.HasNameConflict(m.providers, v.nameInput.Value(), v.originalName)
}

func (v *providerPaneView) setFetchedModels(models []model.RemoteModel) {
	v.models, v.filterFreeOnly = providerdomain.SortFetchedModels(models, v.isOpenCode())
	v.selectedIndex = 0
	v.scrollOffset = 0
}

func (v *providerPaneView) currentModels() []model.RemoteModel {
	return providerdomain.CurrentModels(v.models, v.filterFreeOnly)
}
