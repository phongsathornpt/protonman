package providerio

import (
	"context"
	"time"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
)

type FetchRequest struct {
	ProviderName string
	ProviderType string
	BaseURL      string
	APIKey       string
	Timeout      time.Duration
}

func Discover(ctx context.Context, models app.Models, request FetchRequest) ([]modelcatalog.RemoteModel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = runtimepolicy.ModelDiscoveryTimeout
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return models.Discover(bounded, app.ModelDiscoveryRequest{
		ProviderName: request.ProviderName,
		ProviderType: request.ProviderType,
		BaseURL:      request.BaseURL,
		APIKey:       request.APIKey,
		Timeout:      timeout,
	})
}

type SaveRequest struct {
	ProviderName string
	ProviderType string
	PreviousName string
	BaseURL      string
	APIKey       string
	DefaultModel string
	Activate     bool
}

func Save(providers app.Providers, request SaveRequest) error {
	provider := modelconfig.Provider{
		Name: request.ProviderName, Type: request.ProviderType,
		BaseURL: request.BaseURL, APIKey: request.APIKey,
	}
	return providers.Save(app.ProviderSaveRequest{
		Provider: provider, DefaultModel: request.DefaultModel,
		PreviousName: request.PreviousName, Activate: request.Activate,
	})
}

func SelectModel(providers app.Providers, providerName, modelID string) error {
	return providers.SelectModel(providerName, modelID)
}
