package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"github.com/phongsathornpt/protonman/proton-sdk/internal/providerutil"
)

func (m *LanguageModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	body, err := buildRequest(m.modelID, request, m.provider.options.DefaultMaxTokens)
	if err != nil {
		return nil, err
	}
	encoded, err := providerutil.MarshalWithOptions(body, request.Options.ProviderOptions["anthropic"], "model", "messages", "system", "tools", "max_tokens", "thinking", "output_config", "stream")
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}
	endpoint := messagesEndpoint(m.provider.options.BaseURL)
	policy := sdk.RetryPolicy{
		BaseBackoff:   m.provider.options.RetryBackoff,
		MaxBackoff:    m.provider.options.MaxRetryBackoff,
		MaxRetryAfter: m.provider.options.MaxRetryAfter,
	}
	var lastErr error
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("create anthropic request: %w", err)
		}
		for key, values := range m.provider.options.Headers {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("anthropic-version", m.provider.options.APIVersion)
		if m.provider.options.APIKey != "" {
			req.Header.Set("x-api-key", m.provider.options.APIKey)
		}
		if m.provider.options.UserAgent != "" {
			req.Header.Set("User-Agent", m.provider.options.UserAgent)
		}
		providerutil.ApplySessionID(req.Header, request.Metadata.SessionID)
		resp, err := m.provider.options.HTTPClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = sdk.NewTransportError("anthropic", err)
		} else if resp.StatusCode == http.StatusOK {
			return newStream(resp.Body, anthropicResponseMetadata(resp.Header), request.Options.IncludeRawChunks), nil
		} else {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			providerErr := anthropicHTTPError(resp.StatusCode, data)
			providerErr.RateLimit = sdk.ParseRateLimitHeaders(resp.Header, time.Now())
			lastErr = providerErr
		}
		if attempt >= m.provider.options.MaxRetries {
			return nil, lastErr
		}
		decision := sdk.DecideRetry(lastErr, attempt+1, policy)
		if !decision.Retry {
			return nil, lastErr
		}
		sdk.ObserveRetry(ctx, sdk.RetryEvent{
			Provider: m.Provider(), ModelID: m.modelID, Reason: string(decision.Reason),
			Attempt: attempt + 1, MaxRetries: m.provider.options.MaxRetries, Delay: decision.Delay,
		})
		if err := waitForRetry(ctx, decision.Delay); err != nil {
			return nil, err
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func anthropicResponseMetadata(headers http.Header) sdk.ProviderMetadata {
	values := map[string]any{}
	requestID := strings.TrimSpace(headers.Get("request-id"))
	if requestID == "" {
		requestID = strings.TrimSpace(headers.Get("x-request-id"))
	}
	if requestID != "" {
		values["request_id"] = requestID
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
	return sdk.ProviderMetadata{"anthropic": raw}
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

func anthropicHTTPError(status int, body []byte) *sdk.ProviderError {
	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	code := ""
	if json.Unmarshal(body, &payload) == nil {
		code = payload.Error.Type
		if strings.TrimSpace(payload.Error.Message) != "" {
			message = payload.Error.Message
		}
	}
	return sdk.NewProviderError("anthropic", status, code, message)
}
