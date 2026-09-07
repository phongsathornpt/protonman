package protonsdk

// ModelCapabilities describes provider behavior that agent runtimes may rely on.
// It reports adapter support, not a promise that every model ID exposes every feature.
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

// TokenLimitsModel optionally exposes richer token constraints than a total context window.
type TokenLimitsModel interface {
	TokenLimits() TokenLimits
}

// ModelTokenLimits returns all token limits published by a model. Legacy
// ContextWindowModel implementations are promoted into ContextWindow.
func ModelTokenLimits(model LanguageModel) TokenLimits {
	if model == nil {
		return TokenLimits{}
	}
	if provider, ok := model.(TokenLimitsModel); ok {
		limits := provider.TokenLimits()
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
	return TokenLimits{ContextWindow: ModelContextWindow(model)}
}

// ContextWindowModel optionally exposes the model's authoritative context size.
type ContextWindowModel interface {
	ContextWindow() int
}

// ModelContextWindow returns an optional context-window size without widening
// the core LanguageModel interface for providers that do not publish metadata.
func ModelContextWindow(model LanguageModel) int {
	if model == nil {
		return 0
	}
	provider, ok := model.(ContextWindowModel)
	if !ok {
		return 0
	}
	if tokens := provider.ContextWindow(); tokens > 0 {
		return tokens
	}
	return 0
}
