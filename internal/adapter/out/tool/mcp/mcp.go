// Package mcp defines the provider-neutral boundary for MCP discovery and calls.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/projectTHORN/proton/internal/core/tool"
)

// ErrInvalidTool indicates that an MCP tool manifest cannot be registered.
var ErrInvalidTool = errors.New("invalid MCP tool")

// Tool is the discovered manifest of one MCP server tool.
type Tool struct {
	Name         string
	Description  string
	InputSchema  map[string]any
	OutputSchema map[string]any
	// Mutability optionally declares whether successful execution can change state.
	// Unspecified remains conservative for MCP tools.
	Mutability tool.Mutability
}

// Validate checks the stable fields required for namespaced registration.
func (t Tool) Validate() error {
	trimmed := strings.TrimSpace(t.Name)
	if trimmed == "" || trimmed != t.Name {
		return fmt.Errorf("%w: tool name must be non-empty and trimmed", ErrInvalidTool)
	}
	if strings.HasPrefix(trimmed, ".") || strings.HasSuffix(trimmed, ".") || strings.Contains(trimmed, "..") {
		return fmt.Errorf("%w: tool name %q contains invalid dot sequence", ErrInvalidTool, t.Name)
	}
	for _, character := range trimmed {
		if character == 0 || unicode.IsSpace(character) || unicode.IsControl(character) {
			return fmt.Errorf("%w: tool name %q contains invalid characters", ErrInvalidTool, t.Name)
		}
	}
	switch t.Mutability {
	case tool.MutabilityUnspecified, tool.MutabilityReadOnly, tool.MutabilityMutating:
	default:
		return fmt.Errorf("%w: unsupported mutability %q", ErrInvalidTool, t.Mutability)
	}
	return nil
}

// Result is the provider-neutral result returned by an MCP tool call.
type Result struct {
	Output           string
	StructuredOutput json.RawMessage
	IsError          bool
}

// Server is an injectable MCP discovery and invocation endpoint. Implementations
// must honor context cancellation for in-flight transport operations.
type Server interface {
	Name() string
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, arguments json.RawMessage) (Result, error)
}
