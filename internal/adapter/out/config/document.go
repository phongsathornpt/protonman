package config

import "encoding/json"

type fileDocument struct {
	Permission  filePermission             `json:"permission,omitempty"`
	Workspace   fileWorkspace              `json:"workspace,omitempty"`
	UI          fileUI                     `json:"ui,omitempty"`
	Sandbox     fileSandbox                `json:"sandbox,omitempty"`
	Providers   map[string]fileProvider    `json:"providers,omitempty"`
	Model       fileModel                  `json:"model,omitempty"`
	Agent       fileAgent                  `json:"agent,omitempty"`
	Runtime     fileRuntime                `json:"runtime,omitempty"`
	Skills      *fileSkills                `json:"skills,omitempty"`
	Preferences map[string]json.RawMessage `json:"preferences,omitempty"`
}

type fileSkills struct {
	Active []string `json:"active,omitempty"`
}

type fileMCPIntegration struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

type fileProvider struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
}

type fileModel struct {
	Default  string `json:"default,omitempty"`
	Provider string `json:"provider,omitempty"`
}

func fileProviderFromConfig(provider ProviderConfig) fileProvider {
	return fileProvider{Name: provider.Name, Type: provider.Type, BaseURL: provider.BaseURL, APIKey: provider.APIKey}
}

func (provider fileProvider) config() ProviderConfig {
	return ProviderConfig{Name: provider.Name, Type: provider.Type, BaseURL: provider.BaseURL, APIKey: provider.APIKey}
}

type fileAgent struct {
	SubagentsEnabled     *bool                   `json:"subagents_enabled,omitempty"`
	Subagents            map[string]fileSubagent `json:"subagents,omitempty"`
	MaxToolCalls         *int                    `json:"max_tool_calls,omitempty"`
	Profile              *string                 `json:"profile,omitempty"`
	ReasoningEffort      *string                 `json:"reasoning_effort,omitempty"`
	MaxLiveSubagents     *int                    `json:"max_live_subagents,omitempty"`
	MaxRetainedSubagents *int                    `json:"max_retained_subagents,omitempty"`
	SubagentMaxRuntime   *string                 `json:"subagent_max_runtime,omitempty"`
	SubagentWaitTimeout  *string                 `json:"subagent_wait_timeout,omitempty"`
	SubagentQueueTimeout *string                 `json:"subagent_queue_timeout,omitempty"`
	CompletedResultTTL   *string                 `json:"completed_result_ttl,omitempty"`
}

type fileSubagent struct {
	Provider        string  `json:"provider,omitempty"`
	Model           string  `json:"model,omitempty"`
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
}

type fileRuntime struct {
	TurnTimeout           *string `json:"turn_timeout,omitempty"`
	RoundTimeout          *string `json:"round_timeout,omitempty"`
	ToolPermissionTimeout *string `json:"tool_permission_timeout,omitempty"`
	ToolExecutionTimeout  *string `json:"tool_execution_timeout,omitempty"`
	ModelRequestTimeout   *string `json:"model_request_timeout,omitempty"`
	ModelDiscoveryTimeout *string `json:"model_discovery_timeout,omitempty"`
	WebFetchTimeout       *string `json:"webFetchTimeout,omitempty"`
	ModelCatalogTTL       *string `json:"model_catalog_ttl,omitempty"`
}

type fileSandbox struct {
	Profile string `json:"profile,omitempty"`
}

type filePermission struct {
	Default string     `json:"default,omitempty"`
	Rules   []fileRule `json:"rules,omitempty"`
}

type fileWorkspace struct {
	ProtectedPaths []string `json:"protected_paths,omitempty"`
}

type fileRule struct {
	Action      string `json:"action,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	PatternMode string `json:"pattern_mode,omitempty"`
}

type fileUI struct {
	PermissionMode string `json:"permission_mode,omitempty"`
}
