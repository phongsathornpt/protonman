// Package mcp adapts discovered MCP servers into Proton's permission-aware tool registry.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/projectTHORN/proton/internal/tool"
)

var (
	// ErrInvalidNamespace indicates that a server or tool cannot form a stable name.
	ErrInvalidNamespace = errors.New("invalid MCP namespace")
	// ErrDuplicateServer indicates that one server name was supplied more than once.
	ErrDuplicateServer = errors.New("duplicate MCP server")
	// ErrDuplicateDiscoveredTool indicates that discovery returned the same name twice.
	ErrDuplicateDiscoveredTool = errors.New("duplicate discovered MCP tool")
)

// NamespacedName returns the registry name for one discovered MCP tool.
func NamespacedName(serverName string, toolName string) (string, error) {
	serverName, err := validSegment("server", serverName)
	if err != nil {
		return "", err
	}
	toolName, err = validSegment("tool", toolName)
	if err != nil {
		return "", err
	}
	return "mcp." + serverName + "." + toolName, nil
}

// Discover queries every server, registers each discovered tool under its
// namespaced name, and wires invocations through tool.Handler.
func Discover(ctx context.Context, registry tool.Registrar, servers ...Server) error {
	if registry == nil {
		return fmt.Errorf("discover MCP tools: registry is required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("discover MCP tools: %w", err)
	}

	handlers := make([]tool.Handler, 0)
	seenServers := make(map[string]struct{}, len(servers))
	seenNames := make(map[string]struct{})
	for _, server := range servers {
		if server == nil {
			return fmt.Errorf("discover MCP tools: server is required")
		}
		serverName := strings.TrimSpace(server.Name())
		if _, err := validSegment("server", serverName); err != nil {
			return fmt.Errorf("validate MCP server name: %w", err)
		}
		if _, exists := seenServers[serverName]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateServer, serverName)
		}
		seenServers[serverName] = struct{}{}
		manifests, err := server.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("list MCP tools from %q: %w", serverName, err)
		}
		for _, manifest := range manifests {
			if err := manifest.Validate(); err != nil {
				return fmt.Errorf("validate MCP tool from %q: %w", serverName, err)
			}
			name, err := NamespacedName(serverName, manifest.Name)
			if err != nil {
				return fmt.Errorf("namespace MCP tool %q: %w", manifest.Name, err)
			}
			if _, exists := seenNames[name]; exists {
				return fmt.Errorf("%w: %s", ErrDuplicateDiscoveredTool, name)
			}
			seenNames[name] = struct{}{}
			handlers = append(handlers, newHandler(server, serverName, manifest, name))
		}
	}

	existingNames := make(map[string]struct{}, len(registry.Definitions()))
	for _, definition := range registry.Definitions() {
		existingNames[definition.Name] = struct{}{}
	}
	for _, handler := range handlers {
		name := handler.Definition().Name
		if _, exists := existingNames[name]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateDiscoveredTool, name)
		}
	}
	for _, handler := range handlers {
		if err := registry.Register(handler); err != nil {
			return fmt.Errorf("register MCP tool %q: %w", handler.Definition().Name, err)
		}
	}
	return nil
}

type serverToolHandler struct {
	server     Server
	serverName string
	manifest   Tool
	definition tool.Definition
}

func newHandler(
	server Server,
	serverName string,
	manifest Tool,
	name string,
) tool.Handler {
	description := strings.TrimSpace(manifest.Description)
	if description == "" {
		description = "MCP tool " + name
	}
	return serverToolHandler{
		server:     server,
		serverName: serverName,
		manifest:   manifest,
		definition: tool.Definition{
			Name:        name,
			Description: description,
			Kind:        tool.KindMCP,
			InputSchema: cloneSchema(manifest.InputSchema),
		},
	}
}

func (h serverToolHandler) Definition() tool.Definition {
	return h.definition
}

func (h serverToolHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	result, err := h.server.CallTool(ctx, h.manifest.Name, call.Arguments)
	toolResult := tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   result.Output,
	}
	if err != nil {
		return toolResult, fmt.Errorf("call MCP tool %s.%s: %w", h.serverName, h.manifest.Name, err)
	}
	if result.IsError {
		failure := tool.NewToolError(
			tool.ErrorCodeExecution,
			fmt.Sprintf("MCP tool %s.%s returned an error", h.serverName, h.manifest.Name),
		)
		toolResult.Failure = tool.FailureFromError(failure)
		return toolResult, failure
	}
	return toolResult, nil
}

func validSegment(label string, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value {
		return "", fmt.Errorf("%w: %s name must be non-empty and trimmed", ErrInvalidNamespace, label)
	}
	for _, character := range trimmed {
		if character == '.' || character == 0 || unicode.IsSpace(character) {
			return "", fmt.Errorf("%w: %s name %q contains a reserved character", ErrInvalidNamespace, label, value)
		}
	}
	return trimmed, nil
}

func cloneSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(schema))
	for key, value := range schema {
		clone[key] = value
	}
	return clone
}

var _ tool.Handler = serverToolHandler{}
