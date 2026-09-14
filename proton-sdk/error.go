package protonsdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidRequest   = errors.New("invalid model request")
	ErrInvalidEvent     = errors.New("invalid model event")
	ErrIncompleteStream = errors.New("incomplete model stream")
)

type ErrorKind string

const (
	ErrorAuthentication ErrorKind = "authentication"
	ErrorPermission     ErrorKind = "permission"
	ErrorRateLimit      ErrorKind = "rate_limit"
	ErrorInvalidRequest ErrorKind = "invalid_request"
	ErrorModelNotFound  ErrorKind = "model_not_found"
	ErrorContextLength  ErrorKind = "context_length"
	ErrorOverloaded     ErrorKind = "overloaded"
	ErrorTransport      ErrorKind = "transport"
	ErrorProtocol       ErrorKind = "protocol"
	ErrorUnknown        ErrorKind = "unknown"
)

type ProviderError struct {
	Provider   string
	Kind       ErrorKind
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
	RateLimit  *RateLimitInfo
	Cause      error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "provider error"
	}
	prefix := strings.TrimSpace(e.Provider)
	if prefix == "" {
		prefix = "provider"
	}
	detail := string(e.Kind)
	if strings.TrimSpace(e.Code) != "" {
		detail += ", " + e.Code
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s error (%d, %s): %s", prefix, e.StatusCode, detail, e.Message)
	}
	return fmt.Sprintf("%s error (%s): %s", prefix, detail, e.Message)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewProviderError(provider string, status int, code, message string) *ProviderError {
	kind := classifyProviderError(status, code, message)
	return &ProviderError{
		Provider: provider, Kind: kind, StatusCode: status,
		Code: strings.TrimSpace(code), Message: strings.TrimSpace(message),
		Retryable: kind == ErrorRateLimit || kind == ErrorOverloaded || kind == ErrorTransport,
	}
}

func NewTransportError(provider string, cause error) *ProviderError {
	message := "transport request failed"
	if cause != nil {
		message = cause.Error()
	}
	return &ProviderError{Provider: provider, Kind: ErrorTransport, Message: message, Retryable: true, Cause: cause}
}

func classifyProviderError(status int, code, message string) ErrorKind {
	value := strings.ToLower(strings.TrimSpace(code + " " + message))
	switch {
	case strings.Contains(value, "context_length"), strings.Contains(value, "context window"), strings.Contains(value, "prompt is too long"):
		return ErrorContextLength
	case isOverloadedMessage(value):
		return ErrorOverloaded
	case strings.Contains(value, "model_not_found"), strings.Contains(value, "not_found_error"), strings.Contains(value, "modelerror"), strings.Contains(value, "is not supported"):
		return ErrorModelNotFound
	case strings.Contains(value, "authentication"):
		return ErrorAuthentication
	case strings.Contains(value, "permission"):
		return ErrorPermission
	case strings.Contains(value, "rate_limit"):
		return ErrorRateLimit
	case strings.Contains(value, "providerheadertimeouterror"),
		strings.Contains(value, "providerresponsestreamerror"),
		strings.Contains(value, "header timeout"),
		strings.Contains(value, "response stream error"):
		return ErrorTransport
	case strings.Contains(value, "invalid_request"):
		return ErrorInvalidRequest
	}
	switch status {
	case 400, 422:
		return ErrorInvalidRequest
	case 401:
		return ErrorAuthentication
	case 403:
		return ErrorPermission
	case 404:
		return ErrorModelNotFound
	case 413:
		return ErrorContextLength
	case 429:
		return ErrorRateLimit
	case 529:
		return ErrorOverloaded
	}
	if status >= 500 {
		return ErrorOverloaded
	}
	return ErrorUnknown
}

