package app

import (
	"context"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/core/modelcatalog"
)

// ModelDiscoveryRequest is the application alias for a provider catalog lookup.
type ModelDiscoveryRequest = modelcatalog.DiscoveryRequest

// ModelDiscovery is the outbound provider-catalog port.
type ModelDiscovery = modelcatalog.Discovery

// Models owns provider model-discovery use cases for inbound adapters.
type Models struct{ discovery ModelDiscovery }

func NewModels(discovery ModelDiscovery) Models { return Models{discovery: discovery} }

func (m Models) Discover(ctx context.Context, request ModelDiscoveryRequest) ([]modelcatalog.RemoteModel, error) {
	if m.discovery == nil {
		return nil, fmt.Errorf("model discovery is unavailable")
	}
	return m.discovery.Discover(ctx, request)
}
