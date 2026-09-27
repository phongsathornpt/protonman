package model

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
)

// ResolvePrimaryModelDefaults applies built-in provider/model defaults.
func ResolvePrimaryModelDefaults(selection modelconfig.Selection, providers map[string]modelconfig.Provider) (modelconfig.Selection, map[string]modelconfig.Provider) {
	if providers == nil {
		providers = make(map[string]modelconfig.Provider)
	}
	providerName := strings.ToLower(strings.TrimSpace(selection.Provider))
	if providerName == "" {
		providerName = DefaultOpenCodeName
	}
	if providerName == DefaultOpenCodeName {
		if _, ok := providers[providerName]; !ok {
			providers[providerName] = modelconfig.Provider{
				Name: DefaultOpenCodeName, Type: string(ProviderProtocolOpenAI), BaseURL: DefaultOpenCodeEndpoint,
			}
		}
		if migrateLegacyAnonymousOpenCodeProvider(providers) {
			if !isOpenCodeInferenceModel(selection.Default) {
				selection.Default = ""
			}
		}
		if strings.TrimSpace(selection.Default) == "" {
			selection.Default = DefaultOpenCodeModel
		}
	}
	selection.Provider = providerName
	return selection, providers
}

func migrateLegacyAnonymousOpenCodeProvider(providers map[string]modelconfig.Provider) bool {
	provider, ok := providers[DefaultOpenCodeName]
	if !ok || strings.TrimSpace(provider.APIKey) != "" || !IsOpenCodeZenEndpoint(provider.BaseURL) {
		return false
	}
	provider.Name = DefaultOpenCodeName
	provider.Type = string(ProviderProtocolOpenAI)
	provider.BaseURL = DefaultOpenCodeEndpoint
	providers[DefaultOpenCodeName] = provider
	return true
}

func isOpenCodeInferenceModel(modelID string) bool {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	for _, candidate := range OpenCodeInferenceFreeModels() {
		if strings.EqualFold(candidate.ID, modelID) {
			return true
		}
	}
	return false
}
