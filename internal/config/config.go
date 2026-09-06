// Package config loads layered Proton configuration files.
package config

import (
	"context"
	"errors"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/projectTHORN/proton/internal/appdirs"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/sandbox"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

const (
	// DefaultMaxRounds is the fallback maximum rounds per turn when unspecified.
	DefaultMaxRounds = runtimepolicy.TurnMaxRounds
	// DefaultMaxToolCalls is the fallback cumulative tool-call limit per turn.
	DefaultMaxToolCalls = runtimepolicy.TurnMaxToolCalls
	// DefaultSubagentMaxRuntime is the hard safety ceiling for one spawned subagent.
	DefaultSubagentMaxRuntime = runtimepolicy.AgentMaxRuntime
	// DefaultSubagentWaitTimeout bounds one parent wait without canceling the child.
	DefaultSubagentWaitTimeout = runtimepolicy.AgentWaitTimeout
	// DefaultSubagentQueueTimeout bounds waiting for concurrency/workspace capacity.
	DefaultSubagentQueueTimeout = runtimepolicy.AgentQueueTimeout
	// DefaultMaxLiveSubagents bounds queued and running subagents.
	DefaultMaxLiveSubagents = runtimepolicy.AgentMaxLive
	// DefaultMaxRetainedSubagents bounds terminal lifecycle records kept for later turns.
	DefaultMaxRetainedSubagents = runtimepolicy.AgentMaxRetained
	// DefaultCompletedResultTTL retains terminal results for later turns.
	DefaultCompletedResultTTL = runtimepolicy.AgentResultTTL
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

// ProviderSaveOptions controls how a provider update affects model defaults.
type ProviderSaveOptions struct {
	// DefaultModel is persisted as the active model when Activate is true.
	DefaultModel string
	// PreviousName removes the old provider key when a provider is renamed.
	PreviousName string
	// Activate makes the saved provider and model the active defaults.
	Activate bool
}

// ModelConfig specifies default model settings.
type ModelConfig struct {
	Default  string `toml:"default"`
	Provider string `toml:"provider"`
}

// AgentConfig specifies autonomous agent execution settings.
type AgentConfig struct {
	MaxRounds            int                 `toml:"max_rounds"`
	MaxToolCalls         int                 `toml:"max_tool_calls"`
	Profile              string              `toml:"profile"`
	ReasoningEffort      sdk.ReasoningEffort `toml:"reasoning_effort"`
	MaxLiveSubagents     int                 `toml:"max_live_subagents"`
	MaxRetainedSubagents int                 `toml:"max_retained_subagents"`
	SubagentMaxRuntime   time.Duration       `toml:"-"`
	SubagentWaitTimeout  time.Duration       `toml:"-"`
	SubagentQueueTimeout time.Duration       `toml:"-"`
	CompletedResultTTL   time.Duration       `toml:"-"`
}

// RuntimeConfig specifies execution and network time bounds.
type RuntimeConfig struct {
	TurnTimeout           time.Duration `toml:"-"`
	RoundTimeout          time.Duration `toml:"-"`
	ToolPermissionTimeout time.Duration `toml:"-"`
	ToolExecutionTimeout  time.Duration `toml:"-"`
	ModelRequestTimeout   time.Duration `toml:"-"`
	ModelDiscoveryTimeout time.Duration `toml:"-"`
	WebFetchTimeout       time.Duration `toml:"-"`
	ModelCatalogTTL       time.Duration `toml:"-"`
}

// DefaultRuntimeConfig returns the default shared runtime policy.
func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		TurnTimeout:           runtimepolicy.TurnTimeout,
		RoundTimeout:          runtimepolicy.RoundTimeout,
		ToolPermissionTimeout: runtimepolicy.ToolPermissionTimeout,
		ToolExecutionTimeout:  runtimepolicy.ToolExecutionTimeout,
		ModelRequestTimeout:   runtimepolicy.ModelRequestTimeout,
		ModelDiscoveryTimeout: runtimepolicy.ModelDiscoveryTimeout,
		WebFetchTimeout:       runtimepolicy.WebFetchTimeout,
		ModelCatalogTTL:       runtimepolicy.ModelCatalogTTL,
	}
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
	// Runtime defines shared execution and network policies.
	Runtime RuntimeConfig
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
	Runtime    fileRuntime               `toml:"runtime,omitempty"`
}

