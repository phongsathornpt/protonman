package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/proton/proton-sdk"
	"github.com/phongsathornpt/proton/proton-sdk/internal/providerutil"
)

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}
type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type chatMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}
type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}
type chatFunction struct {
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Parameters      map[string]any  `json:"parameters,omitempty"`
	ProviderOptions json.RawMessage `json:"-"`
}

func (f chatFunction) MarshalJSON() ([]byte, error) {
	base := struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	}{f.Name, f.Description, f.Parameters}
	return providerutil.MarshalWithOptions(base, f.ProviderOptions, "name", "description", "parameters")
}

type chatRequest struct {
	Model           string              `json:"model"`
	Messages        []chatMessage       `json:"messages"`
	Stream          bool                `json:"stream"`
	Tools           []chatTool          `json:"tools,omitempty"`
	ToolChoice      string              `json:"tool_choice,omitempty"`
	MaxTokens       int                 `json:"max_tokens,omitempty"`
	ReasoningEffort sdk.ReasoningEffort `json:"reasoning_effort,omitempty"`
}
type responsesTool struct {
	Type            string          `json:"type"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Parameters      map[string]any  `json:"parameters,omitempty"`
	ProviderOptions json.RawMessage `json:"-"`
}

func (t responsesTool) MarshalJSON() ([]byte, error) {
	base := struct {
		Type        string         `json:"type"`
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	}{t.Type, t.Name, t.Description, t.Parameters}
	return providerutil.MarshalWithOptions(base, t.ProviderOptions, "type", "name", "description", "parameters")
}

type reasoningConfig struct {
	Effort sdk.ReasoningEffort `json:"effort"`
}

type responsesRequest struct {
	Model           string           `json:"model"`
	Stream          bool             `json:"stream"`
	Input           []any            `json:"input"`
	Tools           []responsesTool  `json:"tools,omitempty"`
	ToolChoice      string           `json:"tool_choice,omitempty"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Reasoning       *reasoningConfig `json:"reasoning,omitempty"`
}

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
	policy := sdk.RetryPolicy{
		BaseBackoff:   m.provider.options.RetryBackoff,
		MaxBackoff:    m.provider.options.MaxRetryBackoff,
		MaxRetryAfter: m.provider.options.MaxRetryAfter,
	}
	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("create model request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if m.provider.options.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+m.provider.options.APIKey)
		}
		if m.provider.options.UserAgent != "" {
			httpReq.Header.Set("User-Agent", m.provider.options.UserAgent)
		}
		for key, values := range m.provider.options.Headers {
			for _, value := range values {
				httpReq.Header.Add(key, value)
			}
		}

		resp, requestErr := m.provider.options.HTTPClient.Do(httpReq)
		var providerErr error
		if requestErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			providerErr = sdk.NewTransportError(m.Provider(), requestErr)
		} else if resp.StatusCode == http.StatusOK {
			return newStream(resp.Body, responseMetadata(m.Provider(), resp.Header), request.Options.IncludeRawChunks, m.Provider()), nil
		} else {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			providerErr = providerError(m.Provider(), resp.StatusCode, body, resp.Header)
		}
		if attempt >= m.provider.options.MaxRetries {
			return nil, providerErr
		}
		decision := sdk.DecideRetry(providerErr, attempt+1, policy)
		if !decision.Retry {
			return nil, providerErr
		}
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

