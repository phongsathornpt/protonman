package app

import (
	"context"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
)

// ProviderSaveRequest describes a persisted user provider update.
type ProviderSaveRequest struct {
	Provider     modelconfig.Provider
	DefaultModel string
	PreviousName string
	Activate     bool
}

// ProviderRepository is the outbound persistence port for user provider state.
type ProviderRepository interface {
	SaveProvider(modelconfig.Provider, string, string, bool) error
	SaveModelSelection(string, string) error
	DeleteProvider(string) error
	LoadProviders(context.Context, string) (map[string]modelconfig.Provider, error)
}

// Providers owns user-provider configuration mutations for inbound adapters.
type Providers struct{ repository ProviderRepository }

func NewProviders(repository ProviderRepository) Providers {
	return Providers{repository: repository}
}

func (p Providers) Save(request ProviderSaveRequest) error {
	if p.repository == nil {
		return fmt.Errorf("provider repository is unavailable")
	}
	return p.repository.SaveProvider(request.Provider, request.DefaultModel, request.PreviousName, request.Activate)
}

func (p Providers) Select(providerName string) error {
	return p.Activate(providerName, "")
}

func (p Providers) Activate(providerName, modelID string) error {
	if p.repository == nil {
		return fmt.Errorf("provider repository is unavailable")
	}
	return p.repository.SaveModelSelection(providerName, modelID)
}

func (p Providers) SelectModel(providerName, modelID string) error {
	return p.Activate(providerName, modelID)
}

func (p Providers) Delete(providerName string) error {
	if p.repository == nil {
		return fmt.Errorf("provider repository is unavailable")
	}
	return p.repository.DeleteProvider(providerName)
}

func (p Providers) LoadConfigured(ctx context.Context, workDir string) (map[string]modelconfig.Provider, error) {
	if p.repository == nil {
		return nil, fmt.Errorf("provider repository is unavailable")
	}
	return p.repository.LoadProviders(ctx, workDir)
}
