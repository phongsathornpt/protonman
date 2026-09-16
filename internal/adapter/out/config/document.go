package config

type fileDocument struct {
	Permission filePermission          `json:"permission,omitempty" toml:"permission"`
	Workspace  fileWorkspace           `json:"workspace,omitempty" toml:"workspace"`
	UI         fileUI                  `json:"ui,omitempty" toml:"ui"`
	Sandbox    fileSandbox             `json:"sandbox,omitempty" toml:"sandbox"`
	Providers  map[string]fileProvider `json:"providers,omitempty" toml:"providers,omitempty"`
	Model      fileModel               `json:"model,omitempty" toml:"model,omitempty"`
	Agent      fileAgent               `json:"agent,omitempty" toml:"agent,omitempty"`
	Runtime    fileRuntime             `json:"runtime,omitempty" toml:"runtime,omitempty"`
	Skills     *fileSkills             `json:"skills,omitempty" toml:"skills,omitempty"`
}

type fileSkills struct {
	Active []string `json:"active,omitempty" toml:"active"`
}

type fileProvider struct {
	Name    string `json:"name,omitempty" toml:"name"`
	Type    string `json:"type,omitempty" toml:"type"`
	BaseURL string `json:"base_url,omitempty" toml:"base_url"`
	APIKey  string `json:"api_key,omitempty" toml:"api_key"`
}

type fileModel struct {
	Default  string `json:"default,omitempty" toml:"default"`
	Provider string `json:"provider,omitempty" toml:"provider"`
}

func fileProviderFromConfig(provider ProviderConfig) fileProvider {
	return fileProvider{Name: provider.Name, Type: provider.Type, BaseURL: provider.BaseURL, APIKey: provider.APIKey}
}

func (provider fileProvider) config() ProviderConfig {
	return ProviderConfig{Name: provider.Name, Type: provider.Type, BaseURL: provider.BaseURL, APIKey: provider.APIKey}
}

type fileAgent struct {
	SubagentsEnabled     *bool                   `json:"subagents_enabled,omitempty" toml:"subagents_enabled,omitempty"`
	Subagents            map[string]fileSubagent `json:"subagents,omitempty" toml:"subagents,omitempty"`
	MaxToolCalls         *int                    `json:"max_tool_calls,omitempty" toml:"max_tool_calls,omitempty"`
	Profile              *string                 `json:"profile,omitempty" toml:"profile,omitempty"`
	ReasoningEffort      *string                 `json:"reasoning_effort,omitempty" toml:"reasoning_effort,omitempty"`
	MaxLiveSubagents     *int                    `json:"max_live_subagents,omitempty" toml:"max_live_subagents,omitempty"`
	MaxRetainedSubagents *int                    `json:"max_retained_subagents,omitempty" toml:"max_retained_subagents,omitempty"`
	SubagentMaxRuntime   *string                 `json:"subagent_max_runtime,omitempty" toml:"subagent_max_runtime,omitempty"`
	SubagentWaitTimeout  *string                 `json:"subagent_wait_timeout,omitempty" toml:"subagent_wait_timeout,omitempty"`
	SubagentQueueTimeout *string                 `json:"subagent_queue_timeout,omitempty" toml:"subagent_queue_timeout,omitempty"`
	CompletedResultTTL   *string                 `json:"completed_result_ttl,omitempty" toml:"completed_result_ttl,omitempty"`
}

type fileSubagent struct {
	Provider        string  `json:"provider,omitempty" toml:"provider,omitempty"`
	Model           string  `json:"model,omitempty" toml:"model,omitempty"`
	ReasoningEffort *string `json:"reasoning_effort,omitempty" toml:"reasoning_effort,omitempty"`
}

type fileRuntime struct {
	TurnTimeout           *string `json:"turn_timeout,omitempty" toml:"turn_timeout,omitempty"`
	RoundTimeout          *string `json:"round_timeout,omitempty" toml:"round_timeout,omitempty"`
	ToolPermissionTimeout *string `json:"tool_permission_timeout,omitempty" toml:"tool_permission_timeout,omitempty"`
	ToolExecutionTimeout  *string `json:"tool_execution_timeout,omitempty" toml:"tool_execution_timeout,omitempty"`
	ModelRequestTimeout   *string `json:"model_request_timeout,omitempty" toml:"model_request_timeout,omitempty"`
	ModelDiscoveryTimeout *string `json:"model_discovery_timeout,omitempty" toml:"model_discovery_timeout,omitempty"`
	WebFetchTimeout       *string `json:"webFetchTimeout,omitempty" toml:"webFetchTimeout,omitempty"`
	ModelCatalogTTL       *string `json:"model_catalog_ttl,omitempty" toml:"model_catalog_ttl,omitempty"`
}

type fileSandbox struct {
	Profile string `json:"profile,omitempty" toml:"profile"`
}

type filePermission struct {
	Default string     `json:"default,omitempty" toml:"default"`
	Rules   []fileRule `json:"rules,omitempty" toml:"rules"`
}

type fileWorkspace struct {
	ProtectedPaths []string `json:"protected_paths,omitempty" toml:"protected_paths"`
}

type fileRule struct {
	Action      string `json:"action,omitempty" toml:"action"`
	Tool        string `json:"tool,omitempty" toml:"tool"`
	Pattern     string `json:"pattern,omitempty" toml:"pattern"`
	PatternMode string `json:"pattern_mode,omitempty" toml:"pattern_mode,omitempty"`
}

type fileUI struct {
	PermissionMode string `json:"permission_mode,omitempty" toml:"permission_mode"`
}
