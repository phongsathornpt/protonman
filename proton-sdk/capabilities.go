package protonsdk

// ModelCapabilities describes the effective runtime capabilities of a model instance.
// Provider models may publish protocol defaults; outer model-profile wrappers may narrow them.
type ModelCapabilities struct {
	Streaming        bool
	Tools            bool
	Vision           bool
	ProviderOptions  bool
	ToolResultErrors bool
	RawChunks        bool
}

// TokenLimits describes independently published model token constraints.
type TokenLimits struct {
	ContextWindow   int
	MaxInputTokens  int
	MaxOutputTokens int
}

// TokenLimitsModel is the legacy token-limit extension point.
// Deprecated: implement MetadataModel instead.
type TokenLimitsModel interface {
	TokenLimits() TokenLimits
}

// ContextWindowModel is the legacy context-window extension point.
// Deprecated: implement MetadataModel instead.
type ContextWindowModel interface {
	ContextWindow() int
}

// ModelTokenLimits returns all provider-neutral token limits published by a model.
func ModelTokenLimits(model LanguageModel) TokenLimits {
	return ModelMetadataOf(model).TokenLimits
}

// ModelContextWindow returns the model's authoritative context size when published.
func ModelContextWindow(model LanguageModel) int {
	return ModelTokenLimits(model).ContextWindow
}

func legacyModelTokenLimits(model LanguageModel) TokenLimits {
	if model == nil {
		return TokenLimits{}
	}
	if provider, ok := model.(TokenLimitsModel); ok {
		return normalizeTokenLimits(provider.TokenLimits())
	}
	return TokenLimits{ContextWindow: legacyContextWindow(model)}
}

func legacyContextWindow(model LanguageModel) int {
	provider, ok := model.(ContextWindowModel)
	if !ok {
		return 0
	}
	if tokens := provider.ContextWindow(); tokens > 0 {
		return tokens
	}
	return 0
}

func normalizeTokenLimits(limits TokenLimits) TokenLimits {
	if limits.ContextWindow < 0 {
		limits.ContextWindow = 0
	}
	if limits.MaxInputTokens < 0 {
		limits.MaxInputTokens = 0
	}
	if limits.MaxOutputTokens < 0 {
		limits.MaxOutputTokens = 0
	}
	return limits
}
