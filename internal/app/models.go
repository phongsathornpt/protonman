package app

import (
	"context"
	"strings"
	"time"

	"github.com/phongsathornpt/proton/internal/adapter/out/model"
)

// ModelDiscoveryRequest describes one provider catalog lookup.
type ModelDiscoveryRequest struct {
	ProviderName string
	ProviderType string
	BaseURL      string
	APIKey       string
	Timeout      time.Duration
}

// Models owns provider model-discovery use cases for inbound adapters.
type Models struct{}

func (Models) Discover(ctx context.Context, request ModelDiscoveryRequest) ([]model.RemoteModel, error) {
	protocol := model.ProviderProtocol(strings.ToLower(strings.TrimSpace(request.ProviderType)))
	if protocol == "" {
		if preset := model.MatchProviderPreset(request.ProviderName, request.BaseURL); preset != nil {
			protocol = preset.Protocol
		}
	}
	options := []model.FetchModelsOption(nil)
	if request.Timeout > 0 {
		options = append(options, model.WithDiscoveryTimeout(request.Timeout))
	}
	return model.FetchProviderModelsForProtocol(ctx, protocol, request.BaseURL, request.APIKey, options...)
}
