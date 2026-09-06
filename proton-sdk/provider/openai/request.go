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

	sdk "github.com/projectTHORN/proton/proton-sdk"
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
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}
type chatRequest struct {
	Model      string        `json:"model"`
	Messages   []chatMessage `json:"messages"`
	Stream     bool          `json:"stream"`
	Tools      []chatTool    `json:"tools,omitempty"`
	ToolChoice string        `json:"tool_choice,omitempty"`
}
type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}
type responsesRequest struct {
	Model      string          `json:"model"`
	Stream     bool            `json:"stream"`
	Input      []any           `json:"input"`
	Tools      []responsesTool `json:"tools,omitempty"`
	ToolChoice string          `json:"tool_choice,omitempty"`
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

	var lastErr error
	for attempt := 0; attempt <= m.provider.options.MaxRetries; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * m.provider.options.RetryBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
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

		resp, err := m.provider.options.HTTPClient.Do(httpReq)
		if err != nil {
			lastErr = sdk.NewTransportError("openai", err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return newStream(resp.Body), nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()
		lastErr = openAIHTTPError(resp.StatusCode, body)
		if !retryableStatus(resp.StatusCode) {
			return nil, lastErr
		}
	}
	return nil, lastErr
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
			tools = append(tools, responsesTool{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema})
		}
		encoded, err := json.Marshal(responsesRequest{Model: m.modelID, Stream: true, Input: input, Tools: tools, ToolChoice: toolChoice(len(tools))})
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
		tools = append(tools, chatTool{Type: "function", Function: chatFunction{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema}})
	}
	encoded, err := json.Marshal(chatRequest{Model: m.modelID, Messages: messages, Stream: true, Tools: tools, ToolChoice: toolChoice(len(tools))})
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

func toolChoice(count int) string {
	if count == 0 {
		return "none"
	}
	return ""
}
func retryableStatus(status int) bool {
	return status == 429 || status == 500 || status == 502 || status == 503 || status == 504
}
func openAIHTTPError(status int, body []byte) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	code := ""
	if json.Unmarshal(body, &payload) == nil {
		if strings.TrimSpace(payload.Error.Message) != "" {
			message = payload.Error.Message
		}
		if payload.Error.Code != nil {
			code = fmt.Sprint(payload.Error.Code)
		} else {
			code = payload.Error.Type
		}
	}
	return sdk.NewProviderError("openai", status, code, message)
}
