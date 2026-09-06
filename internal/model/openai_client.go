package model

import (
	"net/http"
	"strings"
	"time"
)

type OpenAIClient struct {
	openAIClientConfig
}

var _ Client = (*OpenAIClient)(nil)

// openAIClientConfig contains settings shared by the legacy compatible client
// and the official openai-go adapter.
type openAIClientConfig struct {
	baseURL    string
	apiKey     string
	modelID    string
	sessionID  string
	clientName string
	userAgent  string
	httpClient *http.Client
}

// OpenAIOption configures an OpenAI-compatible client.
type OpenAIOption func(*openAIClientConfig)

// WithSessionID sets the session identifier for sticky routing and prompt cache optimization.
func WithSessionID(sessionID string) OpenAIOption {
	return func(c *openAIClientConfig) {
		c.sessionID = sessionID
	}
}

// WithClientName sets the client identifier (e.g. "proton").
func WithClientName(clientName string) OpenAIOption {
	return func(c *openAIClientConfig) {
		c.clientName = clientName
	}
}

// WithUserAgent sets a custom User-Agent header (defaults to "Proton/1.0").
func WithUserAgent(userAgent string) OpenAIOption {
	return func(c *openAIClientConfig) {
		c.userAgent = userAgent
	}
}

func newOpenAIClientConfig(baseURL string, apiKey string, modelID string) openAIClientConfig {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultProtonmanEndpoint
	}

	return openAIClientConfig{
		baseURL:    baseURL,
		apiKey:     apiKey,
		modelID:    NormalizeModelID(baseURL, modelID),
		clientName: "proton",
		userAgent:  "Proton/1.0",
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// NewOpenAIClient creates a client targeting an OpenAI, OpenCode, or Protonman chat API.
func NewOpenAIClient(baseURL string, apiKey string, modelID string, opts ...OpenAIOption) *OpenAIClient {
	client := &OpenAIClient{
		openAIClientConfig: newOpenAIClientConfig(baseURL, apiKey, modelID),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&client.openAIClientConfig)
		}
	}
	return client
}

// SessionID returns the configured session identifier.
func (c *OpenAIClient) SessionID() string {
	return c.sessionID
}

// SetSessionID updates the session identifier on an existing client.
func (c *OpenAIClient) SetSessionID(sessionID string) {
	c.sessionID = sessionID
}
