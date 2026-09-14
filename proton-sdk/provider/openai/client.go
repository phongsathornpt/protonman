package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"github.com/phongsathornpt/protonman/proton-sdk/internal/providerutil"
)

func (m *LanguageModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.modelID) == "" {
		return nil, fmt.Errorf("%w: model id is required", sdk.ErrInvalidRequest)
	}
	endpoint, encoded, err := m.encodeRequest(request)
	if err != nil {
		return nil, err
	}
	return providerutil.ExecuteStream(ctx, providerutil.StreamRequest{
		ProviderName: m.Provider(),
		ModelID:      m.modelID,
		Endpoint:     endpoint,
		Payload:      encoded,
		Headers:      m.provider.options.Headers,
		SessionID:    request.Metadata.SessionID,
		HTTPClient:   m.provider.options.HTTPClient,
		MaxRetries:   m.provider.options.MaxRetries,
		RetryPolicy: sdk.RetryPolicy{
			BaseBackoff:       m.provider.options.RetryBackoff,
			PostFirstRetryGap: m.provider.options.RetryPostFirstGap,
			MaxBackoff:        m.provider.options.MaxRetryBackoff,
			MaxRetryAfter:     m.provider.options.MaxRetryAfter,
			RetryDelays:       m.provider.options.RetryDelays,
		},
		PrepareRequest: func(httpReq *http.Request) {
			if m.provider.options.APIKey != "" {
				httpReq.Header.Set("Authorization", "Bearer "+m.provider.options.APIKey)
			}
			if m.provider.options.UserAgent != "" {
				httpReq.Header.Set("User-Agent", m.provider.options.UserAgent)
			}
		},
		ParseError: func(status int, body []byte, headers http.Header) *sdk.ProviderError {
			return providerError(m.Provider(), status, body, headers)
		},
		OnSuccess: func(resp *http.Response) (sdk.Stream, error) {
			return newStream(resp.Body, responseMetadata(m.Provider(), resp.Header), request.Options.IncludeRawChunks, m.Provider()), nil
		},
	})
}

func responseMetadata(provider string, headers http.Header) sdk.ProviderMetadata {
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
	if rateLimit := sdk.ParseRateLimitHeaders(headers, time.Now()); rateLimit != nil {
		values["rate_limit"] = rateLimit
	}
	if len(values) == 0 {
		return nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	return sdk.ProviderMetadata{provider: raw}
}
