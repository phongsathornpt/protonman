package protonsdk

import (
	"context"
	"testing"
	"time"
)

func TestObserveRetryNormalizesDeadline(t *testing.T) {
	var got RetryEvent
	ctx := WithRetryObserver(context.Background(), func(_ context.Context, event RetryEvent) { got = event })
	started := time.Now()
	ObserveRetry(ctx, RetryEvent{Provider: " opencode ", ModelID: " free ", Reason: " rate_limit ", Attempt: 1, MaxRetries: 2, Delay: 25 * time.Millisecond})
	if got.Provider != "opencode" || got.ModelID != "free" || got.Reason != "rate_limit" {
		t.Fatalf("retry event = %+v", got)
	}
	if got.Attempt != 1 || got.MaxRetries != 2 || got.Phase != RetryPhaseWaiting || got.Delay != 25*time.Millisecond {
		t.Fatalf("retry budget = %+v", got)
	}
	if got.RetryAt.Before(started.Add(20*time.Millisecond)) || got.RetryAt.After(time.Now().Add(30*time.Millisecond)) {
		t.Fatalf("retry deadline = %v, want about 25ms from now", got.RetryAt)
	}
}

func TestObserveRetryMarksSubsequentAttemptsAsCooldown(t *testing.T) {
	var got RetryEvent
	ctx := WithRetryObserver(context.Background(), func(_ context.Context, event RetryEvent) { got = event })
	ObserveRetry(ctx, RetryEvent{Attempt: 2, MaxRetries: 2, Delay: 2 * time.Second})
	if got.Phase != RetryPhaseCooldown {
		t.Fatalf("phase = %q, want cooldown", got.Phase)
	}
}

func TestWithRetryObserverComposesExistingObserver(t *testing.T) {
	var calls []string
	ctx := WithRetryObserver(context.Background(), func(context.Context, RetryEvent) { calls = append(calls, "first") })
	ctx = WithRetryObserver(ctx, func(context.Context, RetryEvent) { calls = append(calls, "second") })
	ObserveRetry(ctx, RetryEvent{Attempt: 1})
	if len(calls) != 2 || calls[0] != "first" || calls[1] != "second" {
		t.Fatalf("observer calls = %#v", calls)
	}
}
