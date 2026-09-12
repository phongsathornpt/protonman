package model

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/modelclient"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Factory constructs concrete provider language models for the core model-client port.
type Factory struct{}

func (Factory) Build(request modelclient.Request) sdk.LanguageModel {
	providerName := strings.TrimSpace(request.ProviderName)
	if providerName == "" {
		providerName = DefaultProtonmanName
	}
	if !ProviderHasUsableAuth(providerName, request.BaseURL, request.APIKey) {
		return nil
	}
	baseURL := ResolveProviderBaseURLForProtocol(providerName, request.ProviderType, request.BaseURL)
	opts := []ClientOption{
		WithRequestTimeout(request.RequestTimeout),
		WithAgentProfile(request.AgentProfile),
		WithLowConcurrencyMode(request.LowConcurrency),
	}
	if request.RemoteModel != nil {
		opts = append(opts, WithRemoteModelProfile(providerName, *request.RemoteModel))
	}
	if strings.TrimSpace(request.SessionID) != "" {
		opts = append(opts, WithSessionID(request.SessionID))
	}
	return NewProviderLanguageModel(
		providerName,
		request.ProviderType,
		baseURL,
		request.APIKey,
		request.ModelID,
		opts...,
	)
}
