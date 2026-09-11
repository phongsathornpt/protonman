package agent

import (
	"context"
	"encoding/json"
)

// MetricKind identifies redacted orchestration lifecycle and synthesis telemetry.
type MetricKind string

const (
	MetricWaitTimeout          MetricKind = "agent_wait_timeout"
	MetricCompleted            MetricKind = "agent_completed"
	MetricFailed               MetricKind = "agent_failed"
	MetricCanceled             MetricKind = "agent_canceled"
	MetricInterrupted          MetricKind = "agent_interrupted"
	MetricResumed              MetricKind = "agent_resumed"
	MetricResultBytes          MetricKind = "subagent_result_bytes"
	MetricResultConsumedBytes  MetricKind = "subagent_result_consumed_bytes"
	MetricDuplicateResultBytes MetricKind = "subagent_duplicate_result_bytes"
	MetricSynthesisAgents      MetricKind = "subagent_synthesis_agents"
	MetricWaitSnapshotBytes    MetricKind = "subagent_wait_snapshot_bytes"
)

// MetricEvent excludes task text, model output, tool arguments, and other private data.
type MetricEvent struct {
	Kind      MetricKind
	SessionID string
	AgentID   string
	ParentID  string
	Profile   Profile
	Bytes     int64
	Count     int
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

func metricJSONBytes(value any) int64 {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return int64(len(encoded))
}
