package runtime

import (
	"strings"

	providerdomain "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/provider"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

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

func (v *providerPaneView) hasNameConflict(ctx paneRenderContext) bool {
	return providerdomain.HasNameConflict(ctx.providers, v.nameInput.Value(), v.originalName)
}

func (v *providerPaneView) setFetchedModels(models []model.RemoteModel) {
	v.models, v.filterFreeOnly = providerdomain.SortFetchedModels(models, v.isOpenCode())
	v.modelPickerSet = false
}

func (v *providerPaneView) currentModels() []model.RemoteModel {
	return providerdomain.CurrentModels(v.models, v.filterFreeOnly)
}
