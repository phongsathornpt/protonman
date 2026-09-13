package protonsdk

import "encoding/json"

// ModelMetadata groups optional provider-neutral metadata published by a model.
// New metadata should be added here instead of introducing another optional model interface.
type ModelMetadata struct {
	TokenLimits TokenLimits
}

// MetadataModel is the canonical extension point for model metadata.
type MetadataModel interface {
	Metadata() ModelMetadata
}

// ModelMetadataOf returns normalized model metadata. Legacy token-limit interfaces
// remain supported while callers and adapters migrate to MetadataModel.
func ModelMetadataOf(model LanguageModel) ModelMetadata {
	if model == nil {
		return ModelMetadata{}
	}
	if provider, ok := model.(MetadataModel); ok {
		metadata := provider.Metadata()
		metadata.TokenLimits = normalizeTokenLimits(metadata.TokenLimits)
		return metadata
	}
	return ModelMetadata{TokenLimits: legacyModelTokenLimits(model)}
}

type ProviderOptions map[string]json.RawMessage
type ProviderMetadata map[string]json.RawMessage

func cloneProviderMetadata(metadata ProviderMetadata) ProviderMetadata {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(ProviderMetadata, len(metadata))
	for key, value := range metadata {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
