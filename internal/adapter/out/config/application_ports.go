package config

import (
	"context"

	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// UserProviderRepository persists provider/model selection in user config.
type UserProviderRepository struct{ homeDir string }

func NewUserProviderRepository(homeDir string) *UserProviderRepository {
	return &UserProviderRepository{homeDir: homeDir}
}

func (r *UserProviderRepository) SaveProvider(provider modelconfig.Provider, defaultModel, previousName string, activate bool) error {
	return SaveUserProviderConfigWithOptions(r.homeDir, provider, ProviderSaveOptions{
		DefaultModel: defaultModel,
		PreviousName: previousName,
		Activate:     activate,
	})
}

func (r *UserProviderRepository) SaveModelSelection(provider, modelID string) error {
	return SaveUserModelSelection(r.homeDir, provider, modelID)
}

func (r *UserProviderRepository) DeleteProvider(providerName string) error {
	return DeleteUserProviderConfig(r.homeDir, providerName)
}

func (r *UserProviderRepository) LoadProviders(ctx context.Context, workDir string) (map[string]modelconfig.Provider, error) {
	snapshot, err := Load(ctx, Options{HomeDir: r.homeDir, WorkDir: workDir})
	if err != nil {
		return nil, err
	}
	return snapshot.Providers, nil
}

// UserSettingsStore persists portable user preferences.
type UserSettingsStore struct{ homeDir string }

func NewUserSettingsStore(homeDir string) *UserSettingsStore {
	return &UserSettingsStore{homeDir: homeDir}
}

func (s *UserSettingsStore) SaveSubagentsEnabled(enabled bool) error {
	return SaveUserSubagentsEnabled(s.homeDir, enabled)
}

func (s *UserSettingsStore) SaveReasoningEffort(effort sdk.ReasoningEffort) error {
	return SaveUserReasoningEffort(s.homeDir, effort)
}

func (s *UserSettingsStore) SaveMaxToolCalls(maxToolCalls int) error {
	return SaveUserMaxToolCalls(s.homeDir, maxToolCalls)
}

func (s *UserSettingsStore) SavePermissionRule(rule permission.Rule) error {
	return SaveUserPermissionRule(s.homeDir, rule)
}

func (s *UserSettingsStore) SaveActiveSkills(activeSkills []string) error {
	return SaveUserActiveSkills(s.homeDir, activeSkills)
}

// ProjectSettingsStore persists trusted project-local preferences.
type ProjectSettingsStore struct{}

func (ProjectSettingsStore) SaveAgentProfile(workDir, profile string) error {
	return SaveProjectAgentProfile(workDir, profile)
}

func (ProjectSettingsStore) SaveSubagentsEnabled(workDir string, enabled bool) error {
	return SaveProjectSubagentsEnabled(workDir, enabled)
}

func (ProjectSettingsStore) SaveReasoningEffort(workDir string, effort sdk.ReasoningEffort) error {
	return SaveProjectReasoningEffort(workDir, effort)
}

func (ProjectSettingsStore) SaveMaxToolCalls(workDir string, maxToolCalls int) error {
	return SaveProjectMaxToolCalls(workDir, maxToolCalls)
}

func (ProjectSettingsStore) SavePermissionMode(workDir string, mode permission.Mode) error {
	return SaveProjectPermissionMode(workDir, mode)
}

func (ProjectSettingsStore) SavePermissionRule(workDir string, rule permission.Rule) error {
	return SaveProjectPermissionRule(workDir, rule)
}

func (ProjectSettingsStore) SaveActiveSkills(workDir string, activeSkills []string) error {
	return SaveProjectActiveSkills(workDir, activeSkills)
}
