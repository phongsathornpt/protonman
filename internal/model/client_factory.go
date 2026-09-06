package model

import "strings"

// NewProviderClient creates the model client for a configured provider protocol.
func NewProviderClient(
	providerName string,
	providerType string,
	baseURL string,
	apiKey string,
	modelID string,
	opts ...OpenAIOption,
) Client {
	protocol := ProviderProtocol(strings.ToLower(strings.TrimSpace(providerType)))
	if protocol == "" {
		if preset := MatchProviderPreset(providerName, baseURL); preset != nil {
			protocol = preset.Protocol
		}
	}
	switch protocol {
	case ProviderProtocolAnthropic:
		return newSDKAnthropicClient(baseURL, apiKey, modelID, opts...)
	default:
		return newSDKOpenAIClient(providerName, baseURL, apiKey, modelID, opts...)
	}
}
