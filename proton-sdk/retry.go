package protonsdk

import (
	"errors"
	"time"
)

// Default retry timing. These are the SDK-owned canonical values; the CLI
// runtime policy mirrors them (see internal/base/runtimepolicy) and the
// cross-boundary mapping is pinned by TestRetryDefaultsMatchRuntimePolicy.
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
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = DefaultRetryBaseBackoff
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = DefaultRetryMaxBackoff
	}
	if policy.MaxRetryAfter <= 0 {
		policy.MaxRetryAfter = DefaultRetryMaxAfter
	}
	if providerErr.RateLimit != nil && providerErr.RateLimit.RetryAfter > 0 {
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
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = DefaultRetryBaseBackoff
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = DefaultRetryMaxBackoff
	}
	delay := policy.BaseBackoff
	for i := 1; i < retryIndex && delay < policy.MaxBackoff; i++ {
		delay *= 2
		if delay > policy.MaxBackoff {
			delay = policy.MaxBackoff
		}
	}
	if retryIndex > 1 && policy.PostFirstRetryGap > 0 {
		delay += policy.PostFirstRetryGap
		if delay > policy.MaxBackoff {
			delay = policy.MaxBackoff
		}
	}
	return delay
}
