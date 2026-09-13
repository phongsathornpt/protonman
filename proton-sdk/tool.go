package protonsdk

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
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
