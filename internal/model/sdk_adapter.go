package model

import (
	"net/http"
	"strings"

	"github.com/projectTHORN/proton/internal/runtimepolicy"
	sdk "github.com/projectTHORN/proton/proton-sdk"
	sdkanthropic "github.com/projectTHORN/proton/proton-sdk/provider/anthropic"
	sdkopenai "github.com/projectTHORN/proton/proton-sdk/provider/openai"
)

func newSDKOpenAILanguageModel(providerName, baseURL, apiKey, modelID string, opts ...ClientOption) sdk.LanguageModel {
	cfg := newClientConfig(baseURL, apiKey, modelID)
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	headers := make(http.Header)
	isOpenCode := IsProvider(DefaultOpenCodeName, providerName, cfg.baseURL)
	if cfg.sessionID != "" {
		headers.Set("x-session-affinity", cfg.sessionID)
		headers.Set("X-Session-Id", cfg.sessionID)
		if isOpenCode {
			headers.Set("x-opencode-session", cfg.sessionID)
		}
	}
	if isOpenCode {
		clientName := cfg.clientName
		if clientName == "" {
			clientName = "proton"
		}
		headers.Set("x-opencode-client", clientName)
	}
	provider := sdkopenai.NewProvider(sdkopenai.ProviderOptions{
		ProviderName: strings.ToLower(strings.TrimSpace(providerName)),
		BaseURL:      cfg.baseURL, APIKey: cfg.apiKey, HTTPClient: cfg.httpClient,
		UserAgent: cfg.userAgent, Headers: headers, MaxRetries: 2, RetryBackoff: runtimepolicy.ModelRetryBackoffStep,
	})
	modelOptions := make([]sdkopenai.ModelOption, 0, 1)
	if usesResponsesAPI(cfg.modelID, cfg.baseURL) {
		modelOptions = append(modelOptions, sdkopenai.WithResponsesAPI())
	}
	var model sdk.LanguageModel = provider.Model(cfg.modelID, modelOptions...)
	if cfg.vision != nil {
		model = withVisionCapability(model, *cfg.vision)
	}
	if cfg.tools != nil {
		model = withToolsCapability(model, *cfg.tools)
	}
	if cfg.tokenLimits != nil {
		model = withTokenLimits(model, *cfg.tokenLimits)
	}
	if cfg.contextWindow != nil {
		model = withContextWindow(model, *cfg.contextWindow)
	}
	return withModelProfile(model, cfg.profile)
}

func newSDKAnthropicLanguageModel(baseURL, apiKey, modelID string, opts ...ClientOption) sdk.LanguageModel {
	cfg := newClientConfig(baseURL, apiKey, modelID)
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	provider := sdkanthropic.NewProvider(sdkanthropic.ProviderOptions{
		BaseURL: cfg.baseURL, APIKey: cfg.apiKey, HTTPClient: cfg.httpClient,
		UserAgent: cfg.userAgent, MaxRetries: 2, RetryBackoff: runtimepolicy.ModelRetryBackoffStep,
	})
	var model sdk.LanguageModel = provider.Model(cfg.modelID)
	if cfg.vision != nil {
		model = withVisionCapability(model, *cfg.vision)
	}
	if cfg.tools != nil {
		model = withToolsCapability(model, *cfg.tools)
	}
	if cfg.tokenLimits != nil {
		model = withTokenLimits(model, *cfg.tokenLimits)
	}
	if cfg.contextWindow != nil {
		model = withContextWindow(model, *cfg.contextWindow)
	}
	return withModelProfile(model, cfg.profile)
}

func usesResponsesAPI(modelID, baseURL string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	return strings.HasPrefix(id, "muse-spark") || strings.Contains(id, "responses") || strings.HasSuffix(strings.TrimSpace(baseURL), "/responses")
}
