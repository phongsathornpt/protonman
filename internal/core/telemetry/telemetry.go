// Package telemetry defines the core-owned redacted execution-event contract.
//
// The tool-call service emits these events; platform telemetry implements the
// observer. Owning the types here keeps the dependency pointing inward:
// engine/toolcall and platform/telemetry both depend on this core package,
// never on each other.
package telemetry

import (
	"context"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
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
	// EventRecoveryAttempted marks one bounded host-side deterministic recovery attempt.
	EventRecoveryAttempted EventKind = "tool_recovery_attempted"
	// EventRecoverySucceeded marks a recovery that returned a successful tool result.
	EventRecoverySucceeded EventKind = "tool_recovery_succeeded"
	// EventRecoveryFailed marks a recovery attempt that still failed.
	EventRecoveryFailed EventKind = "tool_recovery_failed"
)

// ProtectionEventKind identifies one redacted loop-safety event.
type ProtectionEventKind string

const (
	ProtectionLoopDetected          ProtectionEventKind = "tool_loop_detected"
	ProtectionCallSuppressed        ProtectionEventKind = "tool_call_suppressed"
	ProtectionPermissionSuppressed  ProtectionEventKind = "tool_permission_retry_suppressed"
	ProtectionRetryBudgetExhausted  ProtectionEventKind = "tool_retry_budget_exhausted"
	ProtectionNoProgressSynthesis   ProtectionEventKind = "tool_no_progress_synthesis"
	ProtectionSafetyBudgetExhausted ProtectionEventKind = "tool_safety_budget_exhausted"
	ProtectionTurnDeadlineExceeded  ProtectionEventKind = "turn_deadline_exceeded"
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
	Kind           EventKind             `json:"kind"`
	Time           time.Time             `json:"time"`
	CallID         string                `json:"call_id,omitempty"`
	ToolName       string                `json:"tool_name,omitempty"`
	ToolKind       permission.ToolKind   `json:"tool_kind,omitempty"`
	Mode           permission.Mode       `json:"mode,omitempty"`
	Decision       permission.Action     `json:"decision,omitempty"`
	GrantScope     permission.GrantScope `json:"grant_scope,omitempty"`
	ArgumentBytes  int                   `json:"argument_bytes,omitempty"`
	DurationMS     int64                 `json:"duration_ms,omitempty"`
	ErrorCode      tool.ErrorCode        `json:"error_code,omitempty"`
	RecoveryAction tool.RecoveryAction   `json:"recovery_action,omitempty"`
}

// Observer receives redacted events and must be safe for concurrent calls.
type Observer interface {
	Observe(context.Context, Event)
}
