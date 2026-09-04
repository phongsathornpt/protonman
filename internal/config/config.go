// Package config loads layered Proton configuration files.
package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/sandbox"
)

const (
	userConfigRelativePath    = ".proton/config.toml"
	projectConfigRelativePath = ".proton/config.toml"

	// DefaultMaxRounds is the fallback maximum rounds per turn when unspecified.
	DefaultMaxRounds = 20
)

// Options controls which configuration layers are considered.
type Options struct {
	// HomeDir is the user's home directory. An empty value resolves through
	// os.UserHomeDir.
	HomeDir string
	// WorkDir is the workspace directory. An empty value resolves through
	// os.Getwd.
	WorkDir string
	// ProjectTrusted allows project-local config to contribute rules. An
	// untrusted project file is detected and reported but never loaded.
	ProjectTrusted bool
}

// ProviderConfig specifies an AI model provider connection.
type ProviderConfig struct {
	Name    string `toml:"name"`
	Type    string `toml:"type"`
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"api_key"`
}

// ModelConfig specifies default model settings.
type ModelConfig struct {
	Default  string `toml:"default"`
	Provider string `toml:"provider"`
}

// AgentConfig specifies autonomous agent execution settings.
type AgentConfig struct {
	MaxRounds int `toml:"max_rounds"`
}

// Snapshot is the effective configuration after layered loading.
type Snapshot struct {
	// Permission is the static permission policy configuration.
	Permission permission.Config
	// Mode is the configured initial permission mode.
	Mode permission.Mode
	// ProtectedPaths are workspace-relative paths that file tools must hide or reject.
	ProtectedPaths []string
	// Sandbox is the requested OS confinement profile. Off is the default.
	Sandbox sandbox.Name
	// Providers are configured AI model providers (e.g. protonman, openai).
	Providers map[string]ProviderConfig
	// Model defines default active model preferences.
	Model ModelConfig
	// Agent defines execution bounds such as max rounds.
	Agent AgentConfig
	// Sources lists files that were loaded successfully.
	Sources []string
	// Warnings reports safe skips, such as an untrusted project config.
	Warnings []string
}

type fileDocument struct {
	Permission filePermission            `toml:"permission"`
	Workspace  fileWorkspace             `toml:"workspace"`
	UI         fileUI                    `toml:"ui"`
	Sandbox    fileSandbox               `toml:"sandbox"`
	Providers  map[string]ProviderConfig `toml:"providers,omitempty"`
	Model      ModelConfig               `toml:"model,omitempty"`
	Agent      fileAgent                 `toml:"agent,omitempty"`
}

type fileAgent struct {
	MaxRounds *int `toml:"max_rounds,omitempty"`
}

type fileSandbox struct {
	Profile string `toml:"profile"`
}

type filePermission struct {
	Default string     `toml:"default"`
	Rules   []fileRule `toml:"rules"`
}

type fileWorkspace struct {
	ProtectedPaths []string `toml:"protected_paths"`
}

type fileRule struct {
	Action      string `toml:"action"`
	Tool        string `toml:"tool"`
	Pattern     string `toml:"pattern"`
	PatternMode string `toml:"pattern_mode"`
}

type fileUI struct {
	PermissionMode string `toml:"permission_mode"`
}

// Load reads user config and, when trusted, project config.
func Load(ctx context.Context, options Options) (Snapshot, error) {
	homeDir := options.HomeDir
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return Snapshot{}, fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	workDir := options.WorkDir
	if workDir == "" {
		resolvedWorkDir, err := os.Getwd()
		if err != nil {
			return Snapshot{}, fmt.Errorf("resolve work directory: %w", err)
		}
		workDir = resolvedWorkDir
	}

	if err := validateDirectory(homeDir, "home"); err != nil {
		return Snapshot{}, err
	}
	if err := validateDirectory(workDir, "work"); err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		Permission: permission.Config{
			Rules:   make([]permission.Rule, 0),
			Default: permission.ActionAsk,
		},
		Mode:           permission.ModeAsk,
		ProtectedPaths: make([]string, 0),
		Sandbox:        sandbox.NameOff,
		Providers:      make(map[string]ProviderConfig),
		Agent: AgentConfig{
			MaxRounds: DefaultMaxRounds,
		},
		Sources:        make([]string, 0, 2),
		Warnings:       make([]string, 0),
	}

	userPath := filepath.Join(homeDir, userConfigRelativePath)
	if err := loadFile(ctx, userPath, &snapshot, false); err != nil {
		return Snapshot{}, err
	}

	projectPath := filepath.Join(workDir, projectConfigRelativePath)
	if !options.ProjectTrusted {
		exists, err := fileExists(projectPath)
		if err != nil {
			return Snapshot{}, fmt.Errorf("inspect project config: %w", err)
		}
		if exists {
			snapshot.Warnings = append(snapshot.Warnings,
				"ignored untrusted project config: "+projectPath,
			)
		}
		return snapshot, nil
	}
	if err := loadFile(ctx, projectPath, &snapshot, true); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func loadFile(ctx context.Context, path string, snapshot *Snapshot, project bool) (loadErr error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before loading %s: %w", path, err)
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open config %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && loadErr == nil {
			loadErr = fmt.Errorf("close config %s: %w", path, closeErr)
		}
	}()

	var document fileDocument
	if err := toml.NewDecoder(file).Decode(&document); err != nil {
		return fmt.Errorf("decode config %s: %w", path, err)
	}
	if err := mergeDocument(document, snapshot); err != nil {
		return fmt.Errorf("apply config %s: %w", path, err)
	}
	snapshot.Sources = append(snapshot.Sources, path)
	if project && document.UI.PermissionMode != "" {
		snapshot.Warnings = append(snapshot.Warnings,
			"project permission mode is active because project config is trusted: "+path,
		)
	}
	return nil
}

func mergeDocument(document fileDocument, snapshot *Snapshot) error {
	if document.Permission.Default != "" {
		defaultAction, err := permission.ParseAction(document.Permission.Default)
		if err != nil {
			return fmt.Errorf("permission.default: %w", err)
		}
		snapshot.Permission.Default = defaultAction
	}
	for index, rawRule := range document.Permission.Rules {
		rule, err := decodeRule(rawRule)
		if err != nil {
			return fmt.Errorf("permission.rules[%d]: %w", index, err)
		}
		snapshot.Permission.Rules = append(snapshot.Permission.Rules, rule)
	}
	for _, protectedPath := range document.Workspace.ProtectedPaths {
		protectedPath = strings.TrimSpace(protectedPath)
		if protectedPath == "" {
			return fmt.Errorf("workspace.protected_paths contains an empty path")
		}
		snapshot.ProtectedPaths = append(snapshot.ProtectedPaths, protectedPath)
	}
	if document.UI.PermissionMode != "" {
		mode, err := permission.ParseMode(document.UI.PermissionMode)
		if err != nil {
			return fmt.Errorf("ui.permission_mode: %w", err)
		}
		snapshot.Mode = mode
	}
	if document.Sandbox.Profile != "" {
		name, err := sandbox.ParseName(document.Sandbox.Profile)
		if err != nil {
			return fmt.Errorf("sandbox.profile: %w", err)
		}
		snapshot.Sandbox = name
	}
	if len(document.Providers) > 0 {
		if snapshot.Providers == nil {
			snapshot.Providers = make(map[string]ProviderConfig)
		}
		maps.Copy(snapshot.Providers, document.Providers)
	}
	if document.Model.Default != "" {
		snapshot.Model.Default = document.Model.Default
	}
	if document.Model.Provider != "" {
		snapshot.Model.Provider = document.Model.Provider
	}
	if document.Agent.MaxRounds != nil {
		snapshot.Agent.MaxRounds = *document.Agent.MaxRounds
	}
	return nil
}

// SaveUserProviderConfig persists or updates a provider configuration in ~/.proton/config.toml.
func SaveUserProviderConfig(homeDir string, provider ProviderConfig, defaultModel string) error {
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	userDir := filepath.Join(homeDir, ".proton")
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	userPath := filepath.Join(homeDir, userConfigRelativePath)

	var doc fileDocument
	data, err := os.ReadFile(userPath)
	if err == nil {
		_ = toml.Unmarshal(data, &doc)
	}

	if doc.Providers == nil {
		doc.Providers = make(map[string]ProviderConfig)
	}
	providerKey := strings.ToLower(strings.TrimSpace(provider.Name))
	if providerKey == "" {
		providerKey = "default"
	}
	doc.Providers[providerKey] = provider

	if defaultModel != "" {
		doc.Model.Default = defaultModel
		doc.Model.Provider = providerKey
	}

	encoded, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode config toml: %w", err)
	}

	tempPath := fmt.Sprintf("%s.tmp.%d", userPath, os.Getpid())
	if err := os.WriteFile(tempPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := os.Rename(tempPath, userPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}

// SaveUserDefaultProvider updates the active provider in ~/.proton/config.toml.
func SaveUserDefaultProvider(homeDir string, provider string) error {
	return SaveUserDefaultModel(homeDir, provider, "")
}

// SaveUserDefaultModel updates the default active model and optionally provider in ~/.proton/config.toml.
func SaveUserDefaultModel(homeDir string, provider string, modelID string) error {
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	userDir := filepath.Join(homeDir, ".proton")
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	userPath := filepath.Join(homeDir, userConfigRelativePath)

	var doc fileDocument
	data, err := os.ReadFile(userPath)
	if err == nil {
		_ = toml.Unmarshal(data, &doc)
	}

	if modelID != "" {
		doc.Model.Default = modelID
	}
	if provider != "" {
		doc.Model.Provider = strings.ToLower(strings.TrimSpace(provider))
	}

	encoded, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode config toml: %w", err)
	}

	tempPath := fmt.Sprintf("%s.tmp.%d", userPath, os.Getpid())
	if err := os.WriteFile(tempPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := os.Rename(tempPath, userPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}

// DeleteUserProviderConfig removes a provider configuration from ~/.proton/config.toml.
func DeleteUserProviderConfig(homeDir string, providerName string) error {
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	userPath := filepath.Join(homeDir, userConfigRelativePath)

	var doc fileDocument
	data, err := os.ReadFile(userPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config file: %w", err)
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

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

	encoded, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode config toml: %w", err)
	}

	tempPath := fmt.Sprintf("%s.tmp.%d", userPath, os.Getpid())
	if err := os.WriteFile(tempPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := os.Rename(tempPath, userPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}

// SaveUserMaxRounds updates the max rounds limit in ~/.proton/config.toml.
func SaveUserMaxRounds(homeDir string, maxRounds int) error {
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	userDir := filepath.Join(homeDir, ".proton")
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	userPath := filepath.Join(homeDir, userConfigRelativePath)

	var doc fileDocument
	data, err := os.ReadFile(userPath)
	if err == nil {
		_ = toml.Unmarshal(data, &doc)
	}

	doc.Agent.MaxRounds = &maxRounds

	encoded, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode config toml: %w", err)
	}

	tempPath := fmt.Sprintf("%s.tmp.%d", userPath, os.Getpid())
	if err := os.WriteFile(tempPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := os.Rename(tempPath, userPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}


func decodeRule(raw fileRule) (permission.Rule, error) {
	// Grok defaults omitted rule actions to deny. Keeping that default avoids
	// turning a partially written rule into an accidental allow.
	action := permission.ActionDeny
	if strings.TrimSpace(raw.Action) != "" {
		parsedAction, err := permission.ParseAction(raw.Action)
		if err != nil {
			return permission.Rule{}, fmt.Errorf("action: %w", err)
		}
		action = parsedAction
	}
	toolKind, err := permission.ParseToolKind(raw.Tool)
	if err != nil {
		return permission.Rule{}, fmt.Errorf("tool: %w", err)
	}
	patternMode, err := permission.ParsePatternMode(raw.PatternMode)
	if err != nil {
		return permission.Rule{}, fmt.Errorf("pattern_mode: %w", err)
	}
	return permission.Rule{
		Action:      action,
		Tool:        toolKind,
		Pattern:     raw.Pattern,
		PatternMode: patternMode,
	}, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func validateDirectory(path string, label string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s directory is required", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s directory %q: %w", label, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s path %q is not a directory", label, path)
	}
	return nil
}
