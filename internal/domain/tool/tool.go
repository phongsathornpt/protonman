// Package tool defines the provider-neutral contract for executable tools.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Kind classifies a tool for permission policy matching.
type Kind string

const (
	// KindRead identifies tools that read local project state.
	KindRead Kind = "read"
	// KindEdit identifies tools that mutate project files.
	KindEdit Kind = "edit"
	// KindBash identifies tools that execute shell commands.
	KindBash Kind = "bash"
	// KindGrep identifies tools that search project contents.
	KindGrep Kind = "grep"
	// KindMCP identifies tools provided by an MCP server.
	KindMCP Kind = "mcp"
	// KindWebFetch identifies tools that fetch a URL.
	KindWebFetch Kind = "web_fetch"
	// KindWebSearch identifies tools that search the web.
	KindWebSearch Kind = "web_search"
)

// ErrInvalidCall indicates that a call envelope cannot be dispatched safely.
var ErrInvalidCall = errors.New("invalid tool call")

// Call is the JSON-typed envelope passed from a model or UI to a tool.
type Call struct {
	// ID uniquely identifies the call within a turn or session.
	ID string
	// Name is the registered tool name.
	Name string
	// Arguments contains a JSON object accepted by the tool.
	Arguments json.RawMessage
}

// NewCall validates and copies a tool call envelope.
func NewCall(id string, name string, arguments json.RawMessage) (Call, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" {
		return Call{}, fmt.Errorf("%w: call id is required", ErrInvalidCall)
	}
	if name == "" {
		return Call{}, fmt.Errorf("%w: tool name is required", ErrInvalidCall)
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(arguments) {
		return Call{}, fmt.Errorf("%w: arguments must be valid JSON", ErrInvalidCall)
	}

	return Call{
		ID:        id,
		Name:      name,
		Arguments: append(json.RawMessage(nil), arguments...),
	}, nil
}

// Validate checks a call that may have been built as a struct literal.
func (c Call) Validate() error {
	_, err := NewCall(c.ID, c.Name, c.Arguments)
	return err
}

// Definition describes a registered tool to the model and the UI.
type Definition struct {
	// Name is the stable dispatch name.
	Name string
	// Description explains the tool's purpose to a model or user.
	Description string
	// Kind is the permission category for this tool.
	Kind Kind
	// PermissionDetailKey names the JSON argument shown to a permission
	// prompt and matched by path, command, or domain rules.
	PermissionDetailKey string
	// InputSchema is the JSON-schema-like manifest exposed to callers.
	InputSchema map[string]any
}

// Validate checks the invariants required for safe registry insertion.
func (d Definition) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("%w: tool name is required", ErrInvalidCall)
	}
	if strings.TrimSpace(d.Description) == "" {
		return fmt.Errorf("%w: description is required for %q", ErrInvalidCall, d.Name)
	}
	if !validKind(d.Kind) {
		return fmt.Errorf("%w: unsupported kind %q for %q", ErrInvalidCall, d.Kind, d.Name)
	}
	return nil
}

// Result is the model-facing output of a tool execution.
type Result struct {
	// CallID identifies the originating call.
	CallID string
	// ToolName identifies the handler that produced the result.
	ToolName string
	// Output is human- and model-readable text for this initial port slice.
	Output string
	// ExitCode is populated by process-backed tools when a process exits.
	ExitCode *int
	// Denied reports that execution was blocked before the handler ran.
	Denied bool
}

// Handler executes one registered tool call.
type Handler interface {
	// Definition returns the stable manifest and permission category.
	Definition() Definition
	// Execute performs the tool's external effect, honoring context
	// cancellation where the underlying operation supports it.
	Execute(ctx context.Context, call Call) (Result, error)
}

// Registry resolves tool names and publishes their definitions.
type Registry interface {
	// Lookup returns the handler registered under name.
	Lookup(name string) (Handler, bool)
	// Definitions returns a stable snapshot of registered tool manifests.
	Definitions() []Definition
}

func validKind(kind Kind) bool {
	switch kind {
	case KindRead, KindEdit, KindBash, KindGrep, KindMCP, KindWebFetch, KindWebSearch:
		return true
	default:
		return false
	}
}
