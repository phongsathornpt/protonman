package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/core/tool"
)

func TestStdioServerListsAndCallsTools(t *testing.T) {
	server, err := NewStdioServer(
		"fixture", os.Args[0], []string{"-test.run=TestMCPStdioHelperProcess"},
		[]string{"GO_WANT_MCP_HELPER=1"}, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	tools, err := server.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" || tools[0].InputSchema["type"] != "object" || tools[0].Mutability != tool.MutabilityReadOnly {
		t.Fatalf("tools = %#v", tools)
	}
	result, err := server.CallTool(context.Background(), "echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "hi" || string(result.StructuredOutput) != `{"echo":"hi"}` || result.IsError {
		t.Fatalf("result = %#v", result)
	}
}

func TestStdioServerRejectsMissingCommand(t *testing.T) {
	if _, err := NewStdioServer("fixture", " ", nil, nil, ""); err == nil {
		t.Fatal("NewStdioServer error = nil")
	}
}

func TestMCPStdioHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request rpcEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}
		if len(request.ID) == 0 {
			continue
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": stdioProtocolVersion, "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "fixture", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "echo text", "inputSchema": map[string]any{"type": "object"}, "outputSchema": map[string]any{"type": "object"}, "annotations": map[string]any{"readOnlyHint": true, "idempotentHint": true}}}}
		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				os.Exit(3)
			}
			text := fmt.Sprint(params.Arguments["text"])
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "structuredContent": map[string]any{"echo": text}}
		default:
			_ = encoder.Encode(rpcEnvelope{JSONRPC: "2.0", ID: request.ID, Error: &rpcError{Code: -32601, Message: "method not found: " + strings.TrimSpace(request.Method)}})
			continue
		}
		data, _ := json.Marshal(result)
		if err := encoder.Encode(rpcEnvelope{JSONRPC: "2.0", ID: request.ID, Result: data}); err != nil {
			os.Exit(4)
		}
	}
	os.Exit(0)
}

func TestStdioServerEnforcesArgumentAndOutputLimits(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxArgumentsBytes = 8
	server, err := NewStdioServerWithLimits("fixture", os.Args[0], []string{"-test.run=TestMCPStdioHelperProcess"}, []string{"GO_WANT_MCP_HELPER=1"}, "", limits)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if _, err := server.CallTool(context.Background(), "echo", json.RawMessage(`{"text":"too-large"}`)); err == nil || !strings.Contains(err.Error(), "arguments") {
		t.Fatalf("argument limit error = %v", err)
	}

	limits = DefaultLimits()
	limits.MaxTextOutputBytes = 2
	server2, err := NewStdioServerWithLimits("fixture2", os.Args[0], []string{"-test.run=TestMCPStdioHelperProcess"}, []string{"GO_WANT_MCP_HELPER=1"}, "", limits)
	if err != nil {
		t.Fatal(err)
	}
	defer server2.Close()
	if _, err := server2.CallTool(context.Background(), "echo", json.RawMessage(`{"text":"hello"}`)); err == nil || !strings.Contains(err.Error(), "text output") {
		t.Fatalf("text limit error = %v", err)
	}
}
