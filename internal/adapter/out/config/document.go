package config

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
	SubagentsEnabled     *bool                   `toml:"subagents_enabled,omitempty"`
	Subagents            map[string]fileSubagent `toml:"subagents,omitempty"`
	MaxToolCalls         *int                    `toml:"max_tool_calls,omitempty"`
	Profile              *string                 `toml:"profile,omitempty"`
	ReasoningEffort      *string                 `toml:"reasoning_effort,omitempty"`
	MaxLiveSubagents     *int                    `toml:"max_live_subagents,omitempty"`
	MaxRetainedSubagents *int                    `toml:"max_retained_subagents,omitempty"`
	SubagentMaxRuntime   *string                 `toml:"subagent_max_runtime,omitempty"`
	SubagentWaitTimeout  *string                 `toml:"subagent_wait_timeout,omitempty"`
	SubagentQueueTimeout *string                 `toml:"subagent_queue_timeout,omitempty"`
	CompletedResultTTL   *string                 `toml:"completed_result_ttl,omitempty"`
	SubagentTimeout      *string                 `toml:"subagent_timeout,omitempty"` // legacy
}

type fileSubagent struct {
	Provider        string  `toml:"provider,omitempty"`
	Model           string  `toml:"model,omitempty"`
	ReasoningEffort *string `toml:"reasoning_effort,omitempty"`
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
	PatternMode string `toml:"pattern_mode,omitempty"`
}

type fileUI struct {
	PermissionMode string `toml:"permission_mode"`
}

// Load reads user config and, when trusted, project config.
