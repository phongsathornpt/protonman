package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/platform/appdirs"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
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

func (s *UserSettingsStore) SaveReasoningEffort(effort domain.ReasoningEffort) error {
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

// UserMCPIntegrationsStore persists desktop-owned MCP definitions in the
// user-global Protonman configuration without exposing the config file to an
// inbound adapter.
type UserMCPIntegrationsStore struct{ homeDir string }

const mcpIntegrationsPreferencesKey = "mcp.integrations.v1"

func NewUserMCPIntegrationsStore(homeDir string) *UserMCPIntegrationsStore {
	return &UserMCPIntegrationsStore{homeDir: homeDir}
}

func (s *UserMCPIntegrationsStore) Load(ctx context.Context) ([]app.MCPIntegration, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dirs, err := appdirs.Resolve(s.homeDir)
	if err != nil {
		return nil, err
	}
	doc, exists, err := readDocument(dirs.Config, "config file", false)
	if err != nil {
		return nil, err
	}
	if !exists || doc.Preferences == nil {
		return nil, nil
	}
	raw, ok := doc.Preferences[mcpIntegrationsPreferencesKey]
	if !ok {
		return nil, nil
	}
	var stored []fileMCPIntegration
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("decode MCP integrations preference: %w", err)
	}
	items := make([]app.MCPIntegration, 0, len(stored))
	for _, item := range stored {
		items = append(items, app.MCPIntegration{
			Name:    item.Name,
			Command: item.Command,
			Args:    append([]string(nil), item.Args...),
			Env:     append([]string(nil), item.Env...),
		})
	}
	return items, nil
}

func (s *UserMCPIntegrationsStore) Save(ctx context.Context, items []app.MCPIntegration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dirs, err := appdirs.Resolve(s.homeDir)
	if err != nil {
		return err
	}
	stored := make([]fileMCPIntegration, 0, len(items))
	for _, item := range items {
		for _, value := range item.Env {
			if containsMCPAssignment(value) {
				return errors.New("MCP environment values are not persisted")
			}
		}
		stored = append(stored, fileMCPIntegration{
			Name:    item.Name,
			Command: item.Command,
			Args:    append([]string(nil), item.Args...),
			Env:     append([]string(nil), item.Env...),
		})
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode MCP integrations preference: %w", err)
	}
	return modifyUserConfigFile(dirs.Home, false, func(doc *fileDocument) {
		if doc.Preferences == nil {
			doc.Preferences = make(map[string]json.RawMessage)
		}
		doc.Preferences[mcpIntegrationsPreferencesKey] = encoded
	})
}

func containsMCPAssignment(value string) bool {
	for _, character := range value {
		if character == '=' {
			return true
		}
	}
	return false
}

// ProjectSettingsStore persists trusted project-local preferences.
type ProjectSettingsStore struct{}

func (ProjectSettingsStore) SaveAgentProfile(workDir, profile string) error {
	return SaveProjectAgentProfile(workDir, profile)
}

func (ProjectSettingsStore) SaveSubagentsEnabled(workDir string, enabled bool) error {
	return SaveProjectSubagentsEnabled(workDir, enabled)
}

func (ProjectSettingsStore) SaveReasoningEffort(workDir string, effort domain.ReasoningEffort) error {
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
