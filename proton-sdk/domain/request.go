package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ToolChoice string

const (
	ToolChoiceAuto     ToolChoice = ""
	ToolChoiceRequired ToolChoice = "required"
)

type ModelOptions struct {
	MaxOutputTokens  int
	ToolChoice       ToolChoice
	ReasoningEffort  ReasoningEffort
	ProviderOptions  ProviderOptions
	IncludeRawChunks bool
}

// RequestMetadata carries provider-neutral, request-scoped transport metadata.
// Providers may map these values to protocol headers, but callers own the identity.
type RequestMetadata struct {
	SessionID string
}

type Request struct {
	Messages []Message
	Tools    []Tool
	Options  ModelOptions
	Metadata RequestMetadata
}

func (r Request) Validate() error {
	if r.Options.MaxOutputTokens < 0 {
		return fmt.Errorf("%w: max output tokens cannot be negative", ErrInvalidRequest)
	}
	if r.Options.ToolChoice != ToolChoiceAuto && r.Options.ToolChoice != ToolChoiceRequired {
		return fmt.Errorf("%w: unsupported tool choice %q", ErrInvalidRequest, r.Options.ToolChoice)
	}
	if !r.Options.ReasoningEffort.Valid() {
		return fmt.Errorf("%w: unsupported reasoning effort %q", ErrInvalidRequest, r.Options.ReasoningEffort)
	}
	if r.Options.ToolChoice == ToolChoiceRequired && len(r.Tools) == 0 {
		return fmt.Errorf("%w: required tool choice needs at least one tool", ErrInvalidRequest)
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

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Clone returns a deep copy of the tool call, copying raw JSON arguments.
func (c ToolCall) Clone() ToolCall {
	c.Arguments = append(json.RawMessage(nil), c.Arguments...)
	return c
}

func (c ToolCall) Validate() error {
	return validateToolCall(c, ErrInvalidEvent)
}

func validateToolCall(c ToolCall, sentinel error) error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: tool call id is required", sentinel)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: tool name is required", sentinel)
	}
	if len(c.Arguments) > 0 && !json.Valid(c.Arguments) {
		return fmt.Errorf("%w: tool arguments must be valid JSON", sentinel)
	}
	return nil
}

type Tool struct {
	Name            string
	Description     string
	InputSchema     map[string]any
	OutputSchema    map[string]any
	ProviderOptions ProviderOptions
	Dynamic         bool
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
