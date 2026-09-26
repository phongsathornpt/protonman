package model

import "strings"

type ProviderProtocol string

const (
	ProviderProtocolOpenAI    ProviderProtocol = "openai"
	ProviderProtocolAnthropic ProviderProtocol = "anthropic"

	// DefaultProtonmanName is the canonical provider label for Protonman.
	DefaultProtonmanName     = "protonman"
	DefaultProtonmanEndpoint = "https://protonman.dev/api/v1"

	// DefaultOpenCodeName is the canonical provider label for the keyless
	// OpenCode Inference API. The authenticated Zen route is a separate preset.
	DefaultOpenCodeName     = "opencode"
	DefaultOpenCodeEndpoint = "https://opencode.ai/inference/openai/v1"
	DefaultOpenCodeModel    = "nemotron-3-super-free"

	// OpenCodeInferenceEndpoint is the documented keyless free chat route.
	OpenCodeInferenceEndpoint = DefaultOpenCodeEndpoint
	// OpenCodeZenEndpoint and OpenCodeGoEndpoint require an OpenCode API key
	// and expose model-specific upstream transports.
	OpenCodeZenEndpoint = "https://opencode.ai/zen/v1"
	OpenCodeGoEndpoint  = "https://opencode.ai/zen/go/v1"

	// DefaultOllamaName is the canonical provider label for local Ollama.
	DefaultOllamaName     = "ollama"
	DefaultOllamaEndpoint = "http://localhost:11434/v1"

	// DefaultOpenAIName is the canonical provider label for OpenAI.
	DefaultOpenAIName     = "openai"
	DefaultOpenAIEndpoint = "https://api.openai.com/v1"

	// DefaultAnthropicName is the canonical provider label for Anthropic.
	DefaultAnthropicName     = "anthropic"
	DefaultAnthropicEndpoint = "https://api.anthropic.com"
)

// SupportedProviderPreset describes an out-of-the-box model provider preset.
type SupportedProviderPreset struct {
	ID             string
	Name           string
	Protocol       ProviderProtocol
	BaseURL        string
	EndpointHosts  []string
	RequiresKey    bool
	KeyPlaceholder string
	Description    string
}

// SupportedPresets lists available provider presets for discovery and quick setup.
var SupportedPresets = []SupportedProviderPreset{
	{
		ID:             DefaultOpenCodeName,
		Name:           "OpenCode (Free)",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        DefaultOpenCodeEndpoint,
		EndpointHosts:  []string{"opencode.ai/inference/openai/v1"},
		RequiresKey:    false,
		KeyPlaceholder: "API key (optional)…",
		Description:    "Keyless OpenCode Inference chat models",
	},
	{
		ID:             "opencode-zen",
		Name:           "OpenCode Zen",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        OpenCodeZenEndpoint,
		EndpointHosts:  []string{"opencode.ai/zen/v1"},
		RequiresKey:    true,
		KeyPlaceholder: "OpenCode Zen API key…",
		Description:    "Authenticated OpenCode Zen catalog with model-specific transports",
	},
	{
		ID:             "opencode-go",
		Name:           "OpenCode Go",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        OpenCodeGoEndpoint,
		EndpointHosts:  []string{"opencode.ai/zen/go/v1"},
		RequiresKey:    true,
		KeyPlaceholder: "OpenCode Go API key…",
		Description:    "Authenticated OpenCode Go catalog",
	},
	{
		ID:             DefaultProtonmanName,
		Name:           "Protonman",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        DefaultProtonmanEndpoint,
		EndpointHosts:  []string{"protonman.dev"},
		RequiresKey:    true,
		KeyPlaceholder: "plk_live_…",
		Description:    "High-speed AI models gateway (plk_...)",
	},
	{
		ID:             DefaultOllamaName,
		Name:           "Ollama (Local)",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        DefaultOllamaEndpoint,
		EndpointHosts:  []string{"localhost", "127.0.0.1"},
		RequiresKey:    false,
		KeyPlaceholder: "API key (optional)…",
		Description:    "Local LLM inference, zero cloud cost",
	},
	{
		ID:             DefaultOpenAIName,
		Name:           "OpenAI Official",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        DefaultOpenAIEndpoint,
		EndpointHosts:  []string{"api.openai.com"},
		RequiresKey:    true,
		KeyPlaceholder: "sk_…",
		Description:    "Direct OpenAI API access (sk-...)",
	},
	{
		ID:             DefaultAnthropicName,
		Name:           "Anthropic",
		Protocol:       ProviderProtocolAnthropic,
		BaseURL:        DefaultAnthropicEndpoint,
		EndpointHosts:  []string{"api.anthropic.com"},
		RequiresKey:    true,
		KeyPlaceholder: "sk-ant-…",
		Description:    "Direct Anthropic Messages API access",
	},
}

// LookupPreset returns the supported provider preset by ID or name, or nil if not matched.
func LookupPreset(idOrName string) *SupportedProviderPreset {
	clean := strings.ToLower(strings.TrimSpace(idOrName))
	for i := range SupportedPresets {
		p := &SupportedPresets[i]
		if strings.EqualFold(p.ID, clean) || strings.EqualFold(p.Name, clean) {
			return p
		}
	}
	return nil
}

