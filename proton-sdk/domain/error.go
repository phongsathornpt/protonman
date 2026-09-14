package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidRequest   = errors.New("invalid model request")
	ErrInvalidEvent     = errors.New("invalid model event")
	ErrIncompleteStream = errors.New("incomplete model stream")
	ErrInvalidToolInput = errors.New("invalid tool input")
	ErrInvalidToolOutput = errors.New("invalid tool output")
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

var defaultRetryDelays = [...]time.Duration{
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Code != "" {
		return fmt.Sprintf("%s error (%s, %s): %s", e.Provider, e.Kind, e.Code, e.Message)
	}
	return fmt.Sprintf("%s error (%s): %s", e.Provider, e.Kind, e.Message)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewProviderError(provider string, statusCode int, code, message string) *ProviderError {
	provider = strings.TrimSpace(provider)
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	kind := classifyHTTPError(statusCode, code, message)
	retryable := false
	switch kind {
	case ErrorRateLimit, ErrorOverloaded:
		retryable = true
	case ErrorTransport:
		retryable = true
	}
	return &ProviderError{
		Provider:   provider,
		Kind:       kind,
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Retryable:  retryable,
	}
}

func NewTransportError(provider string, cause error) *ProviderError {
	provider = strings.TrimSpace(provider)
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	return &ProviderError{
		Provider:  provider,
		Kind:      ErrorTransport,
		Message:   message,
		Retryable: true,
		Cause:     cause,
	}
}

func classifyHTTPError(status int, code, message string) ErrorKind {
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

func classifyMessage(message string, fallback ErrorKind) ErrorKind {
	lowered := strings.ToLower(message)
	switch {
	case strings.Contains(lowered, "context length"), strings.Contains(lowered, "maximum context"), strings.Contains(lowered, "too many tokens"):
		return ErrorContextLength
	case strings.Contains(lowered, "rate limit"), strings.Contains(lowered, "quota"):
		return ErrorRateLimit
	case strings.Contains(lowered, "model") && strings.Contains(lowered, "not found"):
		return ErrorModelNotFound
	case isOverloadedMessage(lowered):
		return ErrorOverloaded
	default:
		return fallback
	}
}

func isOverloadedMessage(value string) bool {
	for _, candidate := range []string{
		"busy",
		"overloaded",
		"service unavailable",
		"bad gateway",
		"gateway timeout",
		"server error",
		"upstream request failed",
	} {
		if strings.Contains(value, candidate) {
			return true
		}
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

const (
	DefaultRetryBaseBackoff = 5 * time.Second
	DefaultRetryMaxBackoff  = 60 * time.Second
	DefaultRetryMaxAfter    = 30 * time.Second
)

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

type RetryObserver func(context.Context, RetryEvent)

type retryObserverContextKey struct{}

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
