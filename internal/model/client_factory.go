package model

import (
	"strings"

	"github.com/projectTHORN/proton/internal/modelprofile"
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
	builtinProfile := modelprofile.ResolveBuiltin(providerName, modelID, modelprofile.CatalogMetadata{})
	opts = append([]ClientOption{withResolvedModelProfile(builtinProfile)}, opts...)
	switch protocol {
	case ProviderProtocolAnthropic:
		return newSDKAnthropicLanguageModel(baseURL, apiKey, modelID, opts...)
	default:
		return newSDKOpenAILanguageModel(providerName, baseURL, apiKey, modelID, opts...)
	}
}
