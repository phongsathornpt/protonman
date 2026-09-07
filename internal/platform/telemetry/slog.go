// Package telemetry adapts Proton's redacted application events to structured logs.
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
)

// SlogObserver writes redacted tool-call events through a structured slog logger.
type SlogObserver struct {
	logger   *slog.Logger
	mu       sync.Mutex
	counters map[string]uint64
}

// NewSlogObserver creates an observer backed by logger.
func NewSlogObserver(logger *slog.Logger) (*SlogObserver, error) {
	if logger == nil {
		return nil, errors.New("telemetry logger is required")
	}
	return &SlogObserver{logger: logger, counters: make(map[string]uint64)}, nil
}

// Observe writes one event without adding private request or result data.
func (o *SlogObserver) Observe(ctx context.Context, event toolcall.Event) {
	attrs := []slog.Attr{
		slog.String("event_kind", string(event.Kind)),
		slog.Time("event_time", event.Time),
	}
	if event.CallID != "" {
		attrs = append(attrs, slog.String("call_id", event.CallID))
	}
	if event.ToolName != "" {
		attrs = append(attrs, slog.String("tool_name", event.ToolName))
	}
	if event.ToolKind != "" {
		attrs = append(attrs, slog.String("tool_kind", string(event.ToolKind)))
	}
	if event.Mode.Valid() {
		attrs = append(attrs, slog.String("mode", event.Mode.String()))
	}
	if event.Decision.Valid() {
		attrs = append(attrs, slog.String("decision", event.Decision.String()))
	}
	if event.GrantScope.Valid() {
		attrs = append(attrs, slog.String("grant_scope", event.GrantScope.String()))
	}
	if event.ArgumentBytes > 0 {
		attrs = append(attrs, slog.Int("argument_bytes", event.ArgumentBytes))
	}
	if event.DurationMS > 0 {
		attrs = append(attrs, slog.Int64("duration_ms", event.DurationMS))
	}
	if event.ErrorCode != "" {
		attrs = append(attrs, slog.String("error_code", string(event.ErrorCode)))
	}
	if event.RecoveryAction != "" {
		attrs = append(attrs, slog.String("recovery_action", event.RecoveryAction))
	}
	switch event.Kind {
	case toolcall.EventRecoveryAttempted:
		o.increment("tool_recovery_attempt_total")
	case toolcall.EventRecoverySucceeded:
		o.increment("tool_recovery_success_total")
	case toolcall.EventRecoveryFailed:
		o.increment("tool_recovery_failure_total")
	}
	if event.Kind == toolcall.EventCallFailed && event.ErrorCode == tool.ErrorCodeStaleContinuation {
		o.increment("tool_continuation_stale_total")
	}
	o.logger.LogAttrs(ctx, slog.LevelInfo, "proton tool-call event", attrs...)
}

// ObserveProtection records redacted loop-safety telemetry and structured logs.
func (o *SlogObserver) ObserveProtection(ctx context.Context, event toolcall.ProtectionEvent) {
	metric := protectionMetric(event.Kind)
	if metric != "" {
		o.increment(metric)
	}
	attrs := []slog.Attr{
		slog.String("event_kind", string(event.Kind)),
		slog.Time("event_time", event.Time),
	}
	if event.Round > 0 {
		attrs = append(attrs, slog.Int("round", event.Round))
	}
	if event.ToolName != "" {
		attrs = append(attrs, slog.String("tool_name", event.ToolName))
	}
	if event.ToolKind != "" {
		attrs = append(attrs, slog.String("tool_kind", string(event.ToolKind)))
	}
	if event.Reason != "" {
		attrs = append(attrs, slog.String("reason", event.Reason))
	}
	if event.Fingerprint != "" {
		attrs = append(attrs, slog.String("semantic_fingerprint", event.Fingerprint))
	}
	if event.RepeatCount > 0 {
		attrs = append(attrs, slog.Int("repeat_count", event.RepeatCount))
	}
	if event.Retryable {
		attrs = append(attrs, slog.Bool("retryable", true))
	}
	if event.ErrorCode != "" {
		attrs = append(attrs, slog.String("error_code", string(event.ErrorCode)))
	}
	o.logger.LogAttrs(ctx, slog.LevelInfo, "proton tool protection event", attrs...)
}

func protectionMetric(kind toolcall.ProtectionEventKind) string {
	switch kind {
	case toolcall.ProtectionLoopDetected:
		return "tool_loop_detected_total"
	case toolcall.ProtectionCallSuppressed:
		return "tool_call_suppressed_total"
	case toolcall.ProtectionPermissionSuppressed:
		return "tool_permission_retry_suppressed_total"
	case toolcall.ProtectionRetryBudgetExhausted:
		return "tool_retry_budget_exhausted_total"
	case toolcall.ProtectionNoProgressSynthesis:
		return "tool_no_progress_synthesis_total"
	case toolcall.ProtectionTurnDeadlineExceeded:
		return "turn_deadline_exceeded_total"
	default:
		return ""
	}
}

func (o *SlogObserver) increment(name string) {
	o.mu.Lock()
	o.counters[name]++
	o.mu.Unlock()
}

// Counters returns a point-in-time copy of protection counters.
func (o *SlogObserver) Counters() map[string]uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	result := make(map[string]uint64, len(o.counters))
	for key, value := range o.counters {
		result[key] = value
	}
	return result
}

var _ toolcall.ProtectionObserver = (*SlogObserver)(nil)

var _ toolcall.Observer = (*SlogObserver)(nil)