type fileAgent struct {
	MaxRounds            *int    `toml:"max_rounds,omitempty"`
	MaxToolCalls         *int    `toml:"max_tool_calls,omitempty"`
	Profile              *string `toml:"profile,omitempty"`
	ReasoningEffort      *string `toml:"reasoning_effort,omitempty"`
	MaxLiveSubagents     *int    `toml:"max_live_subagents,omitempty"`
	MaxRetainedSubagents *int    `toml:"max_retained_subagents,omitempty"`
	SubagentMaxRuntime   *string `toml:"subagent_max_runtime,omitempty"`
	SubagentWaitTimeout  *string `toml:"subagent_wait_timeout,omitempty"`
	SubagentQueueTimeout *string `toml:"subagent_queue_timeout,omitempty"`
	CompletedResultTTL   *string `toml:"completed_result_ttl,omitempty"`
	SubagentTimeout      *string `toml:"subagent_timeout,omitempty"` // legacy
}

type fileRuntime struct {
	TurnTimeout           *string `toml:"turn_timeout,omitempty"`
	RoundTimeout          *string `toml:"round_timeout,omitempty"`
	ToolPermissionTimeout *string `toml:"tool_permission_timeout,omitempty"`
	ToolExecutionTimeout  *string `toml:"tool_execution_timeout,omitempty"`
	ModelRequestTimeout   *string `toml:"model_request_timeout,omitempty"`
	ModelDiscoveryTimeout *string `toml:"model_discovery_timeout,omitempty"`
	WebFetchTimeout       *string `toml:"web_fetch_timeout,omitempty"`
	ModelCatalogTTL       *string `toml:"model_catalog_ttl,omitempty"`
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
			MaxRounds:            DefaultMaxRounds,
			MaxToolCalls:         DefaultMaxToolCalls,
			MaxLiveSubagents:     DefaultMaxLiveSubagents,
			MaxRetainedSubagents: DefaultMaxRetainedSubagents,
			SubagentMaxRuntime:   DefaultSubagentMaxRuntime,
			SubagentWaitTimeout:  DefaultSubagentWaitTimeout,
			SubagentQueueTimeout: DefaultSubagentQueueTimeout,
			CompletedResultTTL:   DefaultCompletedResultTTL,
		},
		Runtime:  DefaultRuntimeConfig(),
		Sources:  make([]string, 0, 2),
		Warnings: make([]string, 0),
	}

	dirs, err := appdirs.Resolve(homeDir)
	if err != nil {
		return Snapshot{}, err
	}
	userPath := dirs.Config
	if err := loadFile(ctx, userPath, &snapshot, false); err != nil {
		return Snapshot{}, err
	}

	projectPath := appdirs.ProjectConfig(workDir)
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
		if *document.Agent.MaxRounds < 0 {
			return fmt.Errorf("agent.max_rounds must be non-negative")
		}
		snapshot.Agent.MaxRounds = *document.Agent.MaxRounds
	}
	if document.Agent.MaxToolCalls != nil {
		if *document.Agent.MaxToolCalls < 0 {
			return fmt.Errorf("agent.max_tool_calls must be non-negative")
		}
		snapshot.Agent.MaxToolCalls = *document.Agent.MaxToolCalls
	}
	if document.Agent.Profile != nil {
		snapshot.Agent.Profile = strings.TrimSpace(*document.Agent.Profile)
	}
	if document.Agent.ReasoningEffort != nil {
		effort, err := sdk.ParseReasoningEffort(*document.Agent.ReasoningEffort)
		if err != nil {
			return fmt.Errorf("agent.reasoning_effort: %w", err)
		}
		snapshot.Agent.ReasoningEffort = effort
	}
	if document.Agent.MaxLiveSubagents != nil {
		if *document.Agent.MaxLiveSubagents <= 0 {
			return fmt.Errorf("agent.max_live_subagents must be positive")
		}
		snapshot.Agent.MaxLiveSubagents = *document.Agent.MaxLiveSubagents
	}
	if document.Agent.MaxRetainedSubagents != nil {
		if *document.Agent.MaxRetainedSubagents <= 0 {
			return fmt.Errorf("agent.max_retained_subagents must be positive")
		}
		snapshot.Agent.MaxRetainedSubagents = *document.Agent.MaxRetainedSubagents
	}
	if document.Agent.SubagentMaxRuntime != nil {
		d, err := parsePositiveDuration("agent.subagent_max_runtime", *document.Agent.SubagentMaxRuntime)
		if err != nil {
			return err
		}
		snapshot.Agent.SubagentMaxRuntime = d
	} else if document.Agent.SubagentTimeout != nil {
		d, err := parsePositiveDuration("agent.subagent_timeout", *document.Agent.SubagentTimeout)
		if err != nil {
			return err
		}
		snapshot.Agent.SubagentMaxRuntime = d
		snapshot.Warnings = append(snapshot.Warnings, "agent.subagent_timeout is deprecated; use agent.subagent_max_runtime")
	}
	if document.Agent.SubagentWaitTimeout != nil {
		d, err := parsePositiveDuration("agent.subagent_wait_timeout", *document.Agent.SubagentWaitTimeout)
		if err != nil {
			return err
		}
		snapshot.Agent.SubagentWaitTimeout = d
	}
	if document.Agent.SubagentQueueTimeout != nil {
		d, err := parsePositiveDuration("agent.subagent_queue_timeout", *document.Agent.SubagentQueueTimeout)
		if err != nil {
			return err
		}
		snapshot.Agent.SubagentQueueTimeout = d
	}
	if document.Agent.CompletedResultTTL != nil {
		d, err := parsePositiveDuration("agent.completed_result_ttl", *document.Agent.CompletedResultTTL)
		if err != nil {
			return err
		}
		snapshot.Agent.CompletedResultTTL = d
	}

	for field, target := range map[string]struct {
		raw *string
		set func(time.Duration)
	}{
		"runtime.turn_timeout":            {document.Runtime.TurnTimeout, func(d time.Duration) { snapshot.Runtime.TurnTimeout = d }},
		"runtime.round_timeout":           {document.Runtime.RoundTimeout, func(d time.Duration) { snapshot.Runtime.RoundTimeout = d }},
		"runtime.tool_permission_timeout": {document.Runtime.ToolPermissionTimeout, func(d time.Duration) { snapshot.Runtime.ToolPermissionTimeout = d }},
		"runtime.tool_execution_timeout":  {document.Runtime.ToolExecutionTimeout, func(d time.Duration) { snapshot.Runtime.ToolExecutionTimeout = d }},
		"runtime.model_request_timeout":   {document.Runtime.ModelRequestTimeout, func(d time.Duration) { snapshot.Runtime.ModelRequestTimeout = d }},
		"runtime.model_discovery_timeout": {document.Runtime.ModelDiscoveryTimeout, func(d time.Duration) { snapshot.Runtime.ModelDiscoveryTimeout = d }},
		"runtime.web_fetch_timeout":       {document.Runtime.WebFetchTimeout, func(d time.Duration) { snapshot.Runtime.WebFetchTimeout = d }},
		"runtime.model_catalog_ttl":       {document.Runtime.ModelCatalogTTL, func(d time.Duration) { snapshot.Runtime.ModelCatalogTTL = d }},
	} {
		if target.raw == nil {
			continue
		}
		d, err := parsePositiveDuration(field, *target.raw)
		if err != nil {
			return err
		}
		target.set(d)
	}
	return nil
}

func parsePositiveDuration(field, raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s must not be empty", field)
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", field, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", field)
	}
	return d, nil
}

// SaveUserProviderConfig persists or updates a provider configuration in ~/.proton/config.toml.
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

// SaveUserDefaultProvider updates the active provider in ~/.proton/config.toml.
func SaveUserDefaultProvider(homeDir string, provider string) error {
	return SaveUserDefaultModel(homeDir, provider, "")
}

// SaveUserDefaultModel updates the default active model and optionally provider in ~/.proton/config.toml.
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

// DeleteUserProviderConfig removes a provider configuration from ~/.proton/config.toml.
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

// SaveUserReasoningEffort updates the portable agent reasoning override in ~/.proton/config.toml.
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

// SaveUserMaxRounds updates the max rounds limit in ~/.proton/config.toml.
func SaveUserMaxRounds(homeDir string, maxRounds int) error {
	if maxRounds < 0 {
		return fmt.Errorf("max rounds cannot be negative")
	}
	return modifyUserConfigFile(homeDir, false, func(doc *fileDocument) {
		doc.Agent.MaxRounds = &maxRounds
	})
}

// SaveUserMaxToolCalls updates the cumulative tool-call limit in ~/.proton/config.toml.
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
