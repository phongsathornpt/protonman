// Package openai implements the OpenAI protocol for Proton SDK language models.
package openai

import (
	"net/http"
	"strings"
	"time"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

const DefaultBaseURL = "https://api.openai.com/v1"

type ProviderOptions struct {
	BaseURL      string
	APIKey       string
	HTTPClient   *http.Client
	UserAgent    string
	Headers      http.Header
	MaxRetries   int
	RetryBackoff time.Duration
}

type Provider struct {
	options ProviderOptions
}

func NewProvider(options ProviderOptions) *Provider {
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
		options.RetryBackoff = 500 * time.Millisecond
	}
	options.Headers = options.Headers.Clone()
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

var _ sdk.LanguageModel = (*LanguageModel)(nil)

func (m *LanguageModel) Provider() string { return "openai" }
func (m *LanguageModel) ModelID() string  { return m.modelID }
