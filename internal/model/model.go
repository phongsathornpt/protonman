// Package model contains Proton CLI provider configuration and compatibility
// adapters. Provider-neutral message types are owned by proton-sdk.
package model

import (
	"context"
	"fmt"

	"github.com/projectTHORN/proton/internal/tool"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

var (
	ErrInvalidRequest   = sdk.ErrInvalidRequest
	ErrInvalidEvent     = sdk.ErrInvalidEvent
	ErrIncompleteStream = sdk.ErrIncompleteStream
)

type Role = sdk.Role

const (
	RoleSystem    = sdk.RoleSystem
	RoleUser      = sdk.RoleUser
	RoleAssistant = sdk.RoleAssistant
	RoleTool      = sdk.RoleTool
)

type ContentPartType = sdk.ContentPartType

const (
	ContentPartText  = sdk.ContentPartText
	ContentPartImage = sdk.ContentPartImage
)

type ContentPart = sdk.ContentPart
type ToolCall = sdk.ToolCall
type Message = sdk.Message

// Request is the legacy CLI request boundary. Tool execution definitions stay
// CLI-owned until the turn loop consumes proton-sdk.Tool directly.
type Request struct {
	Messages []Message
	Tools    []tool.Definition
}

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

func CloneMessages(messages []Message) []Message { return sdk.CloneMessages(messages) }

type EventKind string

const (
	EventTextDelta EventKind = "text_delta"
	EventToolCall  EventKind = "tool_call"
	EventDone      EventKind = "done"
)

type Event struct {
	Kind     EventKind
	Text     string
	ToolCall ToolCall
}

func (e Event) Validate() error {
	switch e.Kind {
	case EventTextDelta, EventDone:
		return nil
	case EventToolCall:
		if err := e.ToolCall.Validate(); err != nil {
			return fmt.Errorf("validate model tool event: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported event kind %q", ErrInvalidEvent, e.Kind)
	}
}

type Stream interface {
	Next(ctx context.Context) (Event, error)
	Close() error
}

type Client interface {
	Stream(ctx context.Context, request Request) (Stream, error)
}
