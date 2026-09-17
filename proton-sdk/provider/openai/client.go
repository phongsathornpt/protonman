package openai

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
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.modelID) == "" {
		return nil, fmt.Errorf("%w: model id is required", domain.ErrInvalidRequest)
	}
	endpoint, encoded, err := m.encodeRequest(request)
	if err != nil {
		return nil, err
	}
	base := m.provider.options.BaseConfig()
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
			if base.APIKey != "" {
				httpReq.Header.Set("Authorization", "Bearer "+base.APIKey)
			}
			if base.UserAgent != "" {
				httpReq.Header.Set("User-Agent", base.UserAgent)
			}
			if strings.EqualFold(strings.TrimSpace(m.Provider()), "opencode") {
				applyOpenCodeRequestHeaders(httpReq.Header, request)
			}
		},
		ParseError: func(status int, body []byte, headers http.Header) *domain.ProviderError {
			return providerError(m.Provider(), status, body, headers)
		},
		OnSuccess: func(resp *http.Response) (port.Stream, error) {
			return newStream(resp.Body, responseMetadata(m.Provider(), resp.Header), request.Options.IncludeRawChunks, m.Provider()), nil
		},
	})
}

func applyOpenCodeRequestHeaders(headers http.Header, request domain.Request) {
	if headers == nil {
		return
	}
	metadata := request.Metadata
	if value := strings.TrimSpace(metadata.SessionID); value != "" {
		headers.Set("x-opencode-session", value)
	}
	if value := strings.TrimSpace(metadata.ProjectID); value != "" {
		headers.Set("x-opencode-project", value)
	}
	if value := strings.TrimSpace(request.EffectiveRequestID()); value != "" {
		headers.Set("x-opencode-request", value)
	}
	if value := strings.TrimSpace(metadata.ParentSessionID); value != "" {
		headers.Set("x-parent-session-id", value)
	}
}

func responseMetadata(provider string, headers http.Header) domain.ProviderMetadata {
	values := map[string]any{}
	for key, header := range map[string]string{
		"request_id":    "x-request-id",
		"organization":  "openai-organization",
		"project":       "openai-project",
		"processing_ms": "openai-processing-ms",
	} {
		if value := strings.TrimSpace(headers.Get(header)); value != "" {
			values[key] = value
		}
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
	return domain.ProviderMetadata{provider: raw}
}
