package protonsdk

import (
	"context"
	"errors"
	"time"
)

// Default retry timing used when a caller does not provide a value. Product
// runtimes should pass their explicit policy at the composition boundary.
const (
	DefaultRetryBaseBackoff = 500 * time.Millisecond
	DefaultRetryMaxBackoff  = 8 * time.Second
	DefaultRetryMaxAfter    = 30 * time.Second
)

type RetryPolicy struct {
	BaseBackoff       time.Duration
	PostFirstRetryGap time.Duration
	MaxBackoff        time.Duration
	MaxRetryAfter     time.Duration
}

type RetryDecision struct {
	Retry  bool
	Delay  time.Duration
	Reason ErrorKind
}

func DecideRetry(err error, retryIndex int, policy RetryPolicy) RetryDecision {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil || !providerErr.Retryable {
		return RetryDecision{}
	}
	if retryIndex < 1 {
		retryIndex = 1
	}
	policy = normalizeRetryPolicy(policy)
	if providerErr.RateLimit != nil && (providerErr.RateLimit.RetryAfter > 0 || !providerErr.RateLimit.ResetAt.IsZero()) {
		if providerErr.RateLimit.RetryAfter > policy.MaxRetryAfter {
			return RetryDecision{Reason: providerErr.Kind}
		}
		return RetryDecision{Retry: true, Delay: providerErr.RateLimit.RetryAfter, Reason: providerErr.Kind}
	}
	return RetryDecision{Retry: true, Delay: RetryDelay(retryIndex, policy), Reason: providerErr.Kind}
}

// RetryDelay returns the bounded local retry delay for a 1-based retry index.
// PostFirstRetryGap is added only after the first retry, allowing callers to
// create an explicit cooldown before subsequent attempts without affecting the
// first recovery attempt.
func RetryDelay(retryIndex int, policy RetryPolicy) time.Duration {
	if retryIndex < 1 {
		retryIndex = 1
	}
	policy = normalizeRetryPolicy(policy)
	if policy.BaseBackoff >= policy.MaxBackoff {
		return policy.MaxBackoff
	}
	delay := policy.BaseBackoff
	for i := 1; i < retryIndex && delay < policy.MaxBackoff; i++ {
		// Check before doubling so a large retry index cannot overflow a
		// time.Duration and turn a bounded delay into a negative duration.
		if delay > policy.MaxBackoff/2 {
			delay = policy.MaxBackoff
			break
		}
		delay *= 2
	}
	if retryIndex > 1 && policy.PostFirstRetryGap > 0 {
		if policy.PostFirstRetryGap >= policy.MaxBackoff-delay {
			delay = policy.MaxBackoff
		} else {
			delay += policy.PostFirstRetryGap
		}
	}
	return delay
}

func normalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = DefaultRetryBaseBackoff
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = DefaultRetryMaxBackoff
	}
	if policy.MaxRetryAfter <= 0 {
		policy.MaxRetryAfter = DefaultRetryMaxAfter
	}
	return policy
}

// WaitForRetry waits for a retry delay while preserving cancellation even
// when the delay is zero. Keeping this at the SDK boundary prevents provider
// and adapter retry loops from drifting into subtly different behavior.
func WaitForRetry(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
