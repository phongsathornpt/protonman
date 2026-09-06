package model

import (
	"net/http"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/buildinfo"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
)

// clientConfig contains CLI-owned settings used to construct proton-sdk provider models.
type clientConfig struct {
	baseURL    string
	apiKey     string
	modelID    string
	sessionID  string
	clientName string
	userAgent  string
	httpClient *http.Client
	vision     *bool
}

// ClientOption configures provider model construction.
type ClientOption func(*clientConfig)

func WithSessionID(sessionID string) ClientOption {
	return func(c *clientConfig) { c.sessionID = sessionID }
}

func WithClientName(clientName string) ClientOption {
	return func(c *clientConfig) { c.clientName = clientName }
}

func WithUserAgent(userAgent string) ClientOption {
	return func(c *clientConfig) { c.userAgent = userAgent }
}

func WithVisionSupport(supported bool) ClientOption {
	return func(c *clientConfig) { c.vision = &supported }
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
