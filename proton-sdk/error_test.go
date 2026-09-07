package protonsdk

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestProviderErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   ErrorKind
	}{
		{401, "authentication_error", ErrorAuthentication},
		{403, "permission_error", ErrorPermission},
		{429, "rate_limit_error", ErrorRateLimit},
		{404, "model_not_found", ErrorModelNotFound},
		{401, "ModelError", ErrorModelNotFound},
		{400, "context_length_exceeded", ErrorContextLength},
		{529, "overloaded_error", ErrorOverloaded},
	} {
		err := NewProviderError("test", tc.status, tc.code, "message")
		if err.Kind != tc.want {
			t.Fatalf("status=%d code=%q kind=%q want=%q", tc.status, tc.code, err.Kind, tc.want)
		}
	}
}

func TestTransportErrorUnwrapsCause(t *testing.T) {
	cause := fmt.Errorf("network down")
	err := NewTransportError("test", cause)
	if err.Kind != ErrorTransport || !err.Retryable || !errors.Is(err, cause) {
		t.Fatalf("transport error = %#v", err)
	}
}

func TestParseRateLimitHeaders(t *testing.T) {
	now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	headers := http.Header{
		"Retry-After":           []string{"12"},
		"X-RateLimit-Limit":     []string{"100"},
		"X-RateLimit-Remaining": []string{"0"},
	}
	got := ParseRateLimitHeaders(headers, now)
	if got == nil || got.RetryAfter != 12*time.Second || !got.ResetAt.Equal(now.Add(12*time.Second)) {
		t.Fatalf("rate limit = %#v", got)
	}
	if got.Limit == nil || *got.Limit != 100 || got.Remaining == nil || *got.Remaining != 0 {
		t.Fatalf("rate limit counters = %#v", got)
	}
}

func TestParseRateLimitHeadersSupportsHTTPDateAndDurationReset(t *testing.T) {
	now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	httpDate := now.Add(45 * time.Second).Format(http.TimeFormat)
	if got := ParseRateLimitHeaders(http.Header{"Retry-After": []string{httpDate}}, now); got == nil || got.RetryAfter != 45*time.Second {
		t.Fatalf("HTTP-date rate limit = %#v", got)
	}
	if got := ParseRateLimitHeaders(http.Header{"X-RateLimit-Reset": []string{"2.5s"}}, now); got == nil || got.RetryAfter != 2500*time.Millisecond {
		t.Fatalf("duration reset rate limit = %#v", got)
	}
}
