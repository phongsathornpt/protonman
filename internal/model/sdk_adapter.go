package model

import (
	"context"
	"net/http"
	"strings"

	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"github.com/projectTHORN/proton/internal/tool"
	sdk "github.com/projectTHORN/proton/proton-sdk"
	sdkanthropic "github.com/projectTHORN/proton/proton-sdk/provider/anthropic"
	sdkopenai "github.com/projectTHORN/proton/proton-sdk/provider/openai"
)

type sdkModelClient struct {
	model sdk.LanguageModel
}

var _ Client = (*sdkModelClient)(nil)

func newSDKOpenAIClient(providerName, baseURL, apiKey, modelID string, opts ...ClientOption) Client {
	cfg := newClientConfig(baseURL, apiKey, modelID)
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	headers := make(http.Header)
	if cfg.sessionID != "" {
		headers.Set("x-session-affinity", cfg.sessionID)
		headers.Set("X-Session-Id", cfg.sessionID)
		if IsProvider(DefaultOpenCodeName, providerName, cfg.baseURL) {
			headers.Set("x-opencode-session", cfg.sessionID)
			clientName := cfg.clientName
			if clientName == "" {
				clientName = "proton"
			}
			headers.Set("x-opencode-client", clientName)
		}
	}
	provider := sdkopenai.NewProvider(sdkopenai.ProviderOptions{
		BaseURL:      cfg.baseURL,
		APIKey:       cfg.apiKey,
		HTTPClient:   cfg.httpClient,
		UserAgent:    cfg.userAgent,
		Headers:      headers,
		MaxRetries:   2,
		RetryBackoff: runtimepolicy.ModelRetryBackoffStep,
	})
	modelOptions := make([]sdkopenai.ModelOption, 0, 1)
	if usesResponsesAPI(cfg.modelID, cfg.baseURL) {
		modelOptions = append(modelOptions, sdkopenai.WithResponsesAPI())
	}
	return &sdkModelClient{model: provider.Model(cfg.modelID, modelOptions...)}
}

func newSDKAnthropicClient(baseURL, apiKey, modelID string, opts ...ClientOption) Client {
	cfg := newClientConfig(baseURL, apiKey, modelID)
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	provider := sdkanthropic.NewProvider(sdkanthropic.ProviderOptions{
		BaseURL:      cfg.baseURL,
		APIKey:       cfg.apiKey,
		HTTPClient:   cfg.httpClient,
		UserAgent:    cfg.userAgent,
		MaxRetries:   2,
		RetryBackoff: runtimepolicy.ModelRetryBackoffStep,
	})
	return &sdkModelClient{model: provider.Model(cfg.modelID)}
}

func (c *sdkModelClient) Stream(ctx context.Context, request Request) (Stream, error) {
	sdkRequest := sdk.Request{
		Messages: make([]sdk.Message, 0, len(request.Messages)),
		Tools:    make([]sdk.Tool, 0, len(request.Tools)),
	}
	for _, message := range request.Messages {
		sdkRequest.Messages = append(sdkRequest.Messages, toSDKMessage(message))
	}
	for _, definition := range request.Tools {
		sdkRequest.Tools = append(sdkRequest.Tools, toSDKTool(definition))
	}
	stream, err := c.model.Stream(ctx, sdkRequest)
	if err != nil {
		return nil, err
	}
	return &sdkStreamAdapter{stream: stream}, nil
}

func toSDKMessage(message Message) sdk.Message {
	converted := sdk.Message{
		Role:       sdk.Role(message.Role),
		Content:    message.Content,
		ToolCallID: message.ToolCallID,
		ToolName:   message.ToolName,
		Parts:      make([]sdk.ContentPart, 0, len(message.Parts)),
		ToolCalls:  make([]sdk.ToolCall, 0, len(message.ToolCalls)),
	}
	for _, part := range message.Parts {
		converted.Parts = append(converted.Parts, sdk.ContentPart{Type: sdk.ContentPartType(part.Type), Text: part.Text, MIMEType: part.MIMEType, Data: part.Data})
	}
	for _, call := range message.ToolCalls {
		converted.ToolCalls = append(converted.ToolCalls, sdk.ToolCall{ID: call.ID, Name: call.Name, Arguments: append([]byte(nil), call.Arguments...)})
	}
	return converted
}

func toSDKTool(definition tool.Definition) sdk.Tool {
	return sdk.Tool{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema}
}

type sdkStreamAdapter struct {
	stream sdk.Stream
}

func (s *sdkStreamAdapter) Next(ctx context.Context) (Event, error) {
	for {
		event, err := s.stream.Next(ctx)
		if err != nil {
			return Event{}, err
		}
		switch event.Kind {
		case sdk.EventTextDelta:
			return Event{Kind: EventTextDelta, Text: event.Text}, nil
		case sdk.EventToolCall:
			return Event{Kind: EventToolCall, ToolCall: ToolCall{ID: event.ToolCall.ID, Name: event.ToolCall.Name, Arguments: append([]byte(nil), event.ToolCall.Arguments...)}}, nil
		case sdk.EventFinish, sdk.EventDone:
			return Event{Kind: EventDone}, nil
		default:
			// The legacy agent boundary consumes complete text/tool/done events.
			// Incremental lifecycle and usage events stay available to direct SDK consumers.
			continue
		}
	}
}

func (s *sdkStreamAdapter) Close() error { return s.stream.Close() }

func usesResponsesAPI(modelID, baseURL string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	return strings.HasPrefix(id, "muse-spark") || strings.Contains(id, "responses") || strings.HasSuffix(strings.TrimSpace(baseURL), "/responses")
}
