// Package openai implements the OpenAI protocol for Protonman SDK language models.
package openai

import (
	"net/http"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"github.com/phongsathornpt/protonman/proton-sdk/internal/providerutil"
)

const DefaultBaseURL = "https://api.openai.com/v1"

// Config configures the OpenAI protocol provider.
type Config struct {
	ProviderName      string
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

type Provider struct {
	options Config
}

func NewProvider(options Config) *Provider {
	options.ProviderName = strings.ToLower(strings.TrimSpace(options.ProviderName))
	if options.ProviderName == "" {
		options.ProviderName = "openai"
	}
	base := options.BaseConfig()
	base.Normalize(DefaultBaseURL)
	options.applyBaseConfig(base)
	return &Provider{options: options}
}

type ModelOption func(*LanguageModel)

func WithResponsesAPI() ModelOption {
	return func(model *LanguageModel) { model.useResponsesAPI = true }
}

func (p *Provider) Model(modelID string, options ...ModelOption) *LanguageModel {
	model := &LanguageModel{provider: p, modelID: strings.TrimSpace(modelID)}
	for _, option := range options {
		if option != nil {
			option(model)
		}
	}
	return model
}

type LanguageModel struct {
	provider        *Provider
	modelID         string
	useResponsesAPI bool
}

var (
	_ sdk.LanguageModel = (*LanguageModel)(nil)
	_ sdk.MetadataModel = (*LanguageModel)(nil)
)

func (m *LanguageModel) Provider() string { return m.provider.options.ProviderName }
func (m *LanguageModel) ModelID() string  { return m.modelID }
func (m *LanguageModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true, Vision: true, ProviderOptions: true, RawChunks: true}
}
func (m *LanguageModel) Metadata() sdk.ModelMetadata { return sdk.ModelMetadata{} }
