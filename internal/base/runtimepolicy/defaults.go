package runtimepolicy

import "time"

const (
	AgentMaxRuntime                   = 30 * time.Minute
	AgentWaitTimeout                  = 30 * time.Second
	AgentQueueTimeout                 = 30 * time.Second
	AgentMaxLive                      = 16
	AgentMaxRetained                  = 64
	AgentResultTTL                    = 24 * time.Hour
	TurnMaxToolCalls                  = 100
	TurnToolResultBytesPerRound       = 4 * 1024 * 1024
	TurnToolResultBytesPerTurn        = 12 * 1024 * 1024
	TurnTimeout                       = time.Duration(0)
	RoundTimeout                      = 5 * time.Minute
	ToolPermissionTimeout             = 2 * time.Minute
	ToolExecutionTimeout              = 2 * time.Minute
	ModelRequestTimeout               = 5 * time.Minute
	ModelDiscoveryTimeout             = 10 * time.Second
	WebFetchTimeout                   = 10 * time.Second
	ModelCatalogTTL                   = 2 * time.Minute
	ReadFileMaxLineScanBytes          = 64 * 1024 * 1024
	CheckpointMaxRetained             = 128
	CheckpointMaxBytes          int64 = 512 * 1024 * 1024
	CheckpointMaxAge                  = 7 * 24 * time.Hour
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