// isOverloadedMessage reports provider-reported server saturation that must
// stay retryable even when no HTTP status is available (mid-stream SSE
// errors). The keyword set mirrors the TUI diagnostic classifier so both
// layers agree on what "overloaded" means. It runs before the generic
// modelerror match so "ModelError: ... overloaded" does not masquerade as
// model_not_found.
func isOverloadedMessage(value string) bool {
	switch {
	case strings.Contains(value, "overloaded"),
		strings.Contains(value, "overloaded_error"),
		strings.Contains(value, "server_is_overloaded"),
		strings.Contains(value, "server_error"),
		strings.Contains(value, "upstream request failed"),
		strings.Contains(value, "service unavailable"),
		strings.Contains(value, "bad gateway"),
		strings.Contains(value, "gateway timeout"):
		return true
	}
	return false
}

type RateLimitKind string

const (
	RateLimitUnknown      RateLimitKind = ""
	RateLimitTransient    RateLimitKind = "transient"
	RateLimitProvider     RateLimitKind = "provider"
	RateLimitFreeUsage    RateLimitKind = "free_usage"
	RateLimitGoFiveHour   RateLimitKind = "go_5_hour"
	RateLimitGoWeekly     RateLimitKind = "go_weekly"
	RateLimitGoMonthly    RateLimitKind = "go_monthly"
	RateLimitUnknownQuota RateLimitKind = "quota"
)

type RateLimitScope string

const (
	RateLimitScopeUnknown  RateLimitScope = ""
	RateLimitScopeRequest  RateLimitScope = "request"
	RateLimitScopeModel    RateLimitScope = "model"
	RateLimitScopeProvider RateLimitScope = "provider"
	RateLimitScopeAccount  RateLimitScope = "account"
	RateLimitScopeIP       RateLimitScope = "ip"
)

type RateLimitInfo struct {
	Kind       RateLimitKind  `json:"kind,omitempty"`
	Scope      RateLimitScope `json:"scope,omitempty"`
	LimitName  string         `json:"limit_name,omitempty"`
	RetryAfter time.Duration  `json:"retry_after,omitempty"`
	ResetAt    time.Time      `json:"reset_at,omitempty"`
	Limit      *int64         `json:"limit,omitempty"`
	Remaining  *int64         `json:"remaining,omitempty"`
}

func ParseRateLimitHeaders(headers http.Header, now time.Time) *RateLimitInfo {
	if len(headers) == 0 {
		return nil
	}
	info := &RateLimitInfo{}
	if value := strings.TrimSpace(headerValue(headers, "Retry-After")); value != "" {
		if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
			info.RetryAfter = time.Duration(seconds) * time.Second
			info.ResetAt = now.Add(info.RetryAfter)
		} else if when, err := http.ParseTime(value); err == nil {
			info.ResetAt = when
			if when.After(now) {
				info.RetryAfter = when.Sub(now)
			}
		}
	}
	info.Limit = firstHeaderInt(headers, "X-RateLimit-Limit", "X-RateLimit-Limit-Requests", "Anthropic-RateLimit-Requests-Limit", "Anthropic-RateLimit-Tokens-Limit")
	info.Remaining = firstHeaderInt(headers, "X-RateLimit-Remaining", "X-RateLimit-Remaining-Requests", "Anthropic-RateLimit-Requests-Remaining", "Anthropic-RateLimit-Tokens-Remaining")
	if info.ResetAt.IsZero() {
		for _, key := range []string{"X-RateLimit-Reset", "X-RateLimit-Reset-Requests", "X-RateLimit-Reset-Tokens", "Anthropic-RateLimit-Requests-Reset", "Anthropic-RateLimit-Tokens-Reset"} {
			if when, ok := parseRateLimitReset(headerValue(headers, key), now); ok {
				info.ResetAt = when
				if when.After(now) {
					info.RetryAfter = when.Sub(now)
				}
				break
			}
		}
	}
	if info.RetryAfter == 0 && info.ResetAt.IsZero() && info.Limit == nil && info.Remaining == nil {
		return nil
	}
	return info
}

