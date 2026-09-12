package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

var userConfigMutationMu sync.Mutex

func SaveUserProviderConfig(homeDir string, provider ProviderConfig, defaultModel string) error {
	return SaveUserProviderConfigWithOptions(homeDir, provider, ProviderSaveOptions{
		DefaultModel: defaultModel,
		Activate:     true,
	})
}

// SaveUserProviderConfigWithOptions persists a provider and optionally changes the active defaults.
func SaveUserProviderConfigWithOptions(homeDir string, provider ProviderConfig, options ProviderSaveOptions) error {
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		if doc.Providers == nil {
			doc.Providers = make(map[string]fileProvider)
		}
		previousKey := strings.ToLower(strings.TrimSpace(options.PreviousName))
		providerKey := strings.ToLower(strings.TrimSpace(provider.Name))
		if providerKey == "" {
			providerKey = "default"
		}
		if previousKey != "" && previousKey != providerKey {
			delete(doc.Providers, previousKey)
		}
		doc.Providers[providerKey] = fileProviderFromConfig(provider)

		if options.Activate {
			if options.DefaultModel != "" {
				doc.Model.Default = options.DefaultModel
			}
			doc.Model.Provider = providerKey
		}
	})
}

// SaveUserDefaultProvider updates the active provider in ~/.protonman/config.toml.
func SaveUserDefaultProvider(homeDir string, provider string) error {
	return SaveUserDefaultModel(homeDir, provider, "")
}

// SaveUserDefaultModel updates the default active model and optionally provider in ~/.protonman/config.toml.
func SaveUserDefaultModel(homeDir string, provider string, modelID string) error {
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		if modelID != "" {
			doc.Model.Default = modelID
		}
		if provider != "" {
			doc.Model.Provider = strings.ToLower(strings.TrimSpace(provider))
		}
	})
}

// SaveUserModelSelection atomically persists the exact active provider/model pair.
// Unlike SaveUserDefaultModel, an empty model intentionally clears stale model state.
func SaveUserModelSelection(homeDir string, provider string, modelID string) error {
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		doc.Model.Provider = strings.ToLower(strings.TrimSpace(provider))
		doc.Model.Default = strings.TrimSpace(modelID)
	})
}

// DeleteUserProviderConfig removes a provider configuration from ~/.protonman/config.toml.
func DeleteUserProviderConfig(homeDir string, providerName string) error {
	return modifyUserConfigFile(homeDir, true, func(doc *fileDocument) {
		providerKey := strings.ToLower(strings.TrimSpace(providerName))
		if doc.Providers != nil {
			delete(doc.Providers, providerKey)
		}

		if strings.EqualFold(doc.Model.Provider, providerKey) {
			doc.Model.Provider = ""
			doc.Model.Default = ""
		}
	})
}

// SaveUserSubagentsEnabled updates the portable subagent capability switch in ~/.protonman/config.toml.
func SaveUserSubagentsEnabled(homeDir string, enabled bool) error {
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		doc.Agent.SubagentsEnabled = &enabled
	})
}

// SaveUserReasoningEffort updates the portable agent reasoning override in ~/.protonman/config.toml.
func SaveUserReasoningEffort(homeDir string, effort sdk.ReasoningEffort) error {
	if !effort.Valid() {
		return fmt.Errorf("invalid reasoning effort %q", effort)
	}
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		value := string(effort)
		if effort == sdk.ReasoningDefault {
			value = "auto"
		}
		doc.Agent.ReasoningEffort = &value
	})
}

// SaveUserMaxToolCalls updates the optional hard tool-call ceiling override in ~/.protonman/config.toml. Zero keeps progress-aware defaults.
func SaveUserMaxToolCalls(homeDir string, maxToolCalls int) error {
	if maxToolCalls < 0 {
		return fmt.Errorf("max tool calls cannot be negative")
	}
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		doc.Agent.MaxToolCalls = &maxToolCalls
	})
}

// SaveUserPermissionRule adds a static permission rule to ~/.protonman/config.toml.
func SaveUserPermissionRule(homeDir string, rule permission.Rule) error {
	if !rule.Action.Valid() {
		return fmt.Errorf("invalid permission action: %v", rule.Action)
	}
	if !permission.ValidToolKind(rule.Tool) {
		return fmt.Errorf("invalid permission tool kind: %q", rule.Tool)
	}
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		appendRuleToDocument(doc, rule)
	})
}

func modifyUserConfigFile(homeDir string, returnIfNotExist bool, mutate func(*fileDocument)) error {
	userConfigMutationMu.Lock()
	defer userConfigMutationMu.Unlock()
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	dirs, err := appdirs.Resolve(homeDir)
	if err != nil {
		return err
	}
	userDir := dirs.Root
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	userPath := dirs.Config

	doc, exists, err := readDocument(userPath, "config file", false)
	if err != nil {
		return err
	}
	if !exists && returnIfNotExist {
		return nil
	}
	mutate(&doc)
	return writeDocumentAtomic(userDir, userPath, "config", 0o600, doc)
}
