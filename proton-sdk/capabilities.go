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