func firstHeaderInt(headers http.Header, keys ...string) *int64 {
	for _, key := range keys {
		value := strings.TrimSpace(headerValue(headers, key))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func parseRateLimitReset(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if seconds > 1_000_000_000 {
			return time.Unix(int64(seconds), int64((seconds-float64(int64(seconds)))*float64(time.Second))), true
		}
		if seconds >= 0 {
			return now.Add(time.Duration(seconds * float64(time.Second))), true
		}
	}
	if duration, err := time.ParseDuration(value); err == nil && duration >= 0 {
		return now.Add(duration), true
	}
	if when, err := http.ParseTime(value); err == nil {
		return when, true
	}
	if when, err := time.Parse(time.RFC3339, value); err == nil {
		return when, true
	}
	return time.Time{}, false
}

func headerValue(headers http.Header, key string) string {
	for candidate, values := range headers {
		if !strings.EqualFold(candidate, key) || len(values) == 0 {
			continue
		}
		return values[0]
	}
	return ""
}

// Default retry timing used when a caller does not provide a value. Product
// runtimes should pass their explicit policy at the composition boundary.
const (
	DefaultRetryBaseBackoff = 5 * time.Second
	DefaultRetryMaxBackoff  = 60 * time.Second
	DefaultRetryMaxAfter    = 30 * time.Second
)

var defaultRetryDelays = [...]time.Duration{
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

type RetryPolicy struct {
	BaseBackoff       time.Duration
	PostFirstRetryGap time.Duration
	MaxBackoff        time.Duration
	MaxRetryAfter     time.Duration
	// RetryDelays, when provided, is the exact 1-based retry schedule. Once
	// the schedule is exhausted, its final delay is reused.
	RetryDelays []time.Duration
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
// An explicit RetryDelays schedule takes precedence over the legacy
// exponential calculation. PostFirstRetryGap is retained for callers using
// that legacy calculation and is added only after the first retry.
func RetryDelay(retryIndex int, policy RetryPolicy) time.Duration {
	if retryIndex < 1 {
		retryIndex = 1
	}
	policy = normalizeRetryPolicy(policy)
	if len(policy.RetryDelays) > 0 {
		index := retryIndex - 1
		if index >= len(policy.RetryDelays) {
			index = len(policy.RetryDelays) - 1
		}
		delay := policy.RetryDelays[index]
		if delay <= 0 {
			return 0
		}
		if delay >= policy.MaxBackoff {
			return policy.MaxBackoff
		}
		return delay
	}
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
	useDefaultSchedule := len(policy.RetryDelays) == 0 &&
		policy.BaseBackoff <= 0 && policy.PostFirstRetryGap <= 0 &&
		policy.MaxBackoff <= 0 && policy.MaxRetryAfter <= 0
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = DefaultRetryBaseBackoff
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = DefaultRetryMaxBackoff
	}
	if policy.MaxRetryAfter <= 0 {
		policy.MaxRetryAfter = DefaultRetryMaxAfter
	}
	if useDefaultSchedule {
		policy.RetryDelays = append([]time.Duration(nil), defaultRetryDelays[:]...)
	} else if len(policy.RetryDelays) > 0 {
		policy.RetryDelays = append([]time.Duration(nil), policy.RetryDelays...)
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

type RetryPhase string

const (
	RetryPhaseWaiting  RetryPhase = "waiting"
	RetryPhaseCooldown RetryPhase = "cooldown"
)

// RetryEvent describes one bounded model retry before the retry wait begins.
type RetryEvent struct {
	Provider   string
	ModelID    string
	Reason     string
	Attempt    int
	MaxRetries int
	Phase      RetryPhase
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
	if event.Phase == "" {
		if event.Attempt > 1 {
			event.Phase = RetryPhaseCooldown
		} else {
			event.Phase = RetryPhaseWaiting
		}
	}
	if event.RetryAt.IsZero() {
		event.RetryAt = time.Now().Add(event.Delay)
	}
	event.Provider = strings.TrimSpace(event.Provider)
	event.ModelID = strings.TrimSpace(event.ModelID)
	event.Reason = strings.TrimSpace(event.Reason)
	observer(ctx, event)
}
