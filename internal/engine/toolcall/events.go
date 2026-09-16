package toolcall

import (
	"context"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	coretelemetry "github.com/phongsathornpt/protonman/internal/core/telemetry"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// Core-owned redacted execution-event contract. The canonical definitions live
// in internal/core/telemetry; these aliases keep existing call sites compiling
// while platform/telemetry depends on core instead of this engine package.
type (
	EventKind           = coretelemetry.EventKind
	ProtectionEventKind = coretelemetry.ProtectionEventKind
	ProtectionEvent     = coretelemetry.ProtectionEvent
	Event               = coretelemetry.Event
	Observer            = coretelemetry.Observer
	ProtectionObserver  = coretelemetry.ProtectionObserver
)

const (
	EventCallStarted        = coretelemetry.EventCallStarted
	EventPermissionResolved = coretelemetry.EventPermissionResolved
	EventCallCompleted      = coretelemetry.EventCallCompleted
	EventCallFailed         = coretelemetry.EventCallFailed
	EventRecoveryAttempted  = coretelemetry.EventRecoveryAttempted
	EventRecoverySucceeded  = coretelemetry.EventRecoverySucceeded
	EventRecoveryFailed     = coretelemetry.EventRecoveryFailed
)

const (
	ProtectionLoopDetected          = coretelemetry.ProtectionLoopDetected
	ProtectionCallSuppressed        = coretelemetry.ProtectionCallSuppressed
	ProtectionPermissionSuppressed  = coretelemetry.ProtectionPermissionSuppressed
	ProtectionRetryBudgetExhausted  = coretelemetry.ProtectionRetryBudgetExhausted
	ProtectionNoProgressSynthesis   = coretelemetry.ProtectionNoProgressSynthesis
	ProtectionSafetyBudgetExhausted = coretelemetry.ProtectionSafetyBudgetExhausted
	ProtectionTurnDeadlineExceeded  = coretelemetry.ProtectionTurnDeadlineExceeded
)

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

func (s *Service) observeRecovery(ctx context.Context, telemetry callTelemetry, kind EventKind, action tool.RecoveryAction, err error) {
	event := Event{
		Kind: kind, Time: time.Now(), CallID: telemetry.call.ID, ToolName: telemetry.call.Name,
		ToolKind: telemetry.toolKind, ArgumentBytes: len(telemetry.call.Arguments), RecoveryAction: action,
	}
	if err != nil {
		if failure := tool.FailureFromError(err); failure != nil {
			event.ErrorCode = failure.Code
		}
	}
	s.observe(ctx, event)
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
