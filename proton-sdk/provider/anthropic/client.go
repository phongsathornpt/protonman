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

	sdk "github.com/projectTHORN/proton/proton-sdk"
	"github.com/projectTHORN/proton/proton-sdk/internal/providerutil"
)

func (m *LanguageModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	body, err := buildRequest(m.modelID, request, m.provider.options.DefaultMaxTokens)
	if err != nil {
		return nil, err
	}
	encoded, err := providerutil.MarshalWithOptions(body, request.Options.ProviderOptions["anthropic"], "model", "messages", "system", "tools", "max_tokens", "stream")
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}
	endpoint := messagesEndpoint(m.provider.options.BaseURL)
	var lastErr error
	for attempt := 0; attempt <= m.provider.options.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * m.provider.options.RetryBackoff):
			}
		}
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
		resp, err := m.provider.options.HTTPClient.Do(req)
		if err != nil {
			lastErr = sdk.NewTransportError("anthropic", err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return newStream(resp.Body, anthropicResponseMetadata(resp.Header), request.Options.IncludeRawChunks), nil
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		lastErr = anthropicHTTPError(resp.StatusCode, data)
		if !retryableStatus(resp.StatusCode) {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func anthropicResponseMetadata(headers http.Header) sdk.ProviderMetadata {
	requestID := strings.TrimSpace(headers.Get("request-id"))
	if requestID == "" {
		requestID = strings.TrimSpace(headers.Get("x-request-id"))
	}
	if requestID == "" {
		return nil
	}
	raw, err := json.Marshal(map[string]string{"request_id": requestID})
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

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusInternalServerError || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}
func anthropicHTTPError(status int, body []byte) error {
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
