package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

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
			doc.Providers = make(map[string]ProviderConfig)
		}
		previousKey := strings.ToLower(strings.TrimSpace(options.PreviousName))
		providerKey := strings.ToLower(strings.TrimSpace(provider.Name))
		if providerKey == "" {
			providerKey = "default"
		}
		if previousKey != "" && previousKey != providerKey {
			delete(doc.Providers, previousKey)
		}
		doc.Providers[providerKey] = provider

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

// DeleteUserProviderConfig removes a provider configuration from ~/.protonman/config.toml.
func DeleteUserProviderConfig(homeDir string, providerName string) error {
	return modifyUserConfigFile(homeDir, true, func(doc *fileDocument) {
		providerKey := strings.ToLower(strings.TrimSpace(providerName))
		if doc.Providers != nil {
			delete(doc.Providers, providerKey)
		}

		if strings.EqualFold(doc.Model.Provider, providerKey) {
			doc.Model.Provider = ""
			for remaining := range doc.Providers {
				doc.Model.Provider = remaining
				break
			}
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

// SaveUserMaxToolCalls updates the cumulative tool-call limit in ~/.protonman/config.toml.
func SaveUserMaxToolCalls(homeDir string, maxToolCalls int) error {
	if maxToolCalls < 0 {
		return fmt.Errorf("max tool calls cannot be negative")
	}
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		doc.Agent.MaxToolCalls = &maxToolCalls
	})
}

func modifyUserConfigFile(homeDir string, returnIfNotExist bool, mutate func(*fileDocument)) error {
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

	var doc fileDocument
	data, err := os.ReadFile(userPath)
	if err == nil {
		if err := toml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("decode existing config %q: %w", userPath, err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if returnIfNotExist {
			return nil
		}
	} else {
		return fmt.Errorf("read config file %q: %w", userPath, err)
	}

	mutate(&doc)

	encoded, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode config toml: %w", err)
	}

	tempFile, err := os.CreateTemp(userDir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(encoded); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("protect temporary config: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}

	if err := os.Rename(tempPath, userPath); err != nil {
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}
