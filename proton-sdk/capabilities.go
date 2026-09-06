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
