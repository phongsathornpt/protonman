// Package model defines the provider-neutral model streaming boundary.
package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
)

// Role identifies the author of a message in a model conversation.
type Role string

const (
	// RoleSystem contains agent or provider instructions.
	RoleSystem Role = "system"
	// RoleUser contains human input.
	RoleUser Role = "user"
	// RoleAssistant contains model output and requested tool calls.
	RoleAssistant Role = "assistant"
	// RoleTool contains the result of an executed tool call.
	RoleTool Role = "tool"
)

// ErrInvalidRequest indicates that a model request cannot be sent safely.
var ErrInvalidRequest = errors.New("invalid model request")

// ErrInvalidEvent indicates that a provider emitted an invalid stream event.
var ErrInvalidEvent = errors.New("invalid model event")

// ToolCall is the model-facing representation of a requested tool call.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Validate checks the identifiers and JSON arguments in a model tool call.
func (c ToolCall) Validate() error {
	if _, err := tool.NewCall(c.ID, c.Name, c.Arguments); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	return nil
}

// ContentPartType distinguishes text from visual or binary content parts.
type ContentPartType string

const (
	// ContentPartText contains plain text.
	ContentPartText ContentPartType = "text"
	// ContentPartImage contains an image payload (base64 encoded).
	ContentPartImage ContentPartType = "image"
)

// ContentPart represents one typed block within a multi-modal message.
type ContentPart struct {
	Type     ContentPartType `json:"type"`
	Text     string          `json:"text,omitempty"`
	MIMEType string          `json:"mime_type,omitempty"`
	Data     string          `json:"data,omitempty"` // Base64-encoded
}

// Message is one provider-neutral conversation message.
type Message struct {
	Role       Role
	Content    string
	Parts      []ContentPart
	ToolCallID string
	ToolName   string
	ToolCalls  []ToolCall
}

// TextContent returns the message text, extracting from Parts if Content is empty.
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

// Validate checks message roles and embedded tool calls.
func (m Message) Validate() error {
	if !validRole(m.Role) {
		return fmt.Errorf("%w: unsupported message role %q", ErrInvalidRequest, m.Role)
	}
	if m.Role != RoleAssistant && len(m.ToolCalls) > 0 {
		return fmt.Errorf("%w: only assistant messages can contain tool calls", ErrInvalidRequest)
	}
	for _, call := range m.ToolCalls {
		if err := call.Validate(); err != nil {
			return fmt.Errorf("validate message tool call: %w", err)
		}
	}
	return nil
}

// Request is the input supplied to an injectable model client.
type Request struct {
	Messages []Message
	Tools    []tool.Definition
}

// Validate checks the conversation and published tool definitions.
func (r Request) Validate() error {
	if len(r.Messages) == 0 {
		return fmt.Errorf("%w: at least one message is required", ErrInvalidRequest)
	}
	for _, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
	}
	for _, definition := range r.Tools {
		if err := definition.Validate(); err != nil {
			return fmt.Errorf("validate model tool %q: %w", definition.Name, err)
		}
	}
	return nil
}

// CloneMessages returns an independent copy suitable for request history.
func CloneMessages(messages []Message) []Message {
	cloned := make([]Message, 0, len(messages))
	for _, message := range messages {
		clone := message
		if len(message.Parts) > 0 {
			clone.Parts = append([]ContentPart{}, message.Parts...)
		}
		clone.ToolCalls = make([]ToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			call.Arguments = append(json.RawMessage{}, call.Arguments...)
			clone.ToolCalls = append(clone.ToolCalls, call)
		}
		cloned = append(cloned, clone)
	}
	return cloned
}

// EventKind identifies a provider stream event.
type EventKind string

const (
	// EventTextDelta is a piece of assistant text.
	EventTextDelta EventKind = "text_delta"
	// EventToolCall is a complete model-requested tool call.
	EventToolCall EventKind = "tool_call"
	// EventDone marks the end of one model response.
	EventDone EventKind = "done"
)

// Event is one item emitted by a model stream.
type Event struct {
	Kind     EventKind
	Text     string
	ToolCall ToolCall
}

// Validate checks the event payload required by its kind.
func (e Event) Validate() error {
	switch e.Kind {
	case EventTextDelta:
		return nil
	case EventToolCall:
		if err := e.ToolCall.Validate(); err != nil {
			return fmt.Errorf("validate model tool event: %w", err)
		}
		return nil
	case EventDone:
		return nil
	default:
		return fmt.Errorf("%w: unsupported event kind %q", ErrInvalidEvent, e.Kind)
	}
}

// Stream reads one model response and must be closed by its caller.
type Stream interface {
	Next(ctx context.Context) (Event, error)
	Close() error
}

// Client is the injectable provider boundary used by the application loop.
type Client interface {
	Stream(ctx context.Context, request Request) (Stream, error)
}

func validRole(role Role) bool {
	switch role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}
