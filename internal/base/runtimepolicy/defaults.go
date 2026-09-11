package runtimepolicy

import "time"

const (
	AgentMaxRuntime                       = 30 * time.Minute
	AgentWaitTimeout                      = 30 * time.Second
	AgentQueueTimeout                     = 2 * time.Minute
	AgentMaxLive                          = 16
	AgentMaxRetained                      = 64
	AgentRetainedResultBytes              = 128 * 1024
	AgentActivityMessageBytes             = 4 * 1024
	AgentActivityErrorBytes               = 1024
	AgentResultTTL                        = 24 * time.Hour
	TurnMaxToolCalls                      = 100
	TurnToolResultBytesPerRound           = 4 * 1024 * 1024
	TurnToolResultBytesPerTurn            = 12 * 1024 * 1024
	ConversationMaxMessages               = 256
	ConversationMaxBytes                  = 16 * 1024 * 1024
	ConversationRecentMessages            = 64
	ConversationHistoricalToolBytes       = 64 * 1024
	TurnTimeout                           = time.Duration(0)
	RoundTimeout                          = 5 * time.Minute
	ToolPermissionTimeout                 = 2 * time.Minute
	ToolExecutionTimeout                  = 2 * time.Minute
	ModelRequestTimeout                   = 5 * time.Minute
	ModelDiscoveryTimeout                 = 10 * time.Second
	WebFetchTimeout                       = 10 * time.Second
	ModelCatalogTTL                       = 2 * time.Minute
	ReadFileMaxLineScanBytes              = 64 * 1024 * 1024
	CheckpointMaxRetained                 = 128
	CheckpointMaxBytes              int64 = 512 * 1024 * 1024
	CheckpointMaxAge                      = 7 * 24 * time.Hour
)

const (
	SessionPersistenceTimeout                 = 5 * time.Second
	AgentCloseTimeout                         = 5 * time.Second
	AgentEventEmitTimeout                     = 100 * time.Millisecond
	AgentLifecycleEmitTimeout                 = 5 * time.Second
	SandboxCommandWaitDelay                   = 2 * time.Second
	TerminalEmitTimeout                       = 5 * time.Second
	ProtectionObserverTimeout                 = time.Second
	ModelRetryMaxRetries                      = 4
	ModelRetryBackoffStep                     = 5 * time.Second
	ModelRetrySecondDelay                     = 15 * time.Second
	ModelRetryThirdDelay                      = 30 * time.Second
	ModelRetryLastDelay                       = 60 * time.Second
	ModelRetryPostFirstGap                    = 0 // legacy exponential-policy field
	ModelRetryMaxBackoff                      = 60 * time.Second
	ModelRetryMaxRetryAfter                   = 30 * time.Second
	OpenCodeFreeFirstEventTimeout             = 30 * time.Second
	OpenCodeFreeIdleEventTimeout              = 60 * time.Second
	OpenCodeFreeStreamMaxDuration             = 5 * time.Minute
	OpenCodeFreeLowConcurrencyInitialInterval = time.Second
	OpenCodeFreeLowConcurrencyMinInterval     = 600 * time.Millisecond
	OpenCodeFreeLowConcurrencyMaxInterval     = 5 * time.Second
	OpenCodeFreeLowConcurrencyQueueCapacity   = 48
	OpenCodeFreeLowConcurrencyMinConcurrency  = 1
	OpenCodeFreeLowConcurrencyMaxConcurrency  = 2
	OpenCodeFreeLowConcurrencyHealthySamples  = 15
	OpenCodeFreeLowConcurrencyPromoteQueue    = 4
	OpenCodeFreeLowConcurrencyRecoveryPercent = 95
	OpenCodeFreeLowConcurrencyBackoffPercent  = 175
)

// ModelRetrySchedule returns a fresh copy of the authoritative local retry
// schedule so callers cannot mutate runtime defaults through a returned slice.
func ModelRetrySchedule() []time.Duration {
	return []time.Duration{
		ModelRetryBackoffStep,
		ModelRetrySecondDelay,
		ModelRetryThirdDelay,
		ModelRetryLastDelay,
	}
}

// OpenCodeFreeLowConcurrencyPolicy is the single source of truth for OpenCode free-model
// admission and pacing. Keep product tuning here rather than in adapters/tests.
type OpenCodeFreeLowConcurrencyPolicy struct {
	InitialInterval time.Duration
	MinInterval     time.Duration
	MaxInterval     time.Duration
	QueueCapacity   int
	MinConcurrency  int
	MaxConcurrency  int
	HealthySamples  int
	PromoteQueue    int
	RecoveryPercent int
	BackoffPercent  int
}

func OpenCodeFreeLowConcurrencyMode() OpenCodeFreeLowConcurrencyPolicy {
	return OpenCodeFreeLowConcurrencyPolicy{
		InitialInterval: OpenCodeFreeLowConcurrencyInitialInterval,
		MinInterval:     OpenCodeFreeLowConcurrencyMinInterval,
		MaxInterval:     OpenCodeFreeLowConcurrencyMaxInterval,
		QueueCapacity:   OpenCodeFreeLowConcurrencyQueueCapacity,
		MinConcurrency:  OpenCodeFreeLowConcurrencyMinConcurrency,
		MaxConcurrency:  OpenCodeFreeLowConcurrencyMaxConcurrency,
		HealthySamples:  OpenCodeFreeLowConcurrencyHealthySamples,
		PromoteQueue:    OpenCodeFreeLowConcurrencyPromoteQueue,
		RecoveryPercent: OpenCodeFreeLowConcurrencyRecoveryPercent,
		BackoffPercent:  OpenCodeFreeLowConcurrencyBackoffPercent,
	}
}
