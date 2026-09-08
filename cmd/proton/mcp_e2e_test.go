package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/phongsathornpt/proton/internal/adapter/in/acp"
	"github.com/phongsathornpt/proton/internal/adapter/out/tool/builtin"
	"github.com/phongsathornpt/proton/internal/core/permission"
	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/engine/toolcall"
)

func TestConfigureACPMCPStdioLifecycleEndToEnd(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	resource, err := configureACPMCP(context.Background(), t.TempDir(), registry, []acp.MCPServerConfig{{
		Name: "fixture", Command: executable,
		Args: []string{"-test.run=TestACPStdioMCPHelperProcess"},
		Env:  []string{"GO_WANT_ACP_MCP_HELPER=1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer resource.Close()

	definition, ok := findDefinition(registry.Definitions(), "mcp.fixture.echo")
	if !ok || definition.Kind != tool.KindMCP {
		t.Fatalf("discovered MCP definition = %#v", definition)
	}
	// The remote read-only hint remains conservative because ACP-provided servers
	// are untrusted unless local policy explicitly opts into trusting annotations.
	if definition.Mutability != tool.MutabilityUnspecified {
		t.Fatalf("effective mutability = %q, want conservative unspecified", definition.Mutability)
	}
	policy, err := permission.NewPolicy(permission.Config{Rules: []permission.Rule{{Action: permission.ActionAllow, Tool: permission.ToolMCP}}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	call, err := tool.NewCall("call-e2e", "mcp.fixture.echo", json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Call(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "hello" || string(result.StructuredOutput) != `{"echo":"hello"}` {
		t.Fatalf("MCP result = %#v", result)
	}
	if err := resource.Close(); err != nil {
		t.Fatal(err)
	}
	if err := resource.Close(); err != nil {
		t.Fatal(err)
	}
}

func findDefinition(definitions []tool.Definition, name string) (tool.Definition, bool) {
	for _, definition := range definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return tool.Definition{}, false
}

func TestACPStdioMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ACP_MCP_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id,omitempty"`
			Method  string          `json:"method,omitempty"`
			Params  json.RawMessage `json:"params,omitempty"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		if len(request.ID) == 0 {
			continue
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "fixture", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{
				"name": "echo", "description": "echo text",
				"inputSchema":  map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}, "required": []string{"text"}},
				"outputSchema": map[string]any{"type": "object", "properties": map[string]any{"echo": map[string]any{"type": "string"}}, "required": []string{"echo"}},
				"annotations":  map[string]any{"readOnlyHint": true, "idempotentHint": true},
			}}}
		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if json.Unmarshal(request.Params, &params) != nil || params.Name != "echo" {
				os.Exit(3)
			}
			text, _ := params.Arguments["text"].(string)
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "structuredContent": map[string]any{"echo": text}}
		default:
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32601, "message": "method not found: " + strings.TrimSpace(request.Method)}})
			continue
		}
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}
	os.Exit(0)
}
