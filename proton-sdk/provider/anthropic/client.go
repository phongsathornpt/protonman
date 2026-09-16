package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/internal/providerutil"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

func (m *LanguageModel) Stream(ctx context.Context, request domain.Request) (port.Stream, error) {
	body, err := buildRequest(m.modelID, request, m.provider.options.DefaultMaxTokens)
	if err != nil {
		return nil, err
	}
	encoded, err := providerutil.MarshalWithOptions(body, request.Options.ProviderOptions["anthropic"], "model", "messages", "system", "tools", "max_tokens", "thinking", "output_config", "stream")
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}
	base := m.provider.options.BaseConfig()
	endpoint := messagesEndpoint(base.BaseURL)
	return providerutil.ExecuteStream(ctx, providerutil.StreamRequest{
		ProviderName: m.Provider(),
		ModelID:      m.modelID,
		Endpoint:     endpoint,
		Payload:      encoded,
		Headers:      base.Headers,
		SessionID:    request.Metadata.SessionID,
		HTTPClient:   base.HTTPClient,
		MaxRetries:   base.MaxRetries,
		RetryPolicy:  base.RetryPolicy(),
		PrepareRequest: func(httpReq *http.Request) {
			httpReq.Header.Set("anthropic-version", m.provider.options.APIVersion)
			if base.APIKey != "" {
				httpReq.Header.Set("x-api-key", base.APIKey)
			}
			if base.UserAgent != "" {
				httpReq.Header.Set("User-Agent", base.UserAgent)
			}
		},
		ParseError: func(status int, body []byte, headers http.Header) *domain.ProviderError {
			err := anthropicHTTPError(status, body)
			err.RateLimit = providerutil.ParseRateLimitHeaders(headers, time.Now())
			return err
		},
		OnSuccess: func(resp *http.Response) (port.Stream, error) {
			return newStream(resp.Body, anthropicResponseMetadata(resp.Header), request.Options.IncludeRawChunks), nil
		},
	})
}

func anthropicResponseMetadata(headers http.Header) domain.ProviderMetadata {
	values := map[string]any{}
	requestID := strings.TrimSpace(headers.Get("request-id"))
	if requestID == "" {
		requestID = strings.TrimSpace(headers.Get("x-request-id"))
	}
	if requestID != "" {
		values["request_id"] = requestID
	}
	if rateLimit := providerutil.ParseRateLimitHeaders(headers, time.Now()); rateLimit != nil {
		values["rate_limit"] = rateLimit
	}
	if len(values) == 0 {
		return nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	return domain.ProviderMetadata{"anthropic": raw}
}

func messagesEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/messages") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/messages"
	}
	return baseURL + "/v1/messages"
}
