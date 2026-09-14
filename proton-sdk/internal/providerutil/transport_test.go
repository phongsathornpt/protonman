package providerutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

type dummyStream struct{}

func (dummyStream) Next(context.Context) (domain.Event, error) { return domain.Event{}, io.EOF }
func (dummyStream) Close() error                               { return nil }

func TestExecuteStreamSuccessOnFirstAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "val" {
			t.Errorf("header X-Custom = %q", r.Header.Get("X-Custom"))
		}
		if r.Header.Get(SessionIDHeader) != "sess_123" {
			t.Errorf("header %s = %q", SessionIDHeader, r.Header.Get(SessionIDHeader))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	stream, err := ExecuteStream(context.Background(), StreamRequest{
		ProviderName: "test-provider",
		ModelID:      "test-model",
		Endpoint:     server.URL,
		Payload:      []byte(`{}`),
		Headers:      http.Header{"X-Custom": []string{"val"}},
		SessionID:    "sess_123",
		HTTPClient:   server.Client(),
		OnSuccess: func(resp *http.Response) (port.Stream, error) {
			resp.Body.Close()
			return dummyStream{}, nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	if stream == nil {
		t.Fatal("ExecuteStream() stream = nil")
	}
}

func TestExecuteStreamRetriesOnTransientError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`rate limit exceeded`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	var observedRetries int
	ctx := domain.WithRetryObserver(context.Background(), func(ctx context.Context, event domain.RetryEvent) {
		observedRetries++
	})

	stream, err := ExecuteStream(ctx, StreamRequest{
		ProviderName: "test-provider",
		ModelID:      "test-model",
		Endpoint:     server.URL,
		Payload:      []byte(`{}`),
		HTTPClient:   server.Client(),
		MaxRetries:   2,
		RetryPolicy: domain.RetryPolicy{
			RetryDelays: []time.Duration{time.Millisecond},
		},
		ParseError: func(status int, body []byte, headers http.Header) *domain.ProviderError {
			return domain.NewProviderError("test-provider", status, "rate_limit", string(body))
		},
		OnSuccess: func(resp *http.Response) (port.Stream, error) {
			resp.Body.Close()
			return dummyStream{}, nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	if stream == nil {
		t.Fatal("ExecuteStream() stream = nil")
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
	if observedRetries != 1 {
		t.Fatalf("observedRetries = %d, want 1", observedRetries)
	}
}

func TestExecuteStreamAbortsOnContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`server overloaded`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ExecuteStream(ctx, StreamRequest{
		ProviderName: "test-provider",
		ModelID:      "test-model",
		Endpoint:     server.URL,
		HTTPClient:   server.Client(),
		MaxRetries:   3,
		RetryPolicy: domain.RetryPolicy{
			RetryDelays: []time.Duration{time.Second},
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestExecuteStreamReturnsErrorWhenRetriesExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`slow down`))
	}))
	defer server.Close()

	_, err := ExecuteStream(context.Background(), StreamRequest{
		ProviderName: "test-provider",
		ModelID:      "test-model",
		Endpoint:     server.URL,
		HTTPClient:   server.Client(),
		MaxRetries:   1,
		RetryPolicy: domain.RetryPolicy{
			RetryDelays: []time.Duration{time.Millisecond},
		},
		ParseError: func(status int, body []byte, headers http.Header) *domain.ProviderError {
			return domain.NewProviderError("test-provider", status, "rate_limit", string(body))
		},
	})
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if !strings.Contains(providerErr.Message, "slow down") {
		t.Fatalf("message = %q, want slow down", providerErr.Message)
	}
}
