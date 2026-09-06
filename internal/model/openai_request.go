package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type openAIToolCallReq struct {
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function openAIFunctionCallReq `json:"function"`
}

type openAIFunctionCallReq struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatMessage struct {
	Role       string              `json:"role"`
	Content    any                 `json:"content,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCallReq `json:"tool_calls,omitempty"`
}

type openAIToolDef struct {
	Type     string            `json:"type"`
	Function openAIFunctionDef `json:"function"`
}

type openAIFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIChatRequest struct {
	Model      string              `json:"model"`
	Messages   []openAIChatMessage `json:"messages"`
	Stream     bool                `json:"stream"`
	Tools      []openAIToolDef     `json:"tools,omitempty"`
	ToolChoice string              `json:"tool_choice,omitempty"`
}

type openAIResponsesToolDef struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIResponsesRequest struct {
	Model      string                   `json:"model"`
	Stream     bool                     `json:"stream"`
	Input      []any                    `json:"input"`
	Tools      []openAIResponsesToolDef `json:"tools,omitempty"`
	ToolChoice string                   `json:"tool_choice,omitempty"`
}

type openAIResponsesChunk struct {
	Type      string               `json:"type"`
	Delta     string               `json:"delta,omitempty"`
	Item      *openAIResponsesItem `json:"item,omitempty"`
	ItemID    string               `json:"item_id,omitempty"`
	Arguments string               `json:"arguments,omitempty"`
	Response  *struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type openAIResponsesItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "message", "function_call", "reasoning"
	Role      string `json:"role,omitempty"`
	Name      string `json:"name,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func isResponsesModel(modelID string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	return strings.HasPrefix(id, "muse-spark") || strings.Contains(id, "responses")
}

