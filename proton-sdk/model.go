// Package protonsdk defines the provider-neutral model boundary used by Protonman agents.
package protonsdk

import (
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

// ModelMetadataOf returns normalized model metadata. Legacy token-limit interfaces
// remain supported while callers and adapters migrate to MetadataModel.
func ModelMetadataOf(model LanguageModel) ModelMetadata {
	return usecase.ModelMetadataOf(model)
}

// ModelTokenLimits returns all provider-neutral token limits published by a model.
func ModelTokenLimits(model LanguageModel) TokenLimits {
	return usecase.ModelTokenLimits(model)
}

// ModelContextWindow returns the model's authoritative context size when published.
func ModelContextWindow(model LanguageModel) int {
	return usecase.ModelContextWindow(model)
}

// NormalizeTokenLimits clamps negative token limits to zero.
func NormalizeTokenLimits(limits TokenLimits) TokenLimits {
	return domain.NormalizeTokenLimits(limits)
}
