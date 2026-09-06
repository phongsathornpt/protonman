package model

import (
	"strings"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// NewProviderLanguageModel creates the proton-sdk language model for a configured provider protocol.
func NewProviderLanguageModel(
	providerName string,
	providerType string,
	baseURL string,
	apiKey string,
	modelID string,
	opts ...ClientOption,
) sdk.LanguageModel {
	protocol := ProviderProtocol(strings.ToLower(strings.TrimSpace(providerType)))
	if protocol == "" {
		if preset := MatchProviderPreset(providerName, baseURL); preset != nil {
			protocol = preset.Protocol
		}
	}
	baseURL = ResolveProviderBaseURLForProtocol(providerName, string(protocol), baseURL)
	switch protocol {
	case ProviderProtocolAnthropic:
		return newSDKAnthropicLanguageModel(baseURL, apiKey, modelID, opts...)
	default:
		return newSDKOpenAILanguageModel(providerName, baseURL, apiKey, modelID, opts...)
	}
}

// NewProviderClient keeps the legacy CLI client boundary available while subagent callers migrate to proton-sdk.
func NewProviderClient(
	providerName string,
	providerType string,
	baseURL string,
	apiKey string,
	modelID string,
	opts ...ClientOption,
) Client {
	return WrapLanguageModel(NewProviderLanguageModel(providerName, providerType, baseURL, apiKey, modelID, opts...))
}
