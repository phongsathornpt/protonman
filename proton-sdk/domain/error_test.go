package domain_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestProviderErrorFormatting(t *testing.T) {
	err := domain.NewProviderError("openai", 429, "rate_limit_exceeded", "quota exceeded")
	if !err.Retryable {
		t.Fatal("429 should be retryable")
	}
	if err.Error() == "" {
		t.Fatal("empty error string")
	}
	if err.Unwrap() != nil {
		t.Fatal("expected nil cause")
	}

	cause := errors.New("connection reset")
	transportErr := domain.NewTransportError("anthropic", cause)
	if !transportErr.Retryable {
		t.Fatal("transport error should be retryable")
	}
	if !errors.Is(transportErr, cause) {
		t.Fatal("transport error should unwrap to cause")
	}
}

func TestDecideRetryAndDelay(t *testing.T) {
	err := domain.NewProviderError("openai", 429, "rate_limit_exceeded", "slow down")
	policy := domain.RetryPolicy{
		BaseBackoff: 100 * time.Millisecond,
		MaxBackoff:  1 * time.Second,
	}
	decision := domain.DecideRetry(err, 1, policy)
	if !decision.Retry {
		t.Fatal("expected decision.Retry == true")
	}
	if decision.Delay != 100*time.Millisecond {
		t.Fatalf("expected delay 100ms, got %v", decision.Delay)
	}

	nonRetryable := domain.NewProviderError("openai", 401, "auth", "bad key")
	nonDecision := domain.DecideRetry(nonRetryable, 1, policy)
	if nonDecision.Retry {
		t.Fatal("401 should not retry")
	}
}

func TestWaitForRetry(t *testing.T) {
	ctx := context.Background()
	if err := domain.WaitForRetry(ctx, 0); err != nil {
		t.Fatalf("zero delay wait failed: %v", err)
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := domain.WaitForRetry(canceledCtx, 10*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error, got %v", err)
	}
}

func TestObserveRetry(t *testing.T) {
	var observed domain.RetryEvent
	ctx := domain.WithRetryObserver(context.Background(), func(_ context.Context, ev domain.RetryEvent) {
		observed = ev
	})
	domain.ObserveRetry(ctx, domain.RetryEvent{
		Provider: "openai",
		ModelID:  "gpt-4",
		Reason:   "rate_limit",
		Attempt:  1,
	})
	if observed.Provider != "openai" || observed.ModelID != "gpt-4" {
		t.Fatalf("unexpected observed event: %+v", observed)
	}
}
