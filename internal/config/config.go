// Package config loads layered Proton configuration files.
package config

import (
	"context"
	"errors"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
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

type ValueSource string

const (
	SourceDefault ValueSource = "default"
	SourceUser    ValueSource = "user"
	SourceProject ValueSource = "project"
)

const (
	FieldModelDefault         = "model.default"
	FieldModelProvider        = "model.provider"
	FieldAgentProfile         = "agent.profile"
	FieldAgentReasoningEffort = "agent.reasoning_effort"
	FieldAgentMaxToolCalls    = "agent.max_tool_calls"
	FieldUIPermissionMode     = "ui.permission_mode"
)

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
	// Agent defines execution bounds and subagent policy.
	Agent AgentConfig
	// Runtime defines shared execution and network policies.
	Runtime RuntimeConfig
	// Sources lists files that were loaded successfully.
	Sources []string
	// Provenance records the configuration layer that last set selected fields.
	Provenance map[string]ValueSource
	// Warnings reports safe skips, such as an untrusted project config.
	Warnings []string
}

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
			MaxToolCalls:         DefaultMaxToolCalls,
			MaxLiveSubagents:     DefaultMaxLiveSubagents,
			MaxRetainedSubagents: DefaultMaxRetainedSubagents,
			SubagentMaxRuntime:   DefaultSubagentMaxRuntime,
			SubagentWaitTimeout:  DefaultSubagentWaitTimeout,
			SubagentQueueTimeout: DefaultSubagentQueueTimeout,
			CompletedResultTTL:   DefaultCompletedResultTTL,
		},
		Runtime: DefaultRuntimeConfig(),
		Provenance: map[string]ValueSource{
			FieldModelDefault:         SourceDefault,
			FieldModelProvider:        SourceDefault,
			FieldAgentProfile:         SourceDefault,
			FieldAgentReasoningEffort: SourceDefault,
			FieldAgentMaxToolCalls:    SourceDefault,
			FieldUIPermissionMode:     SourceDefault,
		},
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
	source := SourceUser
	if project {
		source = SourceProject
	}
	if err := mergeDocument(document, snapshot, source); err != nil {
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
