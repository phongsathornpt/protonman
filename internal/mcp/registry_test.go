package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/projectTHORN/proton/internal/adapters/tools"
	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/permission"
	domaintool "github.com/projectTHORN/proton/internal/tool"
)

func TestNamespacedName(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		toolName   string
		want       string
		wantErr    error
	}{
		{name: "valid", serverName: "github", toolName: "search", want: "mcp.github.search"},
		{name: "empty server", toolName: "search", wantErr: ErrInvalidNamespace},
		{name: "empty tool", serverName: "github", wantErr: ErrInvalidNamespace},
		{name: "server dot", serverName: "github.cloud", toolName: "search", wantErr: ErrInvalidNamespace},
		{name: "tool whitespace", serverName: "github", toolName: "search now", wantErr: ErrInvalidNamespace},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NamespacedName(test.serverName, test.toolName)
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("NamespacedName() error = %v", err)
				}
				if got != test.want {
					t.Fatalf("NamespacedName() = %q, want %q", got, test.want)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("NamespacedName() error = %v, want errors.Is(..., %v)", err, test.wantErr)
			}
		})
	}
}

func TestDiscoverRegistersNamespacedToolsAndDispatches(t *testing.T) {
	server := &fakeServer{
		name: "github",
		tools: []Tool{{
			Name:        "search",
			Description: "search issues",
			InputSchema: map[string]any{"type": "object"},
		}},
		results: map[string]Result{
			"search": {Output: "found 3 issues"},
		},
	}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if err := Discover(context.Background(), registry, server); err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	definitions := registry.Definitions()
	if got, want := len(definitions), 1; got != want {
		t.Fatalf("definitions = %d, want %d", got, want)
	}
	definition := definitions[0]
	if got, want := definition.Name, "mcp.github.search"; got != want {
		t.Fatalf("definition name = %q, want %q", got, want)
	}
	if definition.Kind != domaintool.KindMCP {
		t.Fatalf("definition kind = %q, want %q", definition.Kind, domaintool.KindMCP)
	}

	service := newMCPService(t, registry, permission.ActionAllow)
	call, err := domaintool.NewCall(
		"call-1",
		"mcp.github.search",
		json.RawMessage(`{"query":"proton"}`),
	)
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	result, err := service.Call(context.Background(), call)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if got, want := result.Output, "found 3 issues"; got != want {
		t.Fatalf("MCP output = %q, want %q", got, want)
	}
	if got, want := server.calls, []string{"search"}; !sameStrings(got, want) {
		t.Fatalf("server calls = %#v, want %#v", got, want)
	}
}

func TestDiscoverKeepsMCPCallsBehindPermission(t *testing.T) {
	server := &fakeServer{
		name:  "filesystem",
		tools: []Tool{{Name: "read", Description: "read remote data"}},
		results: map[string]Result{
			"read": {Output: "secret"},
		},
	}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if err := Discover(context.Background(), registry, server); err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	service := newMCPService(t, registry, permission.ActionDeny)
	call, err := domaintool.NewCall("call-denied", "mcp.filesystem.read", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	result, err := service.Call(context.Background(), call)
	if !errors.Is(err, toolcall.ErrPermissionDenied) {
		t.Fatalf("Call() error = %v, want permission denied", err)
	}
	if !result.Denied || result.Failure == nil || result.Failure.Code != domaintool.ErrorCodePermissionDenied {
		t.Fatalf("denied result = %#v, want structured permission denial", result)
	}
	if len(server.calls) != 0 {
		t.Fatalf("server calls = %#v, want none", server.calls)
	}
}

func TestDiscoverDoesNotPartiallyRegisterInvalidResults(t *testing.T) {
	server := &fakeServer{
		name: "broken",
		tools: []Tool{
			{Name: "valid", Description: "valid tool"},
			{Name: "bad tool", Description: "invalid namespace"},
		},
	}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if err := Discover(context.Background(), registry, server); err == nil {
		t.Fatal("Discover() error = nil, want invalid namespace")
	}
	if got, want := len(registry.Definitions()), 0; got != want {
		t.Fatalf("definitions after failed discovery = %d, want %d", got, want)
	}
}

func TestDiscoverRejectsDuplicateToolsAndServers(t *testing.T) {
	first := &fakeServer{
		name:  "github",
		tools: []Tool{{Name: "search", Description: "search"}},
	}
	second := &fakeServer{
		name:  "github",
		tools: []Tool{{Name: "issues", Description: "issues"}},
	}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if err := Discover(context.Background(), registry, first, second); !errors.Is(err, ErrDuplicateServer) {
		t.Fatalf("duplicate server error = %v, want ErrDuplicateServer", err)
	}
	if err := Discover(context.Background(), registry, first); err != nil {
		t.Fatalf("first Discover() error = %v", err)
	}
	if err := Discover(context.Background(), registry, first); !errors.Is(err, ErrDuplicateDiscoveredTool) {
		t.Fatalf("duplicate tool error = %v, want ErrDuplicateDiscoveredTool", err)
	}
}

func TestMCPServerErrorBecomesStructuredToolFailure(t *testing.T) {
	server := &fakeServer{
		name:  "remote",
		tools: []Tool{{Name: "fail", Description: "fails"}},
		results: map[string]Result{
			"fail": {Output: "remote failure", IsError: true},
		},
	}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if err := Discover(context.Background(), registry, server); err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	service := newMCPService(t, registry, permission.ActionAllow)
	call, err := domaintool.NewCall("call-error", "mcp.remote.fail", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	result, err := service.Call(context.Background(), call)
	if err == nil {
		t.Fatal("Call() error = nil, want MCP failure")
	}
	if result.Output != "remote failure" {
		t.Fatalf("MCP error output = %q, want remote failure", result.Output)
	}
	if result.Failure == nil || result.Failure.Code != domaintool.ErrorCodeExecution {
		t.Fatalf("MCP error failure = %#v, want execution_error", result.Failure)
	}
}

type fakeServer struct {
	name    string
	tools   []Tool
	results map[string]Result
	calls   []string
}

func (s *fakeServer) Name() string {
	return s.name
}

func (s *fakeServer) ListTools(context.Context) ([]Tool, error) {
	return append([]Tool{}, s.tools...), nil
}

func (s *fakeServer) CallTool(_ context.Context, name string, _ json.RawMessage) (Result, error) {
	s.calls = append(s.calls, name)
	return s.results[name], nil
}

func newMCPService(t *testing.T, registry domaintool.Registrar, action permission.Action) *toolcall.Service {
	t.Helper()
	policy, err := permission.NewPolicy(permission.Config{
		Rules: []permission.Rule{{
			Action: action,
			Tool:   permission.ToolMCP,
		}},
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(registry, policy)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
