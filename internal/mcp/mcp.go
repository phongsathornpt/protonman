// Package mcp defines the provider-neutral boundary for MCP discovery and calls.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidTool indicates that an MCP tool manifest cannot be registered.
var ErrInvalidTool = errors.New("invalid MCP tool")

// Tool is the discovered manifest of one MCP server tool.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Validate checks the stable fields required for namespaced registration.
func (t Tool) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("%w: tool name is required", ErrInvalidTool)
	}
	if strings.ContainsAny(t.Name, "\x00\r\n\t") {
		return fmt.Errorf("%w: tool name contains control characters", ErrInvalidTool)
	}
	return nil
}

// Result is the provider-neutral result returned by an MCP tool call.
type Result struct {
	Output  string
	IsError bool
}

// Server is an injectable MCP discovery and invocation endpoint.
type Server interface {
	Name() string
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, arguments json.RawMessage) (Result, error)
}
