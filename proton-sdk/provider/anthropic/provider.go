// Package anthropic implements the Anthropic Messages protocol for Protonman SDK.
package anthropic

import (
	"net/http"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"github.com/phongsathornpt/protonman/proton-sdk/internal/providerutil"
)

const (
	DefaultBaseURL    = "https://api.anthropic.com"
	DefaultAPIVersion = "2023-06-01"
	DefaultMaxTokens  = 4096
)

// Config configures the Anthropic Messages protocol provider.
type Config struct {
	BaseURL           string
	APIKey            string
	APIVersion        string
	HTTPClient        *http.Client
	UserAgent         string
	Headers           http.Header
	MaxRetries        int
	RetryBackoff      time.Duration
	RetryPostFirstGap time.Duration
	MaxRetryBackoff   time.Duration
	MaxRetryAfter     time.Duration
	RetryDelays       []time.Duration
	DefaultMaxTokens  int
}

// BaseConfig extracts common provider configuration.
func (c Config) BaseConfig() providerutil.BaseConfig {
	return providerutil.BaseConfig{
		BaseURL:           c.BaseURL,
		APIKey:            c.APIKey,
		HTTPClient:        c.HTTPClient,
		UserAgent:         c.UserAgent,
		Headers:           c.Headers,
		MaxRetries:        c.MaxRetries,
		RetryBackoff:      c.RetryBackoff,
		RetryPostFirstGap: c.RetryPostFirstGap,
		MaxRetryBackoff:   c.MaxRetryBackoff,
		MaxRetryAfter:     c.MaxRetryAfter,
		RetryDelays:       c.RetryDelays,
	}
}

func (c *Config) applyBaseConfig(base providerutil.BaseConfig) {
	c.BaseURL = base.BaseURL
	c.APIKey = base.APIKey
	c.HTTPClient = base.HTTPClient
	c.UserAgent = base.UserAgent
	c.Headers = base.Headers
	c.MaxRetries = base.MaxRetries
	c.RetryBackoff = base.RetryBackoff
	c.RetryPostFirstGap = base.RetryPostFirstGap
	c.MaxRetryBackoff = base.MaxRetryBackoff
	c.MaxRetryAfter = base.MaxRetryAfter
	c.RetryDelays = base.RetryDelays
}

// ProviderOptions is retained as a compatibility alias.
// Deprecated: use Config.
type ProviderOptions = Config

type Provider struct{ options Config }

func NewProvider(options Config) *Provider {
	base := options.BaseConfig()
	base.Normalize(DefaultBaseURL)
	options.applyBaseConfig(base)
	if strings.TrimSpace(options.APIVersion) == "" {
		options.APIVersion = DefaultAPIVersion
	}
	if options.DefaultMaxTokens <= 0 {
		options.DefaultMaxTokens = DefaultMaxTokens
	}
	return &Provider{options: options}
}

func (p *Provider) Model(modelID string) *LanguageModel {
	return &LanguageModel{provider: p, modelID: strings.TrimSpace(modelID)}
}

type LanguageModel struct {
	provider *Provider
	modelID  string
}

var (
	_ sdk.LanguageModel = (*LanguageModel)(nil)
	_ sdk.MetadataModel = (*LanguageModel)(nil)
)

func (m *LanguageModel) Provider() string { return "anthropic" }
func (m *LanguageModel) ModelID() string  { return m.modelID }
func (m *LanguageModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true, Vision: true, ProviderOptions: true, ToolResultErrors: true, RawChunks: true}
}
func (m *LanguageModel) Metadata() sdk.ModelMetadata { return sdk.ModelMetadata{} }
