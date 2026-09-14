package usecase

import (
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

// ModelMetadataOf returns normalized model metadata. Legacy token-limit interfaces
// remain supported while callers and adapters migrate to MetadataModel.
func ModelMetadataOf(model port.LanguageModel) domain.ModelMetadata {
	if model == nil {
		return domain.ModelMetadata{}
	}
	if provider, ok := model.(port.MetadataModel); ok {
		metadata := provider.Metadata()
		metadata.TokenLimits = domain.NormalizeTokenLimits(metadata.TokenLimits)
		return metadata
	}
	return domain.ModelMetadata{TokenLimits: legacyModelTokenLimits(model)}
}

// ModelTokenLimits returns all provider-neutral token limits published by a model.
func ModelTokenLimits(model port.LanguageModel) domain.TokenLimits {
	return ModelMetadataOf(model).TokenLimits
}

// ModelContextWindow returns the model's authoritative context size when published.
func ModelContextWindow(model port.LanguageModel) int {
	return ModelTokenLimits(model).ContextWindow
}

func legacyModelTokenLimits(model port.LanguageModel) domain.TokenLimits {
	if model == nil {
		return domain.TokenLimits{}
	}
	if provider, ok := model.(port.TokenLimitsModel); ok {
		return domain.NormalizeTokenLimits(provider.TokenLimits())
	}
	return domain.TokenLimits{ContextWindow: legacyContextWindow(model)}
}

func legacyContextWindow(model port.LanguageModel) int {
	provider, ok := model.(port.ContextWindowModel)
	if !ok {
		return 0
	}
	if tokens := provider.ContextWindow(); tokens > 0 {
		return tokens
	}
	return 0
}
