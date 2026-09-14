package providerutil

import (
	"net/http"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

// BaseConfig centralizes common provider configuration across SDK protocol providers.
type BaseConfig struct {
	BaseURL           string
	APIKey            string
	HTTPClient        *http.Client
	UserAgent         string
	Headers           http.Header
	MaxRetries        int
	RetryBackoff      time.Duration
	RetryPostFirstGap time.Duration
	MaxRetryBackoff   time.Duration
	MaxRetryAfter     time.Duration
	RetryDelays       []time.Duration
}

// Normalize sanitizes provider options, applying defaults for base URL, HTTP client,
// retries, backoff, and cloning headers/delays to prevent external mutation.
func (c *BaseConfig) Normalize(defaultBaseURL string) {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{}
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = domain.DefaultRetryBaseBackoff
	}
	if c.MaxRetryBackoff <= 0 {
		c.MaxRetryBackoff = domain.DefaultRetryMaxBackoff
	}
	if c.MaxRetryAfter <= 0 {
		c.MaxRetryAfter = domain.DefaultRetryMaxAfter
	}
	c.Headers = c.Headers.Clone()
	c.RetryDelays = append([]time.Duration(nil), c.RetryDelays...)
}

// RetryPolicy derives the canonical SDK retry policy from the configuration.
func (c BaseConfig) RetryPolicy() domain.RetryPolicy {
	return domain.RetryPolicy{
		BaseBackoff:       c.RetryBackoff,
		PostFirstRetryGap: c.RetryPostFirstGap,
		MaxBackoff:        c.MaxRetryBackoff,
		MaxRetryAfter:     c.MaxRetryAfter,
		RetryDelays:       c.RetryDelays,
	}
}
