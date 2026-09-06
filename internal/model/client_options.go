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

// ClientOption configures the CLI-to-SDK provider bridge.
type ClientOption func(*clientConfig)

// OpenAIOption is retained as a compatibility alias for callers compiled against the old bridge name.
type OpenAIOption = ClientOption

func WithSessionID(sessionID string) ClientOption {
	return func(c *clientConfig) { c.sessionID = sessionID }
}

func WithClientName(clientName string) ClientOption {
	return func(c *clientConfig) { c.clientName = clientName }
}

func WithUserAgent(userAgent string) ClientOption {
	return func(c *clientConfig) { c.userAgent = userAgent }
}

func WithRequestTimeout(timeout time.Duration) ClientOption {
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
