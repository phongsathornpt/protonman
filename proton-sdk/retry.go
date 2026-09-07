package protonsdk

import (
	"errors"
	"time"
)

type RetryPolicy struct {
	BaseBackoff   time.Duration
	MaxBackoff    time.Duration
	MaxRetryAfter time.Duration
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
		policy.BaseBackoff = 500 * time.Millisecond
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = 8 * time.Second
	}
	if policy.MaxRetryAfter <= 0 {
		policy.MaxRetryAfter = 30 * time.Second
	}
	if providerErr.RateLimit != nil && providerErr.RateLimit.RetryAfter > 0 {
		if providerErr.RateLimit.RetryAfter > policy.MaxRetryAfter {
			return RetryDecision{Reason: providerErr.Kind}
		}
		return RetryDecision{Retry: true, Delay: providerErr.RateLimit.RetryAfter, Reason: providerErr.Kind}
	}
	delay := policy.BaseBackoff
	for i := 1; i < retryIndex && delay < policy.MaxBackoff; i++ {
		delay *= 2
		if delay > policy.MaxBackoff {
			delay = policy.MaxBackoff
		}
	}
	return RetryDecision{Retry: true, Delay: delay, Reason: providerErr.Kind}
}
