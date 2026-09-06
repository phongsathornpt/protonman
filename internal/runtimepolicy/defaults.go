package runtimepolicy

import "time"

const (
	AgentMaxRuntime       = 30 * time.Minute
	AgentWaitTimeout      = 30 * time.Second
	AgentQueueTimeout     = 30 * time.Second
	AgentMaxLive          = 16
	AgentMaxRetained      = 64
	AgentResultTTL        = 10 * time.Minute
	TurnMaxRounds         = 20
	TurnMaxToolCalls      = 100
	TurnTimeout           = 10 * time.Minute
	RoundTimeout          = 5 * time.Minute
	ToolPermissionTimeout = 2 * time.Minute
	ToolExecutionTimeout  = 2 * time.Minute
	ModelRequestTimeout   = 5 * time.Minute
	ModelDiscoveryTimeout = 10 * time.Second
	WebFetchTimeout       = 10 * time.Second
	ModelCatalogTTL       = 2 * time.Minute
)

const (
	SessionPersistenceTimeout = 5 * time.Second
	AgentCloseTimeout         = 5 * time.Second
	AgentEventEmitTimeout     = 100 * time.Millisecond
	AgentLifecycleEmitTimeout = 5 * time.Second
	SandboxCommandWaitDelay   = 2 * time.Second
	TerminalEmitTimeout       = 5 * time.Second
	ProtectionObserverTimeout = time.Second
	ModelRetryBackoffStep     = 500 * time.Millisecond
)
