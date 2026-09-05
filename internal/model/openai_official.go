package model

import (
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OfficialOpenAIClient is the official openai-go adapter under construction.
//
// It deliberately shares configuration with OpenAIClient so the runtime cutover
// can preserve headers, timeouts, and session routing without changing callers.
type OfficialOpenAIClient struct {
	openAIClientConfig
	sdk openai.Client
}

// NewOfficialOpenAIClient creates an official openai-go client without changing
// the currently wired compatible-provider client.
func NewOfficialOpenAIClient(
	baseURL string,
	apiKey string,
	modelID string,
	opts ...OpenAIOption,
) *OfficialOpenAIClient {
	client := &OfficialOpenAIClient{
		openAIClientConfig: newOpenAIClientConfig(baseURL, apiKey, modelID),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&client.openAIClientConfig)
		}
	}

	client.sdk = openai.NewClient(officialOpenAIOptions(client.openAIClientConfig)...)
	return client
}

func officialOpenAIOptions(config openAIClientConfig) []option.RequestOption {
	requestOptions := []option.RequestOption{
		option.WithAPIKey(config.apiKey),
		option.WithBaseURL(config.baseURL),
		option.WithMaxRetries(2),
	}
	if config.httpClient != nil {
		requestOptions = append(requestOptions,
			option.WithHTTPClient(config.httpClient),
			option.WithRequestTimeout(config.httpClient.Timeout),
		)
	}
	if config.userAgent != "" {
		requestOptions = append(requestOptions, option.WithHeader("User-Agent", config.userAgent))
	}
	if config.sessionID != "" {
		requestOptions = append(requestOptions,
			option.WithHeader("x-session-affinity", config.sessionID),
			option.WithHeader("X-Session-Id", config.sessionID),
		)
	}
	return requestOptions
}

// SessionID returns the configured session identifier.
func (c *OfficialOpenAIClient) SessionID() string {
	return c.sessionID
}

// SetSessionID updates the session identifier for future requests.
func (c *OfficialOpenAIClient) SetSessionID(sessionID string) {
	c.sessionID = sessionID
	requestOptions := officialOpenAIOptions(c.openAIClientConfig)
	c.sdk = openai.NewClient(requestOptions...)
}
