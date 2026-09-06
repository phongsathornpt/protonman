// Package protonsdk defines the provider-neutral model boundary used by Proton agents.
package protonsdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidRequest   = errors.New("invalid model request")
	ErrInvalidEvent     = errors.New("invalid model event")
	ErrIncompleteStream = errors.New("incomplete model stream")
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ContentPartType string

const (
	ContentPartText  ContentPartType = "text"
	ContentPartImage ContentPartType = "image"
)

type ContentPart struct {
	Type     ContentPartType `json:"type"`
	Text     string          `json:"text,omitempty"`
	MIMEType string          `json:"mime_type,omitempty"`
	Data     string          `json:"data,omitempty"`
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

func (c ToolCall) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: tool call id is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: tool name is required", ErrInvalidEvent)
	}
	if len(c.Arguments) > 0 && !json.Valid(c.Arguments) {
		return fmt.Errorf("%w: tool arguments must be valid JSON", ErrInvalidEvent)
	}
	return nil
}

type Tool struct {
	Name         string
	Description  string
	InputSchema  map[string]any
	OutputSchema map[string]any
	Dynamic      bool
}

// ToolResult is the provider-neutral result returned to a model after an agent
// executes a tool call. Execution policy remains owned by the agent runtime.
type ToolResult struct {
	ToolCallID string
	ToolName   string
	Content    string
	Parts      []ContentPart
	IsError    bool
}

func (r ToolResult) Validate() error {
	if strings.TrimSpace(r.ToolCallID) == "" {
		return fmt.Errorf("%w: tool result call id is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(r.ToolName) == "" {
		return fmt.Errorf("%w: tool result name is required", ErrInvalidRequest)
	}
	return nil
}

func (t Tool) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("%w: tool name is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(t.Description) == "" {
		return fmt.Errorf("%w: description is required for %q", ErrInvalidRequest, t.Name)
	}
	return nil
}

type Message struct {
	Role       Role
	Content    string
	Parts      []ContentPart
	ToolCallID string
	ToolName   string
	ToolCalls  []ToolCall
}

func (m Message) TextContent() string {
	if m.Content != "" {
		return m.Content
	}
	var builder strings.Builder
	for _, part := range m.Parts {
		if part.Type == ContentPartText && part.Text != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

func (m Message) Validate() error {
	if !validRole(m.Role) {
		return fmt.Errorf("%w: unsupported message role %q", ErrInvalidRequest, m.Role)
	}
	if m.Role != RoleAssistant && len(m.ToolCalls) > 0 {
		return fmt.Errorf("%w: only assistant messages can contain tool calls", ErrInvalidRequest)
	}
	for _, call := range m.ToolCalls {
		if err := call.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type ProviderOptions map[string]json.RawMessage
type ProviderMetadata map[string]json.RawMessage

type ModelOptions struct {
	MaxOutputTokens int
	ProviderOptions ProviderOptions
}

type Request struct {
	Messages []Message
	Tools    []Tool
	Options  ModelOptions
}

func (r Request) Validate() error {
	if r.Options.MaxOutputTokens < 0 {
		return fmt.Errorf("%w: max output tokens cannot be negative", ErrInvalidRequest)
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("%w: at least one message is required", ErrInvalidRequest)
	}
	for _, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
	}
	for _, tool := range r.Tools {
		if err := tool.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func CloneMessages(messages []Message) []Message {
	cloned := make([]Message, 0, len(messages))
	for _, message := range messages {
		clone := message
		clone.Parts = append([]ContentPart(nil), message.Parts...)
		clone.ToolCalls = make([]ToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			call.Arguments = append(json.RawMessage(nil), call.Arguments...)
			clone.ToolCalls = append(clone.ToolCalls, call)
		}
		cloned = append(cloned, clone)
	}
	return cloned
}

type FinishReason string

const (
	FinishStop      FinishReason = "stop"
	FinishLength    FinishReason = "length"
	FinishToolCalls FinishReason = "tool_calls"
	FinishError     FinishReason = "error"
	FinishOther     FinishReason = "other"
)

type Usage struct {
	InputTokens       int64
	OutputTokens      int64
	TotalTokens       int64
	CachedInputTokens int64
}

func (u Usage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 || u.CachedInputTokens < 0 {
		return fmt.Errorf("%w: token usage cannot be negative", ErrInvalidEvent)
	}
	return nil
}

type EventKind string

const (
	EventTextStart     EventKind = "text_start"
	EventTextDelta     EventKind = "text_delta"
	EventTextEnd       EventKind = "text_end"
	EventToolCallStart EventKind = "tool_call_start"
	EventToolCallDelta EventKind = "tool_call_delta"
	EventToolCallEnd   EventKind = "tool_call_end"
	// EventToolCall carries a complete tool call for agent runtimes that do not
	// need incremental argument rendering. Providers may emit both lifecycle
	// events and this normalized complete event.
	EventToolCall EventKind = "tool_call"
	EventUsage    EventKind = "usage"
	EventFinish   EventKind = "finish"
)

type Event struct {
	Kind EventKind
	Text string

	ToolCall         ToolCall
	ToolCallID       string
	ToolName         string
	ArgumentsDelta   string
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata ProviderMetadata
}

func (e Event) Validate() error {
	switch e.Kind {
	case EventTextStart, EventTextDelta, EventTextEnd:
		return nil
	case EventUsage:
		return e.Usage.Validate()
	case EventFinish:
		if !validFinishReason(e.FinishReason) {
			return fmt.Errorf("%w: unsupported finish reason %q", ErrInvalidEvent, e.FinishReason)
		}
		return nil
	case EventToolCallStart:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return fmt.Errorf("%w: tool call start id is required", ErrInvalidEvent)
		}
		if strings.TrimSpace(e.ToolName) == "" {
			return fmt.Errorf("%w: tool call start name is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCallDelta, EventToolCallEnd:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return fmt.Errorf("%w: tool call id is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCall:
		return e.ToolCall.Validate()
	default:
		return fmt.Errorf("%w: unsupported event kind %q", ErrInvalidEvent, e.Kind)
	}
}

type Stream interface {
	Next(ctx context.Context) (Event, error)
	Close() error
}

func validFinishReason(reason FinishReason) bool {
	switch reason {
	case FinishStop, FinishLength, FinishToolCalls, FinishError, FinishOther:
		return true
	default:
		return false
	}
}

func validRole(role Role) bool {
	switch role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}
