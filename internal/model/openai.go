package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OpenAIClient streams chat completions from an OpenAI-compatible endpoint.
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	modelID    string
	sessionID  string
	clientName string
	userAgent  string
	httpClient *http.Client
}

var _ Client = (*OpenAIClient)(nil)

// OpenAIOption configures an OpenAIClient.
type OpenAIOption func(*OpenAIClient)

// WithSessionID sets the session identifier for sticky routing and prompt cache optimization.
func WithSessionID(sessionID string) OpenAIOption {
	return func(c *OpenAIClient) {
		c.sessionID = sessionID
	}
}

// WithClientName sets the client identifier (e.g. "proton").
func WithClientName(clientName string) OpenAIOption {
	return func(c *OpenAIClient) {
		c.clientName = clientName
	}
}

// WithUserAgent sets a custom User-Agent header (defaults to "Proton/1.0").
func WithUserAgent(userAgent string) OpenAIOption {
	return func(c *OpenAIClient) {
		c.userAgent = userAgent
	}
}

// NewOpenAIClient creates a client targeting an OpenAI, OpenCode, or Protonman chat API.
func NewOpenAIClient(baseURL string, apiKey string, modelID string, opts ...OpenAIOption) *OpenAIClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultProtonmanEndpoint
	}
	client := &OpenAIClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		modelID:    modelID,
		clientName: "proton",
		userAgent:  "Proton/1.0",
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	return client
}

// SessionID returns the configured session identifier.
func (c *OpenAIClient) SessionID() string {
	return c.sessionID
}

// SetSessionID updates the session identifier on an existing client.
func (c *OpenAIClient) SetSessionID(sessionID string) {
	c.sessionID = sessionID
}

type openAIToolCallReq struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openAIFunctionCallReq  `json:"function"`
}

type openAIFunctionCallReq struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatMessage struct {
	Role       string              `json:"role"`
	Content    *string             `json:"content,omitempty"`
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
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Tools    []openAIToolDef     `json:"tools,omitempty"`
}

// Stream initiates a server-sent events stream for the conversation request.
func (c *OpenAIClient) Stream(ctx context.Context, request Request) (Stream, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
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

		content := m.Content
		msg := openAIChatMessage{
			Role:    string(m.Role),
			Content: &content,
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
		Model:    c.modelID,
		Messages: messages,
		Stream:   true,
		Tools:    tools,
	}

	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	endpoint := c.baseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	maxRetries := 2
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*500) * time.Millisecond
			select {
			case <-ctx.Done():
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

			if strings.Contains(strings.ToLower(c.baseURL), "opencode.ai") {
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
			lastErr = fmt.Errorf("execute chat http request: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return newOpenAIStream(resp.Body), nil
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()

		lastErr = fmt.Errorf("provider returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))

		// Retry only on transient upstream errors
		if resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusInternalServerError {
			continue
		}

		return nil, lastErr
	}

	return nil, lastErr
}

type accumulatedToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

type openAIStream struct {
	reader    *bufio.Reader
	closer    io.Closer
	toolCalls map[int]*accumulatedToolCall
	queue     []Event
	done      bool
}

func newOpenAIStream(r io.ReadCloser) *openAIStream {
	return &openAIStream{
		reader:    bufio.NewReader(r),
		closer:    r,
		toolCalls: make(map[int]*accumulatedToolCall),
		queue:     make([]Event, 0),
	}
}

type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Role      string `json:"role,omitempty"`
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function,omitempty"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (s *openAIStream) Next(ctx context.Context) (Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Event{}, err
		}

		if len(s.queue) > 0 {
			ev := s.queue[0]
			s.queue = s.queue[1:]
			return ev, nil
		}

		if s.done {
			return Event{}, io.EOF
		}

		line, err := s.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.flushToolCalls()
				s.queue = append(s.queue, Event{Kind: EventDone})
				s.done = true
				if len(s.queue) > 0 {
					ev := s.queue[0]
					s.queue = s.queue[1:]
					return ev, nil
				}
				return Event{}, io.EOF
			}
			return Event{}, fmt.Errorf("read stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			s.flushToolCalls()
			s.queue = append(s.queue, Event{Kind: EventDone})
			s.done = true
			if len(s.queue) > 0 {
				ev := s.queue[0]
				s.queue = s.queue[1:]
				return ev, nil
			}
			return Event{Kind: EventDone}, nil
		}

		var chunk openAIChunk
		if unmarshalErr := json.Unmarshal([]byte(payload), &chunk); unmarshalErr != nil {
			continue
		}

		if chunk.Error != nil {
			return Event{}, fmt.Errorf("model error: %s", chunk.Error.Message)
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				s.queue = append(s.queue, Event{
					Kind: EventTextDelta,
					Text: choice.Delta.Content,
				})
			}

			for _, tc := range choice.Delta.ToolCalls {
				acc, exists := s.toolCalls[tc.Index]
				if !exists {
					acc = &accumulatedToolCall{}
					s.toolCalls[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.arguments.WriteString(tc.Function.Arguments)
				}
			}
		}

		if len(s.queue) > 0 {
			ev := s.queue[0]
			s.queue = s.queue[1:]
			return ev, nil
		}
	}
}

func (s *openAIStream) flushToolCalls() {
	if len(s.toolCalls) == 0 {
		return
	}

	indices := make([]int, 0, len(s.toolCalls))
	for idx := range s.toolCalls {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	for _, idx := range indices {
		acc := s.toolCalls[idx]
		argsStr := strings.TrimSpace(acc.arguments.String())
		if argsStr == "" {
			argsStr = "{}"
		}
		callID := acc.id
		if callID == "" {
			callID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), idx)
		}
		s.queue = append(s.queue, Event{
			Kind: EventToolCall,
			ToolCall: ToolCall{
				ID:        callID,
				Name:      acc.name,
				Arguments: json.RawMessage(argsStr),
			},
		})
	}
	s.toolCalls = make(map[int]*accumulatedToolCall)
}

func (s *openAIStream) Close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}
