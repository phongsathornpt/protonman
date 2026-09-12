package model

import (
	"context"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/modelcatalog"
)

// Catalog adapts provider HTTP discovery to the core discovery port.
type Catalog struct{}

func (Catalog) Discover(ctx context.Context, request modelcatalog.DiscoveryRequest) ([]modelcatalog.RemoteModel, error) {
	protocol := ProviderProtocol(strings.ToLower(strings.TrimSpace(request.ProviderType)))
	if protocol == "" {
		if preset := MatchProviderPreset(request.ProviderName, request.BaseURL); preset != nil {
			protocol = preset.Protocol
		}
	}
	options := []FetchModelsOption(nil)
	if request.Timeout > 0 {
		options = append(options, WithDiscoveryTimeout(request.Timeout))
	}
	return FetchProviderModelsForProtocol(ctx, protocol, request.BaseURL, request.APIKey, options...)
}