func (m *LanguageModel) encodeRequest(request sdk.Request) (string, []byte, error) {
	if m.useResponsesAPI || strings.HasSuffix(m.provider.options.BaseURL, "/responses") {
		endpoint := m.provider.options.BaseURL
		if strings.HasSuffix(endpoint, "/chat/completions") {
			endpoint = strings.TrimSuffix(endpoint, "/chat/completions") + "/responses"
		} else if !strings.HasSuffix(endpoint, "/responses") {
			endpoint += "/responses"
		}
		input := make([]any, 0, len(request.Messages))
		for _, message := range request.Messages {
			switch message.Role {
			case sdk.RoleUser, sdk.RoleSystem:
				input = append(input, map[string]any{"role": string(message.Role), "content": message.TextContent()})
			case sdk.RoleAssistant:
				if text := strings.TrimSpace(message.TextContent()); text != "" {
					input = append(input, map[string]any{"role": "assistant", "content": text})
				}
				for _, call := range message.ToolCalls {
					input = append(input, map[string]any{"type": "function_call", "name": call.Name, "call_id": call.ID, "arguments": string(call.Arguments)})
				}
			case sdk.RoleTool:
				input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": message.TextContent()})
			}
		}
		tools := make([]responsesTool, 0, len(request.Tools))
		for _, tool := range request.Tools {
			tools = append(tools, responsesTool{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema, ProviderOptions: tool.ProviderOptions["openai"]})
		}
		encoded, err := providerutil.MarshalWithOptions(responsesRequest{Model: m.modelID, Stream: true, Input: input, Tools: tools, ToolChoice: toolChoice(len(tools), request.Options.ToolChoice), MaxOutputTokens: request.Options.MaxOutputTokens, Reasoning: responseReasoning(request.Options.ReasoningEffort)}, request.Options.ProviderOptions["openai"], "model", "stream", "input", "tools", "tool_choice", "max_output_tokens", "reasoning")
		if err != nil {
			return "", nil, fmt.Errorf("marshal responses request: %w", err)
		}
		return endpoint, encoded, nil
	}

	endpoint := m.provider.options.BaseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	messages := make([]chatMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == sdk.RoleAssistant && len(message.ToolCalls) > 0 {
			if text := strings.TrimSpace(message.TextContent()); text != "" {
				content := text
				messages = append(messages, chatMessage{Role: "assistant", Content: &content})
			}
			calls := make([]chatToolCall, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, chatToolCall{ID: call.ID, Type: "function", Function: chatFunctionCall{Name: call.Name, Arguments: string(call.Arguments)}})
			}
			messages = append(messages, chatMessage{Role: "assistant", ToolCalls: calls})
			continue
		}
		msg := chatMessage{Role: string(message.Role)}
		if len(message.Parts) > 0 {
			msg.Content = chatContentParts(message.Parts)
		} else {
			content := message.Content
			msg.Content = &content
		}
		if message.Role == sdk.RoleTool {
			msg.ToolCallID = message.ToolCallID
		}
		messages = append(messages, msg)
	}
	tools := make([]chatTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, chatTool{Type: "function", Function: chatFunction{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema, ProviderOptions: tool.ProviderOptions["openai"]}})
	}
	encoded, err := providerutil.MarshalWithOptions(chatRequest{Model: m.modelID, Messages: messages, Stream: true, Tools: tools, ToolChoice: toolChoice(len(tools), request.Options.ToolChoice), MaxTokens: request.Options.MaxOutputTokens, ReasoningEffort: request.Options.ReasoningEffort}, request.Options.ProviderOptions["openai"], "model", "messages", "stream", "tools", "tool_choice", "max_tokens", "reasoning_effort")
	if err != nil {
		return "", nil, fmt.Errorf("marshal chat request: %w", err)
	}
	return endpoint, encoded, nil
}

func chatContentParts(parts []sdk.ContentPart) []map[string]any {
	content := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case sdk.ContentPartText:
			if part.Text != "" {
				content = append(content, map[string]any{"type": "text", "text": part.Text})
			}
		case sdk.ContentPartImage:
			mime := part.MIMEType
			if mime == "" {
				mime = "image/png"
			}
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": fmt.Sprintf("data:%s;base64,%s", mime, part.Data)}})
		}
	}
	return content
}

func responseReasoning(effort sdk.ReasoningEffort) *reasoningConfig {
	if effort == sdk.ReasoningDefault {
		return nil
	}
	return &reasoningConfig{Effort: effort}
}

func toolChoice(count int, choice sdk.ToolChoice) string {
	if count == 0 {
		return "none"
	}
	if choice == sdk.ToolChoiceRequired {
		return "required"
	}
	return ""
}
