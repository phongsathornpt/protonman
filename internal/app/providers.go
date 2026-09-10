package app

import (
	"context"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"strings"
)

// ResolvePrimaryModelDefaults guarantees a usable built-in default when the
// persisted primary model selection is incomplete. Explicit selections win.
func ResolvePrimaryModelDefaults(selection config.ModelConfig, providers map[string]config.ProviderConfig) (config.ModelConfig, map[string]config.ProviderConfig) {
	if providers == nil {
		providers = make(map[string]config.ProviderConfig)
	}
	providerName := strings.ToLower(strings.TrimSpace(selection.Provider))
	if providerName == "" {
		providerName = model.DefaultOpenCodeName
	}
	if providerName == model.DefaultOpenCodeName {
		if _, ok := providers[providerName]; !ok {
			providers[providerName] = config.ProviderConfig{
				Name: model.DefaultOpenCodeName, Type: string(model.ProviderProtocolOpenAI), BaseURL: model.DefaultOpenCodeEndpoint,
			}
		}
		if strings.TrimSpace(selection.Default) == "" {
			selection.Default = model.DefaultOpenCodeModel
		}
	}
	selection.Provider = providerName
	return selection, providers
}

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
	return config.SaveUserModelSelection(homeDir, providerName, modelID)
}

func (Providers) SelectModel(providerName, modelID string) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.SaveUserModelSelection(homeDir, providerName, modelID)
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
