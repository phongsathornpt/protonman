package protonsdk

import (
	"context"
	"strings"
	"time"
)

// RetryEvent describes one bounded model retry before the retry wait begins.
type RetryEvent struct {
	Provider   string
	ModelID    string
	Reason     string
	Attempt    int
	MaxRetries int
	Delay      time.Duration
	RetryAt    time.Time
}

// RetryObserver receives optional retry lifecycle notifications. Observers are
// presentation/telemetry hooks only and must not change retry semantics.
type RetryObserver func(context.Context, RetryEvent)

type retryObserverContextKey struct{}

// WithRetryObserver attaches a request-scoped retry observer.
func WithRetryObserver(ctx context.Context, observer RetryObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if observer == nil {
		return ctx
	}
	previous, _ := ctx.Value(retryObserverContextKey{}).(RetryObserver)
	if previous == nil {
		return context.WithValue(ctx, retryObserverContextKey{}, observer)
	}
	return context.WithValue(ctx, retryObserverContextKey{}, RetryObserver(func(observeCtx context.Context, event RetryEvent) {
		previous(observeCtx, event)
		observer(observeCtx, event)
	}))
}

// ObserveRetry notifies a request-scoped observer when one is present.
func ObserveRetry(ctx context.Context, event RetryEvent) {
	if ctx == nil {
		return
	}
	observer, _ := ctx.Value(retryObserverContextKey{}).(RetryObserver)
	if observer == nil {
		return
	}
	if event.Delay < 0 {
		event.Delay = 0
	}
	if event.RetryAt.IsZero() {
		event.RetryAt = time.Now().Add(event.Delay)
	}
	event.Provider = strings.TrimSpace(event.Provider)
	event.ModelID = strings.TrimSpace(event.ModelID)
	event.Reason = strings.TrimSpace(event.Reason)
	observer(ctx, event)
}
