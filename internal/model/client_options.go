package model

import (
	"net/http"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/buildinfo"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
)

// clientConfig is the temporary CLI compatibility configuration applied when
// constructing proton-sdk provider models.
type clientConfig struct {
	baseURL    string
	apiKey     string
	modelID    string
	sessionID  string
	clientName string
	userAgent  string
	httpClient *http.Client
}

// OpenAIOption is retained as a compatibility name while callers migrate to
// provider-neutral SDK construction options.
type OpenAIOption func(*clientConfig)

func WithSessionID(sessionID string) OpenAIOption {
	return func(c *clientConfig) { c.sessionID = sessionID }
}

func WithClientName(clientName string) OpenAIOption {
	return func(c *clientConfig) { c.clientName = clientName }
}

func WithUserAgent(userAgent string) OpenAIOption {
	return func(c *clientConfig) { c.userAgent = userAgent }
}

func WithRequestTimeout(timeout time.Duration) OpenAIOption {
	return func(c *clientConfig) {
		if timeout > 0 && c.httpClient != nil {
			c.httpClient.Timeout = timeout
		}
	}
}

func newClientConfig(baseURL, apiKey, modelID string) clientConfig {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultProtonmanEndpoint
	}
	return clientConfig{
		baseURL:    baseURL,
		apiKey:     apiKey,
		modelID:    strings.TrimSpace(modelID),
		clientName: "proton",
		userAgent:  buildinfo.UserAgent(),
		httpClient: &http.Client{Timeout: runtimepolicy.ModelRequestTimeout},
	}
}
