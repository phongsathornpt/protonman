package providerutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// StreamRequest configures an outbound model streaming HTTP request with retry semantics.
type StreamRequest struct {
	ProviderName   string
	ModelID        string
	Endpoint       string
	Payload        []byte
	Headers        http.Header
	SessionID      string
	HTTPClient     *http.Client
	RetryPolicy    sdk.RetryPolicy
	MaxRetries     int
	PrepareRequest func(req *http.Request)
	ParseError     func(status int, body []byte, headers http.Header) *sdk.ProviderError
	OnSuccess      func(resp *http.Response) (sdk.Stream, error)
}

// ExecuteStream performs the HTTP request with bounded retry and exponential backoff.
func ExecuteStream(ctx context.Context, req StreamRequest) (sdk.Stream, error) {
	if req.HTTPClient == nil {
		req.HTTPClient = &http.Client{}
	}
	var lastErr error
	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Endpoint, bytes.NewReader(req.Payload))
		if err != nil {
			return nil, fmt.Errorf("create %s request: %w", req.ProviderName, err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		for key, values := range req.Headers {
			for _, value := range values {
				httpReq.Header.Add(key, value)
			}
		}
		ApplySessionID(httpReq.Header, req.SessionID)
		if req.PrepareRequest != nil {
			req.PrepareRequest(httpReq)
		}

		resp, requestErr := req.HTTPClient.Do(httpReq)
		if requestErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = sdk.NewTransportError(req.ProviderName, requestErr)
		} else {
			if resp.StatusCode == http.StatusOK {
				return req.OnSuccess(resp)
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			if req.ParseError != nil {
				lastErr = req.ParseError(resp.StatusCode, body, resp.Header)
			} else {
				lastErr = sdk.NewProviderError(req.ProviderName, resp.StatusCode, "", string(body))
			}
		}

		if attempt >= req.MaxRetries {
			return nil, lastErr
		}
		decision := sdk.DecideRetry(lastErr, attempt+1, req.RetryPolicy)
		if !decision.Retry {
			return nil, lastErr
		}
		sdk.ObserveRetry(ctx, sdk.RetryEvent{
			Provider:   req.ProviderName,
			ModelID:    req.ModelID,
			Reason:     string(decision.Reason),
			Attempt:    attempt + 1,
			MaxRetries: req.MaxRetries,
			Delay:      decision.Delay,
		})
		if err := sdk.WaitForRetry(ctx, decision.Delay); err != nil {
			return nil, err
		}
	}
}
