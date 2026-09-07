// Package mcp adapts discovered MCP servers into Proton's permission-aware tool registry.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/projectTHORN/proton/internal/core/tool"
	sdk "github.com/projectTHORN/proton/proton-sdk"
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
	serverName, err := validServerName(serverName)
	if err != nil {
		return "", err
	}
	toolName, err = validToolName(toolName)
	if err != nil {
		return "", err
	}
	return "mcp." + serverName + "." + toolName, nil
}

// DiscoveryOptions configures trust and resource policy for one MCP catalog snapshot.
type DiscoveryOptions struct {
	Limits            Limits
	TrustServerSafety bool
}

func DefaultDiscoveryOptions() DiscoveryOptions {
	return DiscoveryOptions{Limits: DefaultLimits()}
}

// Discover queries every server concurrently using conservative, untrusted safety semantics.
func Discover(ctx context.Context, registry tool.BatchRegistrar, servers ...Server) error {
	return DiscoverWithOptions(ctx, registry, DefaultDiscoveryOptions(), servers...)
}

// DiscoverWithLimits discovers MCP tools while enforcing explicit resource bounds.
func DiscoverWithLimits(ctx context.Context, registry tool.BatchRegistrar, limits Limits, servers ...Server) error {
	options := DefaultDiscoveryOptions()
	options.Limits = limits
	return DiscoverWithOptions(ctx, registry, options, servers...)
}

// DiscoverWithOptions discovers MCP tools with an explicit local trust decision.
func DiscoverWithOptions(ctx context.Context, registry tool.BatchRegistrar, options DiscoveryOptions, servers ...Server) error {
	limits := options.Limits
	if registry == nil {
		return fmt.Errorf("discover MCP tools: registry is required")
	}
	if err := limits.validate(); err != nil {
		return fmt.Errorf("discover MCP tools: %w", err)
	}
	if len(servers) > limits.MaxServers {
		return fmt.Errorf("discover MCP tools: %d servers exceeds limit %d", len(servers), limits.MaxServers)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("discover MCP tools: %w", err)
	}

	if len(servers) == 0 {
		return nil
	}

	serverNames := make([]string, len(servers))
	seenServers := make(map[string]struct{}, len(servers))
	for i, server := range servers {
		if server == nil {
			return fmt.Errorf("discover MCP tools: server is required")
		}
		serverName := server.Name()
		if _, err := validServerName(serverName); err != nil {
			return fmt.Errorf("validate MCP server name: %w", err)
		}
		if _, exists := seenServers[serverName]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateServer, serverName)
		}
		seenServers[serverName] = struct{}{}
		serverNames[i] = serverName
	}

	type serverResult struct {
		manifests []Tool
		err       error
	}

	queryCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type indexedResult struct {
		index int
		serverResult
	}
	resultCh := make(chan indexedResult, len(servers))
	concurrency := limits.MaxConcurrentDiscovery
	if concurrency > len(servers) {
		concurrency = len(servers)
	}
	semaphore := make(chan struct{}, concurrency)
	for i, server := range servers {
		go func(idx int, s Server) {
			select {
			case semaphore <- struct{}{}:
			case <-queryCtx.Done():
				resultCh <- indexedResult{index: idx, serverResult: serverResult{err: queryCtx.Err()}}
				return
			}
			defer func() { <-semaphore }()
			manifests, err := s.ListTools(queryCtx)
			resultCh <- indexedResult{index: idx, serverResult: serverResult{manifests: manifests, err: err}}
		}(i, server)
	}

	results := make([]serverResult, len(servers))
	for received := 0; received < len(servers); received++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("discover MCP tools: %w", ctx.Err())
		case result := <-resultCh:
			results[result.index] = result.serverResult
			if result.err != nil {
				cancel()
				return fmt.Errorf("list MCP tools from %q: %w", serverNames[result.index], result.err)
			}
		}
	}

	totalTools := 0
	for i, result := range results {
		if len(result.manifests) > limits.MaxToolsPerServer {
			return fmt.Errorf("MCP server %q exposed %d tools; limit is %d", serverNames[i], len(result.manifests), limits.MaxToolsPerServer)
		}
		totalTools += len(result.manifests)
	}
	if totalTools > limits.MaxTotalTools {
		return fmt.Errorf("MCP servers exposed %d tools; total limit is %d", totalTools, limits.MaxTotalTools)
	}

	handlers := make([]tool.Handler, 0, totalTools)
	seenNames := make(map[string]struct{}, totalTools)
	for i, server := range servers {
		serverName := serverNames[i]
		for _, manifest := range results[i].manifests {
			if err := validateManifestLimits(serverName, manifest, limits); err != nil {
				return err
			}
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
			handler, err := newHandler(server, serverName, manifest, name, options.TrustServerSafety)
			if err != nil {
				return fmt.Errorf("clone MCP tool %q schemas: %w", name, err)
			}
			handlers = append(handlers, handler)
		}
	}

	for _, handler := range handlers {
		definition := handler.Definition()
		if err := definition.Validate(); err != nil {
			return fmt.Errorf("validate MCP tool %q contract: %w", definition.Name, err)
		}
		sdkTool := sdk.Tool{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema}
		if _, err := sdk.CompileToolInputValidator(sdkTool); err != nil {
			return fmt.Errorf("validate MCP tool %q input schema: %w", definition.Name, err)
		}
		if _, err := sdk.CompileToolOutputValidator(sdkTool); err != nil {
			return fmt.Errorf("validate MCP tool %q output schema: %w", definition.Name, err)
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
	if err := registry.RegisterBatch(handlers); err != nil {
		return fmt.Errorf("register MCP tools atomically: %w", err)
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
	trustServerSafety bool,
) (tool.Handler, error) {
	description := strings.TrimSpace(manifest.Description)
	if description == "" {
		description = "MCP tool " + name
	}
	inputSchema, err := cloneMCPSchema(manifest.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("input schema: %w", err)
	}
	outputSchema, err := cloneMCPSchema(manifest.OutputSchema)
	if err != nil {
		return nil, fmt.Errorf("output schema: %w", err)
	}
	return serverToolHandler{
		server: server, serverName: serverName, manifest: manifest,
		definition: tool.Definition{
			Name: name, Description: description, Kind: tool.KindMCP, Mutability: effectiveMCPMutability(manifest.Mutability, trustServerSafety),
			InputSchema: inputSchema, OutputSchema: outputSchema,
		},
	}, nil
}

func (h serverToolHandler) Definition() tool.Definition {
	return h.definition
}

func (h serverToolHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	result, err := h.server.CallTool(ctx, h.manifest.Name, call.Arguments)
	toolResult := tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           result.Output,
		StructuredOutput: append(json.RawMessage(nil), result.StructuredOutput...),
	}
	if err != nil {
		return toolResult, fmt.Errorf("call MCP tool %s.%s: %w", h.serverName, h.manifest.Name, err)
	}
	if result.IsError {
		message := fmt.Sprintf("MCP tool %s.%s returned an error", h.serverName, h.manifest.Name)
		if firstLine := diagnosticFirstLine(result.Output, 120); firstLine != "" {
			message = fmt.Sprintf("MCP tool %s.%s returned an error: %s", h.serverName, h.manifest.Name, firstLine)
		}
		failure := classifyFailure(FailureTool, h.serverName, h.manifest.Name, "tools/call", errors.New(message))
		toolResult.Failure = tool.FailureFromError(failure)
		return toolResult, failure
	}
	return toolResult, nil
}

func validServerName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value {
		return "", fmt.Errorf("%w: server name must be non-empty and trimmed", ErrInvalidNamespace)
	}
	for _, character := range trimmed {
		if character == '.' || character == 0 || unicode.IsSpace(character) || unicode.IsControl(character) {
			return "", fmt.Errorf("%w: server name %q contains a reserved character", ErrInvalidNamespace, value)
		}
	}
	return trimmed, nil
}

