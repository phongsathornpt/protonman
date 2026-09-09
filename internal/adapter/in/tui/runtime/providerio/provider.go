package providerio

import (
	"context"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

type FetchRequest struct {
	ProviderName string
	ProviderType string
	BaseURL      string
	APIKey       string
	Timeout      time.Duration
}

func Discover(ctx context.Context, request FetchRequest) ([]model.RemoteModel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = runtimepolicy.ModelDiscoveryTimeout
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return (app.Models{}).Discover(bounded, app.ModelDiscoveryRequest{
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

func Save(request SaveRequest) error {
	provider := config.ProviderConfig{
		Name:    request.ProviderName,
		Type:    request.ProviderType,
		BaseURL: request.BaseURL,
		APIKey:  request.APIKey,
	}
	return (app.Providers{}).Save(app.ProviderSaveRequest{
		Provider:     provider,
		DefaultModel: request.DefaultModel,
		PreviousName: request.PreviousName,
		Activate:     request.Activate,
	})
}
