// Package config loads layered Proton configuration files.
package config

import (
	"time"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/base/runtimepolicy"
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