func validToolName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value {
		return "", fmt.Errorf("%w: tool name must be non-empty and trimmed", ErrInvalidNamespace)
	}
	if strings.HasPrefix(trimmed, ".") || strings.HasSuffix(trimmed, ".") || strings.Contains(trimmed, "..") {
		return "", fmt.Errorf("%w: tool name %q contains invalid dot sequence", ErrInvalidNamespace, value)
	}
	for _, character := range trimmed {
		if character == 0 || unicode.IsSpace(character) || unicode.IsControl(character) {
			return "", fmt.Errorf("%w: tool name %q contains a reserved character", ErrInvalidNamespace, value)
		}
	}
	return trimmed, nil
}

func effectiveMCPMutability(declared tool.Mutability, trusted bool) tool.Mutability {
	// A mutating declaration can only make policy stricter, so always honor it.
	if declared == tool.MutabilityMutating {
		return tool.MutabilityMutating
	}
	// Read-only claims can relax permission behavior and therefore require a
	// local trust decision. Untrusted or unspecified tools stay conservative.
	if trusted && declared == tool.MutabilityReadOnly {
		return tool.MutabilityReadOnly
	}
	return tool.MutabilityUnspecified
}

func cloneMCPSchema(schema map[string]any) (map[string]any, error) {
	if schema == nil {
		return map[string]any{}, nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("schema is not JSON-compatible: %w", err)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("decode cloned schema: %w", err)
	}
	return clone, nil
}

var _ tool.Handler = serverToolHandler{}
