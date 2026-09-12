package turn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/contextutil"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
)

func shouldWarnSoftToolBudget(used, max int, warned bool) bool {
	if warned || max <= 0 || used <= 0 {
		return false
	}
	return used*5 >= max*3
}

func (l *Loop) newTurnContext(parent context.Context) (context.Context, context.CancelFunc) {
	if l.turnTimeout > 0 {
		return context.WithTimeout(parent, l.turnTimeout)
	}
	return context.WithCancel(parent)
}

func (l *Loop) newRoundContext(parent context.Context) (context.Context, context.CancelFunc) {
	if l.roundTimeout > 0 {
		return context.WithTimeout(parent, l.roundTimeout)
	}
	return context.WithCancel(parent)
}

func (l *Loop) observeSuppression(ctx context.Context, round int, execution executedCall) {
	event := toolcall.ProtectionEvent{
		Kind: toolcall.ProtectionCallSuppressed, Time: time.Now(), Round: round,
		ToolName: execution.call.Name, Reason: execution.suppressionReason,
		Fingerprint: execution.semanticFingerprint, RepeatCount: execution.repeatCount,
		Retryable: execution.retryable,
	}
	if execution.result.Failure != nil {
		event.ErrorCode = execution.result.Failure.Code
	}
	for _, definition := range l.tools.Definitions() {
		if definition.Name == execution.call.Name {
			event.ToolKind = permission.ToolKind(definition.Kind)
			break
		}
	}
	l.observeProtection(ctx, event)
	specific := event
	switch execution.suppressionReason {
	case "permission_retry":
		specific.Kind = toolcall.ProtectionPermissionSuppressed
	case "retry_budget_exhausted":
		specific.Kind = toolcall.ProtectionRetryBudgetExhausted
	default:
		return
	}
	l.observeProtection(ctx, specific)
}

func (l *Loop) observeProtection(ctx context.Context, event toolcall.ProtectionEvent) {
	if l == nil || l.tools == nil {
		return
	}
	l.tools.ObserveProtection(ctx, event)
}

func (l *Loop) fail(ctx context.Context, sink Sink, round int, err error) (Result, error) {
	return l.failWithResult(ctx, sink, round, Result{}, err)
}

func (l *Loop) failWithResult(ctx context.Context, sink Sink, round int, result Result, err error) (Result, error) {
	if errors.Is(err, context.DeadlineExceeded) && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		observeCtx, observeCancel := contextutil.DetachedTimeout(ctx, protectionObserverTimeout)
		l.observeProtection(observeCtx, toolcall.ProtectionEvent{Kind: toolcall.ProtectionTurnDeadlineExceeded, Time: time.Now(), Round: round, Reason: "turn_deadline"})
		observeCancel()
	}
	slog.DebugContext(ctx, "turn failed",
		"round", round,
		"error_type", fmt.Sprintf("%T", err),
		"context_error", ctx.Err() != nil,
	)
	emitContext := ctx
	emitCancel := func() {}
	if ctx.Err() != nil {
		// A terminal failure still needs to reach adapters after cancellation,
		// but detached cleanup must remain bounded.
		emitContext, emitCancel = contextutil.DetachedTimeout(ctx, terminalEmitTimeout)
	}
	defer emitCancel()
	if emitErr := emit(emitContext, sink, Event{
		Kind:  EventFailed,
		Round: round,
		Err:   err,
	}); emitErr != nil {
		slog.DebugContext(emitContext, "turn failure event failed",
			"round", round,
			"error_type", fmt.Sprintf("%T", emitErr),
		)
		return result, emitErr
	}
	return result, err
}

func emit(ctx context.Context, sink Sink, event Event) error {
	if err := ctx.Err(); err != nil {
		slog.DebugContext(ctx, "turn event emission cancelled",
			"event_kind", event.Kind,
			"round", event.Round,
			"error_type", fmt.Sprintf("%T", err),
		)
		return fmt.Errorf("emit model/tool event: %w", err)
	}
	if err := sink(ctx, event); err != nil {
		slog.DebugContext(ctx, "turn event sink failed",
			"event_kind", event.Kind,
			"round", event.Round,
			"error_type", fmt.Sprintf("%T", err),
		)
		return fmt.Errorf("emit model/tool event: %w", err)
	}
	return nil
}
