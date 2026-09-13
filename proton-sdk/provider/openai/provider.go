// Package openai implements the OpenAI protocol for Protonman SDK language models.
package openai

import (
	"net/http"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if options.BaseURL == "" {
		options.BaseURL = DefaultBaseURL
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{}
	}
	if options.MaxRetries < 0 {
		options.MaxRetries = 0
	}
	if options.RetryBackoff <= 0 {
		options.RetryBackoff = sdk.DefaultRetryBaseBackoff
	}
	if options.MaxRetryBackoff <= 0 {
		options.MaxRetryBackoff = sdk.DefaultRetryMaxBackoff
	}
	if options.MaxRetryAfter <= 0 {
		options.MaxRetryAfter = sdk.DefaultRetryMaxAfter
	}
	options.Headers = options.Headers.Clone()
	options.RetryDelays = append([]time.Duration(nil), options.RetryDelays...)
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