// Stream initiates a server-sent events stream for the conversation request.
func (c *OpenAIClient) Stream(ctx context.Context, request Request) (Stream, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	var endpoint string
	var encoded []byte

	if isResponsesModel(c.modelID) || strings.HasSuffix(c.baseURL, "/responses") {
		endpoint = c.baseURL
		if strings.HasSuffix(endpoint, "/chat/completions") {
			endpoint = strings.TrimSuffix(endpoint, "/chat/completions") + "/responses"
		} else if !strings.HasSuffix(endpoint, "/responses") {
			endpoint += "/responses"
		}

		input := make([]any, 0, len(request.Messages))
		for _, m := range request.Messages {
			switch m.Role {
			case RoleUser:
				input = append(input, map[string]any{
					"role":    "user",
					"content": m.Content,
				})
			case RoleSystem:
				input = append(input, map[string]any{
					"role":    "system",
					"content": m.Content,
				})
			case RoleAssistant:
				if strings.TrimSpace(m.Content) != "" {
					input = append(input, map[string]any{
						"role":    "assistant",
						"content": m.Content,
					})
				}
				for _, tc := range m.ToolCalls {
					input = append(input, map[string]any{
						"type":      "function_call",
						"name":      tc.Name,
						"call_id":   tc.ID,
						"arguments": string(tc.Arguments),
					})
				}
			case RoleTool:
				input = append(input, map[string]any{
					"type":    "function_call_output",
					"call_id": m.ToolCallID,
					"output":  m.Content,
				})
			}
		}

		var tools []openAIResponsesToolDef
		if len(request.Tools) > 0 {
			tools = make([]openAIResponsesToolDef, 0, len(request.Tools))
			for _, t := range request.Tools {
				tools = append(tools, openAIResponsesToolDef{
					Type:        "function",
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.InputSchema,
				})
			}
		}

		reqBody := openAIResponsesRequest{
			Model:      c.modelID,
			Stream:     true,
			Input:      input,
			Tools:      tools,
			ToolChoice: noToolChoice(len(request.Tools)),
		}

		var err error
		encoded, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal responses request: %w", err)
		}
	} else {
		endpoint = c.baseURL
		if !strings.HasSuffix(endpoint, "/chat/completions") {
			endpoint += "/chat/completions"
		}

		messages := make([]openAIChatMessage, 0, len(request.Messages))
		for _, m := range request.Messages {
			if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
				// If conversational text was emitted alongside tool calls, separate into two messages.
				// Upstream providers (such as Ling/01.AI via OpenCode/OpenRouter) reject assistant messages
				// combining both non-empty content and tool_calls with 503 "Upstream request failed".
				if strings.TrimSpace(m.Content) != "" {
					text := m.Content
					messages = append(messages, openAIChatMessage{
						Role:    "assistant",
						Content: &text,
					})
				}
				toolCalls := make([]openAIToolCallReq, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					toolCalls = append(toolCalls, openAIToolCallReq{
						ID:   tc.ID,
						Type: "function",
						Function: openAIFunctionCallReq{
							Name:      tc.Name,
							Arguments: string(tc.Arguments),
						},
					})
				}
				messages = append(messages, openAIChatMessage{
					Role:      "assistant",
					ToolCalls: toolCalls,
				})
				continue
			}

			msg := openAIChatMessage{
				Role: string(m.Role),
			}
			if len(m.Parts) > 0 {
				parts := make([]map[string]any, 0, len(m.Parts))
				for _, part := range m.Parts {
					switch part.Type {
					case ContentPartText:
						if part.Text != "" {
							parts = append(parts, map[string]any{
								"type": "text",
								"text": part.Text,
							})
						}
					case ContentPartImage:
						mime := part.MIMEType
						if mime == "" {
							mime = "image/png"
						}
						parts = append(parts, map[string]any{
							"type": "image_url",
							"image_url": map[string]any{
								"url": fmt.Sprintf("data:%s;base64,%s", mime, part.Data),
							},
						})
					}
				}
				msg.Content = parts
			} else {
				content := m.Content
				msg.Content = &content
			}
			if m.Role == RoleTool {
				msg.ToolCallID = m.ToolCallID
			}
			messages = append(messages, msg)
		}

		var tools []openAIToolDef
		if len(request.Tools) > 0 {
			tools = make([]openAIToolDef, 0, len(request.Tools))
			for _, t := range request.Tools {
				tools = append(tools, openAIToolDef{
					Type: "function",
					Function: openAIFunctionDef{
						Name:        t.Name,
						Description: t.Description,
						Parameters:  t.InputSchema,
					},
				})
			}
		}

		reqBody := openAIChatRequest{
			Model:      c.modelID,
			Messages:   messages,
			Stream:     true,
			Tools:      tools,
			ToolChoice: noToolChoice(len(request.Tools)),
		}

		var err error
		encoded, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal chat request: %w", err)
		}
	}

	maxRetries := 2
	var lastErr error
	slog.DebugContext(ctx, "model stream request started",
		"model", c.modelID,
		"message_count", len(request.Messages),
		"tool_count", len(request.Tools),
		"request_bytes", len(encoded),
	)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		slog.DebugContext(ctx, "model stream request attempt",
			"attempt", attempt+1,
			"max_attempts", maxRetries+1,
		)
		if attempt > 0 {
			backoff := time.Duration(attempt*500) * time.Millisecond
			select {
			case <-ctx.Done():
				slog.DebugContext(ctx, "model stream retry cancelled", "attempt", attempt+1)
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("create chat http request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if c.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		}

		ua := c.userAgent
		if ua == "" {
			ua = "Proton/1.0"
		}
		httpReq.Header.Set("User-Agent", ua)

		if c.sessionID != "" {
			httpReq.Header.Set("x-session-affinity", c.sessionID)
			httpReq.Header.Set("X-Session-Id", c.sessionID)

			if IsProvider(DefaultOpenCodeName, "", c.baseURL) {
				httpReq.Header.Set("x-opencode-session", c.sessionID)
				clientName := c.clientName
				if clientName == "" {
					clientName = "proton"
				}
				httpReq.Header.Set("x-opencode-client", clientName)
			}
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			slog.DebugContext(ctx, "model stream request failed",
				"attempt", attempt+1,
				"error_type", fmt.Sprintf("%T", err),
			)
			lastErr = fmt.Errorf("execute chat http request: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			slog.DebugContext(ctx, "model stream response opened",
				"attempt", attempt+1,
				"status", resp.StatusCode,
			)
			return newOpenAIStream(resp.Body), nil
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()

		lastErr = fmt.Errorf("provider returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))

		// Retry only on transient upstream errors
		retryable := resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusInternalServerError
		slog.DebugContext(ctx, "model stream response rejected",
			"attempt", attempt+1,
			"status", resp.StatusCode,
			"body_bytes", len(body),
			"retryable", retryable,
		)
		if retryable {
			continue
		}

		return nil, lastErr
	}

	slog.DebugContext(ctx, "model stream request exhausted",
		"attempts", maxRetries+1,
		"error_type", fmt.Sprintf("%T", lastErr),
	)
	return nil, lastErr
}

func noToolChoice(toolCount int) string {
	if toolCount == 0 {
		return "none"
	}
	return ""
}
