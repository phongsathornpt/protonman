// Package telemetry adapts Proton's redacted application events to structured logs.
package telemetry

import (
	"context"
	"errors"
	"log/slog"

	"github.com/projectTHORN/proton/internal/application/toolcall"
)

// SlogObserver writes redacted tool-call events through a structured slog logger.
type SlogObserver struct {
	logger *slog.Logger
}

// NewSlogObserver creates an observer backed by logger.
func NewSlogObserver(logger *slog.Logger) (*SlogObserver, error) {
	if logger == nil {
		return nil, errors.New("telemetry logger is required")
	}
	return &SlogObserver{logger: logger}, nil
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
	o.logger.LogAttrs(ctx, slog.LevelInfo, "proton tool-call event", attrs...)
}

var _ toolcall.Observer = (*SlogObserver)(nil)
