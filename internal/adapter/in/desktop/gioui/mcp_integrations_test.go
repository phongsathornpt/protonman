//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type gioMCPRepository struct {
	items []app.MCPIntegration
	save  chan []app.MCPIntegration
	err   error
}

func (r *gioMCPRepository) Load(context.Context) ([]app.MCPIntegration, error) {
	return cloneAppMCPIntegrations(r.items), r.err
}

func (r *gioMCPRepository) Save(_ context.Context, items []app.MCPIntegration) error {
	if r.save != nil {
		r.save <- cloneAppMCPIntegrations(items)
	}
	return r.err
}

func cloneAppMCPIntegrations(items []app.MCPIntegration) []app.MCPIntegration {
	cloned := make([]app.MCPIntegration, len(items))
	for index, item := range items {
		cloned[index] = item
		cloned[index].Args = append([]string(nil), item.Args...)
		cloned[index].Env = append([]string(nil), item.Env...)
	}
	return cloned
}

func newMCPTestController(repository app.MCPIntegrationsRepository) *controller {
	return &controller{
		ctx:             context.Background(),
		mcpIntegrations: app.NewMCPIntegrations(repository),
		state:           desktopstate.State{},
		histories:       make(map[string]historyState),
	}
}

func TestMCPIntegrationsLoadNormalizesPersistedDefinitions(t *testing.T) {
	controller := newMCPTestController(&gioMCPRepository{items: []app.MCPIntegration{{
		Name:    " docs ",
		Command: " mcp-docs ",
		Args:    []string{" --stdio ", ""},
		Env:     []string{"TOKEN=secret", "TOKEN", "REGION"},
	}}})

	controller.loadMCPIntegrations()

	want := []desktopstate.MCPIntegrationState{{
		Name:    "docs",
		Command: "mcp-docs",
		Args:    []string{"--stdio"},
		Env:     []string{"TOKEN", "REGION"},
	}}
	if !reflect.DeepEqual(controller.state.Integrations, want) {
		t.Fatalf("integrations = %#v, want %#v", controller.state.Integrations, want)
	}
}

func TestMCPIntegrationsLoadPreservesRepositoryError(t *testing.T) {
	controller := newMCPTestController(&gioMCPRepository{err: errors.New("invalid preference")})

	controller.loadMCPIntegrations()

	if got := controller.snapshot().MCPError; got != "invalid preference" {
		t.Fatalf("MCP error = %q", got)
	}
}

func TestMCPIntegrationPayloadResolvesEnvironmentAtRuntime(t *testing.T) {
	t.Setenv("PROTONMAN_MCP_TEST_TOKEN", "runtime-secret")
	items := []desktopstate.MCPIntegrationState{{
		Name:    "docs",
		Command: "mcp-docs",
		Args:    []string{"--stdio"},
		Env:     []string{"PROTONMAN_MCP_TEST_TOKEN", "MISSING"},
	}}
	got := buildMCPServersPayload(items, func(key string) (string, bool) {
		if key == "PROTONMAN_MCP_TEST_TOKEN" {
			return "runtime-secret", true
		}
		return "", false
	})
	want := []map[string]any{{
		"name":    "docs",
		"command": "mcp-docs",
		"args":    []string{"--stdio"},
		"env":     []string{"PROTONMAN_MCP_TEST_TOKEN=runtime-secret"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload = %#v, want %#v", got, want)
	}
	if items[0].Env[0] != "PROTONMAN_MCP_TEST_TOKEN" {
		t.Fatal("payload construction mutated persisted environment keys")
	}
}

func TestMCPEnvironmentInputRejectsInlineSecret(t *testing.T) {
	if _, err := parseMCPEnvironmentKeys(`["TOKEN=secret"]`); err == nil || !strings.Contains(err.Error(), "not stored") {
		t.Fatalf("error = %v, want inline-secret guidance", err)
	}
}

func TestMCPSessionParamsIncludeAdditionalDirectoriesAndServers(t *testing.T) {
	t.Setenv("PROTONMAN_MCP_TEST_TOKEN", "secret")
	controller := newMCPTestController(nil)
	controller.state.Integrations = []desktopstate.MCPIntegrationState{{
		Name: "docs", Command: "mcp-docs", Env: []string{"PROTONMAN_MCP_TEST_TOKEN"},
	}}

	params := controller.mcpSessionParams("session-1", "/workspace", []string{"/workspace/extra"})
	if params["sessionId"] != "session-1" || params["cwd"] != "/workspace" {
		t.Fatalf("session params = %#v", params)
	}
	if !reflect.DeepEqual(params["additionalDirectories"], []string{"/workspace/extra"}) {
		t.Fatalf("additional directories = %#v", params["additionalDirectories"])
	}
	servers, ok := params["mcpServers"].([]map[string]any)
	if !ok || len(servers) != 1 || servers[0]["env"].([]string)[0] != "PROTONMAN_MCP_TEST_TOKEN=secret" {
		t.Fatalf("mcp servers = %#v", params["mcpServers"])
	}
}

func TestMCPReconnectRefusesBusySession(t *testing.T) {
	controller := newMCPTestController(&gioMCPRepository{})
	controller.state.Sessions = []desktopstate.SessionState{{ID: "session-1", Status: desktopstate.TaskRunning}}

	controller.reconnectMCP()

	if got := controller.snapshot().Status; got != "Cannot reconnect ACP while a session is active" {
		t.Fatalf("status = %q", got)
	}
	if controller.snapshot().MCPReconnecting {
		t.Fatal("busy session started an ACP reconnect")
	}
}

func TestMCPSavePersistsNormalizedState(t *testing.T) {
	repository := &gioMCPRepository{save: make(chan []app.MCPIntegration, 1)}
	controller := newMCPTestController(repository)

	controller.saveMCPIntegration("docs", "mcp-docs", `["--stdio"]`, `["TOKEN"]`)
	select {
	case saved := <-repository.save:
		if !reflect.DeepEqual(saved, []app.MCPIntegration{{Name: "docs", Command: "mcp-docs", Args: []string{"--stdio"}, Env: []string{"TOKEN"}}}) {
			t.Fatalf("saved = %#v", saved)
		}
	}
	for i := 0; i < 100 && controller.snapshot().MCPUpdating; i++ {
		time.Sleep(time.Millisecond)
	}
	if controller.snapshot().MCPUpdating {
		t.Fatal("MCP mutation did not finish")
	}
	if got := controller.state.Integrations; len(got) != 1 || got[0].Name != "docs" {
		t.Fatalf("state integrations = %#v", got)
	}
}

func TestMCPUpsertNormalizesAndDeduplicates(t *testing.T) {
	items, err := upsertMCPIntegration([]desktopstate.MCPIntegrationState{
		{Name: "z", Command: "z-server", Env: []string{"TOKEN=secret"}},
	}, desktopstate.MCPIntegrationState{Name: "a", Command: "a-server", Args: []string{" --stdio "}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "a" || items[1].Env[0] != "TOKEN" {
		t.Fatalf("items = %#v", items)
	}
}
