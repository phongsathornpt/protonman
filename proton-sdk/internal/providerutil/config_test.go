package providerutil

import (
	"net/http"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestBaseConfigNormalizeDefaults(t *testing.T) {
	var cfg BaseConfig
	cfg.Normalize("https://api.example.com/v1")

	if cfg.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("BaseURL = %q, want https://api.example.com/v1", cfg.BaseURL)
	}
	if cfg.HTTPClient == nil {
		t.Fatal("HTTPClient should not be nil")
	}
	if cfg.MaxRetries != 0 {
		t.Fatalf("MaxRetries = %d, want 0", cfg.MaxRetries)
	}
	if cfg.RetryBackoff != domain.DefaultRetryBaseBackoff {
		t.Fatalf("RetryBackoff = %v, want %v", cfg.RetryBackoff, domain.DefaultRetryBaseBackoff)
	}
	if cfg.MaxRetryBackoff != domain.DefaultRetryMaxBackoff {
		t.Fatalf("MaxRetryBackoff = %v, want %v", cfg.MaxRetryBackoff, domain.DefaultRetryMaxBackoff)
	}
	if cfg.MaxRetryAfter != domain.DefaultRetryMaxAfter {
		t.Fatalf("MaxRetryAfter = %v, want %v", cfg.MaxRetryAfter, domain.DefaultRetryMaxAfter)
	}
}

func TestBaseConfigNormalizeTrimsTrailingSlash(t *testing.T) {
	cfg := BaseConfig{BaseURL: "  https://api.example.com/v1/// "}
	cfg.Normalize("https://fallback.example.com")

	if cfg.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("BaseURL = %q, want https://api.example.com/v1", cfg.BaseURL)
	}
}

func TestBaseConfigNormalizeClonesHeadersAndDelays(t *testing.T) {
	headers := http.Header{"X-Custom": []string{"original"}}
	delays := []time.Duration{100 * time.Millisecond}
	cfg := BaseConfig{
		Headers:     headers,
		RetryDelays: delays,
	}
	cfg.Normalize("https://api.example.com")

	headers.Set("X-Custom", "mutated")
	delays[0] = 5 * time.Second

	if cfg.Headers.Get("X-Custom") != "original" {
		t.Fatalf("Headers mutated: %q", cfg.Headers.Get("X-Custom"))
	}
	if cfg.RetryDelays[0] != 100*time.Millisecond {
		t.Fatalf("RetryDelays mutated: %v", cfg.RetryDelays[0])
	}
}

func TestBaseConfigRetryPolicy(t *testing.T) {
	cfg := BaseConfig{
		RetryBackoff:      2 * time.Second,
		RetryPostFirstGap: 500 * time.Millisecond,
		MaxRetryBackoff:   30 * time.Second,
		MaxRetryAfter:     15 * time.Second,
		RetryDelays:       []time.Duration{time.Second, 2 * time.Second},
	}
	policy := cfg.RetryPolicy()

	if policy.BaseBackoff != 2*time.Second {
		t.Fatalf("BaseBackoff = %v", policy.BaseBackoff)
	}
	if policy.PostFirstRetryGap != 500*time.Millisecond {
		t.Fatalf("PostFirstRetryGap = %v", policy.PostFirstRetryGap)
	}
	if policy.MaxBackoff != 30*time.Second {
		t.Fatalf("MaxBackoff = %v", policy.MaxBackoff)
	}
	if policy.MaxRetryAfter != 15*time.Second {
		t.Fatalf("MaxRetryAfter = %v", policy.MaxRetryAfter)
	}
	if len(policy.RetryDelays) != 2 || policy.RetryDelays[0] != time.Second {
		t.Fatalf("RetryDelays = %v", policy.RetryDelays)
	}
}
