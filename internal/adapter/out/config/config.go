// Package config loads layered Protonman configuration files.
package config

import (
	"time"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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
	// ProjectScope reuses a project namespace already resolved by bootstrap.
	ProjectScope *appdirs.ProjectScope
}

// ProviderConfig is the persisted provider connection schema.
type ProviderConfig = modelconfig.Provider

// ProviderSaveOptions controls how a provider update affects model defaults.
type ProviderSaveOptions struct {
	// DefaultModel is persisted as the active model when Activate is true.
	DefaultModel string
	// PreviousName removes the old provider key when a provider is renamed.
	PreviousName string
	// Activate makes the saved provider and model the active defaults.
	Activate bool
}

// ModelConfig is the persisted active model selection.
type ModelConfig = modelconfig.Selection

// SubagentModelConfig is the persisted per-profile model/reasoning override.
type SubagentModelConfig = modelconfig.SubagentRoute

// AgentConfig specifies autonomous agent execution settings.
type AgentConfig struct {
	SubagentsEnabled     bool                           `toml:"subagents_enabled"`
	Subagents            map[string]SubagentModelConfig `toml:"subagents,omitempty"`
	MaxToolCalls         int                            `toml:"max_tool_calls"`
	Profile              string                         `toml:"profile"`
	ReasoningEffort      sdk.ReasoningEffort            `toml:"reasoning_effort"`
	MaxLiveSubagents     int                            `toml:"max_live_subagents"`
	MaxRetainedSubagents int                            `toml:"max_retained_subagents"`
	SubagentMaxRuntime   time.Duration                  `toml:"-"`
	SubagentWaitTimeout  time.Duration                  `toml:"-"`
	SubagentQueueTimeout time.Duration                  `toml:"-"`
	CompletedResultTTL   time.Duration                  `toml:"-"`
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
	FieldModelDefault          = "model.default"
	FieldModelProvider         = "model.provider"
	FieldAgentProfile          = "agent.profile"
	FieldAgentSubagentsEnabled = "agent.subagents_enabled"
	FieldAgentReasoningEffort  = "agent.reasoning_effort"
	FieldAgentMaxToolCalls     = "agent.max_tool_calls"
	FieldUIPermissionMode      = "ui.permission_mode"
	FieldSkillsActive          = "skills.active"
)

// SkillsConfig specifies configured skill settings.
type SkillsConfig struct {
	Active []string `toml:"active,omitempty"`
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
	// Agent defines execution bounds and subagent policy.
	Agent AgentConfig
	// Skills defines active skills preference.
	Skills SkillsConfig
	// Runtime defines shared execution and network policies.
	Runtime RuntimeConfig
	// Sources lists files that were loaded successfully.
	Sources []string
	// Provenance records the configuration layer that last set selected fields.
	Provenance map[string]ValueSource
	// Warnings reports safe skips, such as an untrusted project config.
	Warnings []string
}
