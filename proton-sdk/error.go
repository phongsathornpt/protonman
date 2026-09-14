package protonsdk

import (
	"context"
	"net/http"
	"time"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/internal/providerutil"
)

var (
	ErrInvalidRequest   = domain.ErrInvalidRequest
	ErrInvalidEvent     = domain.ErrInvalidEvent
	ErrIncompleteStream = domain.ErrIncompleteStream
)

const (
	ErrorAuthentication = domain.ErrorAuthentication
	ErrorPermission     = domain.ErrorPermission
	ErrorRateLimit      = domain.ErrorRateLimit
	ErrorInvalidRequest = domain.ErrorInvalidRequest
	ErrorModelNotFound  = domain.ErrorModelNotFound
	ErrorContextLength  = domain.ErrorContextLength
	ErrorOverloaded     = domain.ErrorOverloaded
	ErrorTransport      = domain.ErrorTransport
	ErrorProtocol       = domain.ErrorProtocol
	ErrorUnknown        = domain.ErrorUnknown
)

const (
	RateLimitUnknown      = domain.RateLimitUnknown
	RateLimitTransient    = domain.RateLimitTransient
	RateLimitProvider     = domain.RateLimitProvider
	RateLimitFreeUsage    = domain.RateLimitFreeUsage
	RateLimitGoFiveHour   = domain.RateLimitGoFiveHour
	RateLimitGoWeekly     = domain.RateLimitGoWeekly
	RateLimitGoMonthly    = domain.RateLimitGoMonthly
	RateLimitUnknownQuota = domain.RateLimitUnknownQuota
)

const (
	RateLimitScopeUnknown  = domain.RateLimitScopeUnknown
	RateLimitScopeRequest  = domain.RateLimitScopeRequest
	RateLimitScopeModel    = domain.RateLimitScopeModel
	RateLimitScopeProvider = domain.RateLimitScopeProvider
	RateLimitScopeAccount  = domain.RateLimitScopeAccount
	RateLimitScopeIP       = domain.RateLimitScopeIP
)

const (
	DefaultRetryBaseBackoff = domain.DefaultRetryBaseBackoff
	DefaultRetryMaxBackoff  = domain.DefaultRetryMaxBackoff
	DefaultRetryMaxAfter    = domain.DefaultRetryMaxAfter
)

const (
	RetryPhaseWaiting  = domain.RetryPhaseWaiting
	RetryPhaseCooldown = domain.RetryPhaseCooldown
)

func NewProviderError(provider string, status int, code, message string) *ProviderError {
	return domain.NewProviderError(provider, status, code, message)
}

func NewTransportError(provider string, cause error) *ProviderError {
	return domain.NewTransportError(provider, cause)
}

func DecideRetry(err error, retryIndex int, policy RetryPolicy) RetryDecision {
	return domain.DecideRetry(err, retryIndex, policy)
}

func RetryDelay(retryIndex int, policy RetryPolicy) time.Duration {
	return domain.RetryDelay(retryIndex, policy)
}

func WaitForRetry(ctx context.Context, delay time.Duration) error {
	return domain.WaitForRetry(ctx, delay)
}

func WithRetryObserver(ctx context.Context, observer RetryObserver) context.Context {
	return domain.WithRetryObserver(ctx, observer)
}

func ObserveRetry(ctx context.Context, event RetryEvent) {
	domain.ObserveRetry(ctx, event)
}

func ParseRateLimitHeaders(headers http.Header, now time.Time) *RateLimitInfo {
	return providerutil.ParseRateLimitHeaders(headers, now)
}


