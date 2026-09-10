// Package anthropic implements the Anthropic Messages protocol for Protonman SDK.
package anthropic

import (
	"net/http"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const (
	DefaultBaseURL    = "https://api.anthropic.com"
	DefaultAPIVersion = "2023-06-01"
	DefaultMaxTokens  = 4096
)

type ProviderOptions struct {
	BaseURL          string
	APIKey           string
	APIVersion       string
	HTTPClient       *http.Client
	UserAgent        string
	Headers          http.Header
	MaxRetries       int
	RetryBackoff     time.Duration
	MaxRetryBackoff  time.Duration
	MaxRetryAfter    time.Duration
	DefaultMaxTokens int
}

type Provider struct{ options ProviderOptions }

func NewProvider(options ProviderOptions) *Provider {
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if options.BaseURL == "" {
		options.BaseURL = DefaultBaseURL
	}
	if strings.TrimSpace(options.APIVersion) == "" {
		options.APIVersion = DefaultAPIVersion
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{}
	}
	if options.MaxRetries < 0 {
		options.MaxRetries = 0
	}
	if options.RetryBackoff <= 0 {
		options.RetryBackoff = 500 * time.Millisecond
	}
	if options.MaxRetryBackoff <= 0 {
		options.MaxRetryBackoff = 8 * time.Second
	}
	if options.MaxRetryAfter <= 0 {
		options.MaxRetryAfter = 30 * time.Second
	}
	if options.DefaultMaxTokens <= 0 {
		options.DefaultMaxTokens = DefaultMaxTokens
	}
	options.Headers = options.Headers.Clone()
	return &Provider{options: options}
}

func (p *Provider) Model(modelID string) *LanguageModel {
	return &LanguageModel{provider: p, modelID: strings.TrimSpace(modelID)}
}

type LanguageModel struct {
	provider *Provider
	modelID  string
}

var _ sdk.LanguageModel = (*LanguageModel)(nil)

func (m *LanguageModel) Provider() string { return "anthropic" }
func (m *LanguageModel) ModelID() string  { return m.modelID }
func (m *LanguageModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true, Vision: true, ProviderOptions: true, ToolResultErrors: true, RawChunks: true}
}
