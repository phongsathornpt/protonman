package runtime

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionbridge"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type testProviderRepository struct{ fallbackHome string }

func newPermissionBridge() *permissionbridge.Bridge {
	return permissionbridge.New()
}

func (r testProviderRepository) home() string {
	if home := strings.TrimSpace(os.Getenv("PROTONMAN_HOME")); home != "" {
		return home
	}
	return r.fallbackHome
}
func (r testProviderRepository) SaveProvider(provider modelconfig.Provider, defaultModel, previousName string, activate bool) error {
	return config.SaveUserProviderConfigWithOptions(r.home(), provider, config.ProviderSaveOptions{
		DefaultModel: defaultModel, PreviousName: previousName, Activate: activate,
	})
}

func (r testProviderRepository) SaveModelSelection(provider, modelID string) error {
	return config.SaveUserModelSelection(r.home(), provider, modelID)
}

func (r testProviderRepository) DeleteProvider(providerName string) error {
	return config.DeleteUserProviderConfig(r.home(), providerName)
}

func (r testProviderRepository) LoadProviders(ctx context.Context, workDir string) (map[string]modelconfig.Provider, error) {
	snapshot, err := config.Load(ctx, config.Options{HomeDir: r.home(), WorkDir: workDir})
	if err != nil {
		return nil, err
	}
	return snapshot.Providers, nil
}

type testUserSettingsStore struct{ fallbackHome string }

func (s testUserSettingsStore) home() string {
	if home := strings.TrimSpace(os.Getenv("PROTONMAN_HOME")); home != "" {
		return home
	}
	return s.fallbackHome
}

func (s testUserSettingsStore) SaveSubagentsEnabled(enabled bool) error {
	return config.SaveUserSubagentsEnabled(s.home(), enabled)
}
func (s testUserSettingsStore) SaveReasoningEffort(effort sdk.ReasoningEffort) error {
	return config.SaveUserReasoningEffort(s.home(), effort)
}
func (s testUserSettingsStore) SaveMaxToolCalls(maxToolCalls int) error {
	return config.SaveUserMaxToolCalls(s.home(), maxToolCalls)
}
func (s testUserSettingsStore) SavePermissionRule(rule permission.Rule) error {
	return config.SaveUserPermissionRule(s.home(), rule)
}

func attachTestApplication(t testing.TB, m *bubbleModel) {
	t.Helper()
	home := t.TempDir()
	m.application = app.Services{
		Models:       app.NewModels(model.Catalog{}),
		Providers:    app.NewProviders(testProviderRepository{fallbackHome: home}),
		Projects:     app.NewProjects(config.ProjectSettingsStore{}),
		UserSettings: app.NewUserSettings(testUserSettingsStore{fallbackHome: home}),
		ModelFactory: model.Factory{},
	}
}
