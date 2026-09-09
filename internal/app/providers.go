package app

import (
	"context"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

// ProviderSaveRequest describes a persisted user provider update.
type ProviderSaveRequest struct {
	Provider     config.ProviderConfig
	DefaultModel string
	PreviousName string
	Activate     bool
}

// Providers owns user-provider configuration mutations for inbound adapters.
type Providers struct{}

func (Providers) Save(request ProviderSaveRequest) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.SaveUserProviderConfigWithOptions(homeDir, request.Provider, config.ProviderSaveOptions{
		DefaultModel: request.DefaultModel,
		PreviousName: request.PreviousName,
		Activate:     request.Activate,
	})
}

func (Providers) Select(providerName string) error {
	return (Providers{}).Activate(providerName, "")
}

// Activate atomically persists the active provider and an optional reconciled model.
func (Providers) Activate(providerName, modelID string) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.SaveUserDefaultModel(homeDir, providerName, modelID)
}

func (Providers) SelectModel(providerName, modelID string) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.SaveUserDefaultModel(homeDir, providerName, modelID)
}

func (Providers) Delete(providerName string) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.DeleteUserProviderConfig(homeDir, providerName)
}

func userHomeDir() (string, error) {
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return "", err
	}
	return dirs.Home, nil
}

// LoadConfigured reloads provider configuration for a workspace using the user layer.
func (Providers) LoadConfigured(ctx context.Context, workDir string) (map[string]config.ProviderConfig, error) {
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return nil, err
	}
	snapshot, err := config.Load(ctx, config.Options{HomeDir: dirs.Home, WorkDir: workDir})
	if err != nil {
		return nil, err
	}
	return snapshot.Providers, nil
}
