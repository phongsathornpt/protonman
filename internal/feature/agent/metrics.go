package agent

import "context"

// MetricKind identifies redacted orchestration lifecycle telemetry.
type MetricKind string

const (
	MetricWaitTimeout MetricKind = "agent_wait_timeout"
	MetricCompleted   MetricKind = "agent_completed"
	MetricFailed      MetricKind = "agent_failed"
	MetricCanceled    MetricKind = "agent_canceled"
	MetricInterrupted MetricKind = "agent_interrupted"
	MetricResumed     MetricKind = "agent_resumed"
)

// MetricEvent excludes task text, model output, tool arguments, and other private data.
type MetricEvent struct {
	Kind      MetricKind
	SessionID string
	AgentID   string
	ParentID  string
	Profile   Profile
}

// MetricObserver receives redacted orchestration telemetry and must be concurrency-safe.
type MetricObserver func(context.Context, MetricEvent)

func WithMetricObserver(observer MetricObserver) Option {
	return func(c *Coordinator) { c.metricObserver = observer }
}

func (c *Coordinator) observeMetric(ctx context.Context, event MetricEvent) {
	if c == nil || c.metricObserver == nil {
		return
	}
	c.metricObserver(ctx, event)
}
