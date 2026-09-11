package protonsdk

import (
	"fmt"
	"strings"
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
