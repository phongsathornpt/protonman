package toolcall

import (
	"context"
	"time"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

// EventKind identifies one redacted tool-call lifecycle event.
type EventKind string

const (
	// EventCallStarted marks a validated or attempted tool call entering the service.
	EventCallStarted EventKind = "tool_call_started"
	// EventPermissionResolved records the decision without its private detail or reason.
	EventPermissionResolved EventKind = "permission_resolved"
	// EventCallCompleted marks a tool handler that returned successfully.
	EventCallCompleted EventKind = "tool_call_completed"
	// EventCallFailed marks a call that was denied, canceled, or failed in a handler.
	EventCallFailed EventKind = "tool_call_failed"
)

// ProtectionEventKind identifies one redacted loop-safety event.
type ProtectionEventKind string

const (
	ProtectionLoopDetected         ProtectionEventKind = "tool_loop_detected"
	ProtectionCallSuppressed       ProtectionEventKind = "tool_call_suppressed"
	ProtectionPermissionSuppressed ProtectionEventKind = "tool_permission_retry_suppressed"
	ProtectionRetryBudgetExhausted ProtectionEventKind = "tool_retry_budget_exhausted"
	ProtectionNoProgressSynthesis  ProtectionEventKind = "tool_no_progress_synthesis"
	ProtectionTurnDeadlineExceeded ProtectionEventKind = "turn_deadline_exceeded"
)

// ProtectionEvent contains only redacted metadata. Fingerprint is a short hash
// of canonical call semantics and never contains raw arguments.
type ProtectionEvent struct {
	Kind        ProtectionEventKind
	Time        time.Time
	Round       int
	ToolName    string
	ToolKind    permission.ToolKind
	Reason      string
	Fingerprint string
	RepeatCount int
	Retryable   bool
	ErrorCode   tool.ErrorCode
}

// ProtectionObserver optionally extends a tool-call observer with loop safety events.
type ProtectionObserver interface {
	ObserveProtection(context.Context, ProtectionEvent)
}

// Event is the redacted metadata emitted around a tool call.
//
// Event deliberately excludes permission.Request.Detail, permission.Request.Arguments,
// handler output, and error messages. Observers can use ArgumentBytes to understand
// request size without receiving commands, paths, URLs, or other sensitive values.
type Event struct {
	Kind          EventKind             `json:"kind"`
	Time          time.Time             `json:"time"`
	CallID        string                `json:"call_id,omitempty"`
	ToolName      string                `json:"tool_name,omitempty"`
	ToolKind      permission.ToolKind   `json:"tool_kind,omitempty"`
	Mode          permission.Mode       `json:"mode,omitempty"`
	Decision      permission.Action     `json:"decision,omitempty"`
	GrantScope    permission.GrantScope `json:"grant_scope,omitempty"`
	ArgumentBytes int                   `json:"argument_bytes,omitempty"`
	DurationMS    int64                 `json:"duration_ms,omitempty"`
	ErrorCode     tool.ErrorCode        `json:"error_code,omitempty"`
}

// Observer receives redacted events and must be safe for concurrent calls.
type Observer interface {
	Observe(context.Context, Event)
}

// ObserveProtection forwards one redacted loop-safety event when the configured
// observer supports protection telemetry.
func (s *Service) ObserveProtection(ctx context.Context, event ProtectionEvent) {
	if s == nil || s.observer == nil {
		return
	}
	observer, ok := s.observer.(ProtectionObserver)
	if !ok {
		return
	}
	observer.ObserveProtection(ctx, event)
}

type callTelemetry struct {
	started  time.Time
	call     tool.Call
	toolKind permission.ToolKind
}

func (s *Service) observe(ctx context.Context, event Event) {
	if s.observer == nil {
		return
	}
	s.observer.Observe(ctx, event)
}

func (s *Service) observeCallStarted(ctx context.Context, telemetry callTelemetry) {
	s.observe(ctx, Event{
		Kind:          EventCallStarted,
		Time:          telemetry.started,
		CallID:        telemetry.call.ID,
		ToolName:      telemetry.call.Name,
		ToolKind:      telemetry.toolKind,
		ArgumentBytes: len(telemetry.call.Arguments),
	})
}

func (s *Service) observePermission(
	ctx context.Context,
	telemetry callTelemetry,
	resolution permission.Resolution,
) {
	s.observe(ctx, Event{
		Kind:          EventPermissionResolved,
		Time:          time.Now(),
		CallID:        telemetry.call.ID,
		ToolName:      telemetry.call.Name,
		ToolKind:      telemetry.toolKind,
		Mode:          s.Mode(),
		Decision:      resolution.Action,
		GrantScope:    resolution.Scope,
		ArgumentBytes: len(telemetry.call.Arguments),
	})
}

func (s *Service) observeCallResult(
	ctx context.Context,
	telemetry callTelemetry,
	result tool.Result,
	err error,
) {
	kind := EventCallCompleted
	if err != nil {
		kind = EventCallFailed
	}
	event := Event{
		Kind:          kind,
		Time:          time.Now(),
		CallID:        telemetry.call.ID,
		ToolName:      telemetry.call.Name,
		ToolKind:      telemetry.toolKind,
		ArgumentBytes: len(telemetry.call.Arguments),
		DurationMS:    time.Since(telemetry.started).Milliseconds(),
	}
	if result.Failure != nil {
		event.ErrorCode = result.Failure.Code
	}
	s.observe(ctx, event)
}