// MatchProviderPreset resolves a known provider by endpoint first, then by name.
// OpenCode exposes multiple routes on the same host with different auth and
// discovery contracts, so name-first matching would incorrectly treat Zen as the
// keyless free preset.
func MatchProviderPreset(providerName, baseURL string) *SupportedProviderPreset {
	endpoint := normalizeProviderEndpoint(baseURL)
	if endpoint != "" {
		for i := range SupportedPresets {
			preset := &SupportedPresets[i]
			if endpointMatchesProviderPreset(endpoint, preset) {
				return preset
			}
		}
		// Also recognize an explicitly configured OpenCode route behind a local
		// reverse proxy or test server.
		switch {
		case IsOpenCodeInferenceEndpoint(endpoint):
			return LookupPreset(DefaultOpenCodeName)
		case IsOpenCodeZenEndpoint(endpoint):
			return LookupPreset("opencode-zen")
		case IsOpenCodeGoEndpoint(endpoint):
			return LookupPreset("opencode-go")
		}
		// An explicitly configured but unknown OpenCode host/path must not
		// silently inherit the keyless preset from the provider name.
		if strings.EqualFold(strings.TrimSpace(providerName), DefaultOpenCodeName) && isOpenCodeEndpoint(endpoint) {
			return nil
		}
	}
	return LookupPreset(providerName)
}

func normalizeProviderEndpoint(baseURL string) string {
	endpoint := strings.TrimSpace(baseURL)
	if index := strings.IndexAny(endpoint, "?#"); index >= 0 {
		endpoint = endpoint[:index]
	}
	return strings.ToLower(strings.TrimRight(endpoint, "/"))
}

func endpointMatchesProviderPreset(endpoint string, preset *SupportedProviderPreset) bool {
	if preset == nil {
		return false
	}
	for _, marker := range preset.EndpointHosts {
		marker = normalizeProviderEndpoint(marker)
		if marker == "" {
			continue
		}
		endpointWithoutScheme := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
		markerWithoutScheme := strings.TrimPrefix(strings.TrimPrefix(marker, "https://"), "http://")
		if endpointWithoutScheme == markerWithoutScheme || strings.HasPrefix(endpointWithoutScheme, markerWithoutScheme+"/") {
			return true
		}
	}
	return false
}

func isOpenCodeEndpoint(endpoint string) bool {
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	return strings.HasPrefix(endpoint, "opencode.ai/")
}

// IsOpenCodeInferenceEndpoint reports whether baseURL targets the keyless
// OpenCode Inference API.
func IsOpenCodeInferenceEndpoint(baseURL string) bool {
	return openCodeEndpointPathMatch(baseURL, "/inference/openai/v1")
}

// IsOpenCodeZenEndpoint reports whether baseURL targets authenticated Zen.
func IsOpenCodeZenEndpoint(baseURL string) bool {
	return openCodeEndpointPathMatch(baseURL, "/zen/v1")
}

// IsOpenCodeGoEndpoint reports whether baseURL targets authenticated Go.
func IsOpenCodeGoEndpoint(baseURL string) bool {
	return openCodeEndpointPathMatch(baseURL, "/zen/go/v1")
}

func openCodeEndpointPathMatch(baseURL, suffix string) bool {
	endpoint := normalizeProviderEndpoint(baseURL)
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	pathIndex := strings.IndexByte(endpoint, '/')
	if pathIndex < 0 {
		return false
	}
	path := strings.TrimRight(endpoint[pathIndex:], "/")
	return path == suffix || strings.HasSuffix(path, suffix)
}

// IsProvider reports whether provider identity or endpoint maps to providerID.
func IsProvider(providerID, providerName, baseURL string) bool {
	preset := MatchProviderPreset(providerName, baseURL)
	return preset != nil && strings.EqualFold(preset.ID, providerID)
}

// ProviderHasUsableAuth reports whether a provider can be used with the supplied key.
func ProviderHasUsableAuth(providerName, baseURL, apiKey string) bool {
	if strings.TrimSpace(apiKey) != "" {
		return true
	}
	preset := MatchProviderPreset(providerName, baseURL)
	return preset != nil && !preset.RequiresKey
}

// ResolveProviderBaseURL returns the configured endpoint or the preset default
// for a known provider. Unknown providers retain the historical Protonman default.
func ResolveProviderBaseURL(providerName string, configuredURL string) string {
	return ResolveProviderBaseURLForProtocol(providerName, "", configuredURL)
}

// ResolveProviderBaseURLForProtocol resolves defaults using explicit protocol
// when a custom provider name does not match a built-in preset.
func ResolveProviderBaseURLForProtocol(providerName, providerType, configuredURL string) string {
	if baseURL := strings.TrimSpace(configuredURL); baseURL != "" {
		return strings.TrimRight(baseURL, "/")
	}
	if preset := LookupPreset(providerName); preset != nil {
		return preset.BaseURL
	}
	switch ProviderProtocol(strings.ToLower(strings.TrimSpace(providerType))) {
	case ProviderProtocolAnthropic:
		return DefaultAnthropicEndpoint
	case ProviderProtocolOpenAI:
		return DefaultOpenAIEndpoint
	default:
		return DefaultProtonmanEndpoint
	}
}
