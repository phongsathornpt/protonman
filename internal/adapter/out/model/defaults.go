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
		if strings.TrimSpace(selection.Default) == "" {
			selection.Default = DefaultOpenCodeModel
		}
	}
	selection.Provider = providerName
	return selection, providers
}
