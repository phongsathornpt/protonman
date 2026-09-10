package model

import "strings"

type ProviderProtocol string

const (
	ProviderProtocolOpenAI    ProviderProtocol = "openai"
	ProviderProtocolAnthropic ProviderProtocol = "anthropic"

	// DefaultProtonmanName is the canonical provider label for Protonman.
	DefaultProtonmanName     = "protonman"
	DefaultProtonmanEndpoint = "https://protonman.dev/api/v1"

	// DefaultOpenCodeName is the canonical provider label for OpenCode Zen.
	DefaultOpenCodeName     = "opencode"
	DefaultOpenCodeEndpoint = "https://opencode.ai/zen/v1"
	DefaultOpenCodeModel    = "nemotron-3.5-lightning-free"

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
		EndpointHosts:  []string{"opencode.ai"},
		RequiresKey:    false,
		KeyPlaceholder: "API key (optional)…",
		Description:    "Free tier models, zero API key required",
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

// MatchProviderPreset resolves a known provider by configured name or endpoint.
func MatchProviderPreset(providerName, baseURL string) *SupportedProviderPreset {
	if preset := LookupPreset(providerName); preset != nil {
		return preset
	}
	endpoint := strings.ToLower(strings.TrimSpace(baseURL))
	if endpoint == "" {
		return nil
	}
	for i := range SupportedPresets {
		preset := &SupportedPresets[i]
		for _, host := range preset.EndpointHosts {
			if strings.Contains(endpoint, strings.ToLower(host)) {
				return preset
			}
		}
	}
	return nil
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
