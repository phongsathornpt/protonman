//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

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

func TestMCPNewSessionParamsIncludeAdditionalDirectories(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()
	second := t.TempDir()
	controller := newMCPTestController(nil)

	params := controller.mcpNewSessionParams(primary, []string{extra, second})
	if params["cwd"] != primary {
		t.Fatalf("cwd = %v, want the primary folder", params["cwd"])
	}
	if !reflect.DeepEqual(params["additionalDirectories"], []string{extra, second}) {
		t.Fatalf("additional directories = %#v", params["additionalDirectories"])
	}
}

func TestMCPNewSessionParamsOmitEmptyAdditionalDirectories(t *testing.T) {
	controller := newMCPTestController(nil)

	params := controller.mcpNewSessionParams(t.TempDir(), nil)
	if _, present := params["additionalDirectories"]; present {
		t.Fatalf("params = %#v, want no additionalDirectories key", params)
	}
}

func TestProjectAdditionalDirectoriesExcludesPrimaryAndStaleFolders(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()
	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "alias")
	if err := os.Symlink(extra, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	controller := newMCPTestController(nil)
	controller.state.Projects = []desktopstate.ProjectState{{
		ID: "project-1",
		Folders: []desktopstate.ProjectFolder{
			{Path: primary, Primary: true},
			{Path: extra},
			{Path: alias}, // resolves to extra: duplicate
			{Path: filepath.Join(extra, "does-not-exist")}, // stale
		},
	}}

	// Comparison uses the canonical form: t.TempDir() is under /var, which is a
	// symlink to /private/var on macOS.
	want := canonicalWorkspacePath(extra)
	got := controller.projectAdditionalDirectoriesLocked("project-1", primary)
	if !reflect.DeepEqual(got, []string{want}) {
		t.Fatalf("additional directories = %#v, want only %q", got, want)
	}
}

func TestProjectAdditionalDirectoriesIgnoresOtherProjects(t *testing.T) {
	extra := t.TempDir()
	controller := newMCPTestController(nil)
	controller.state.Projects = []desktopstate.ProjectState{
		{ID: "other", Folders: []desktopstate.ProjectFolder{{Path: extra}}},
		{ID: "project-1", Folders: []desktopstate.ProjectFolder{{Path: extra}}},
	}

	want := canonicalWorkspacePath(extra)
	got := controller.projectAdditionalDirectoriesLocked("project-1", t.TempDir())
	if len(got) != 1 || got[0] != want {
		t.Fatalf("additional directories = %#v, want only %q", got, want)
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

func TestParseSmartArgumentsAndEnvironment(t *testing.T) {
	args, err := parseMCPStringList("--stdio --log-level debug")
	if err != nil {
		t.Fatalf("parseMCPStringList: %v", err)
	}
	if !reflect.DeepEqual(args, []string{"--stdio", "--log-level", "debug"}) {
		t.Fatalf("unexpected args: %#v", args)
	}

	env, err := parseMCPEnvironmentKeys("API_KEY, GH_TOKEN\nPROT_SECRET")
	if err != nil {
		t.Fatalf("parseMCPEnvironmentKeys: %v", err)
	}
	if !reflect.DeepEqual(env, []string{"API_KEY", "GH_TOKEN", "PROT_SECRET"}) {
		t.Fatalf("unexpected env: %#v", env)
	}
}

func TestMCPIntegrationsPanelAndPresetsLayout(t *testing.T) {
	view := newShell(newTheme("dark"))
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			Integrations: []desktopstate.MCPIntegrationState{
				{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}, Env: []string{"GITHUB_TOKEN"}},
			},
		},
	}

	// 1. Layout panel
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	dims := view.layoutMCPIntegrationsPanel(gtx, snapshot)
	router.Frame(gtx.Ops)
	if dims.Size.X == 0 {
		t.Fatal("layoutMCPIntegrationsPanel returned 0 width")
	}

	// 2. Open form for new integration
	view.mcpFormVisible = true
	view.mcpSelectedName = ""
	view.clearMCPIntegrationEditors()

	// Click GitHub preset
	view.mcpPresetGitHubBtn.Click()
	var opGitHub op.Ops
	gtxGitHub := layout.Context{
		Ops:         &opGitHub,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutMCPIntegrationsPanel(gtxGitHub, snapshot)
	router.Frame(gtxGitHub.Ops)

	if view.mcpNameEditor.Text() != "github" || view.mcpCommandEditor.Text() != "npx" || !strings.Contains(view.mcpArgsEditor.Text(), "server-github") {
		t.Fatalf("github preset mismatch: name=%q cmd=%q args=%q", view.mcpNameEditor.Text(), view.mcpCommandEditor.Text(), view.mcpArgsEditor.Text())
	}

	// Click Memory preset
	view.mcpPresetMemoryBtn.Click()
	var opMemory op.Ops
	gtxMemory := layout.Context{
		Ops:         &opMemory,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutMCPIntegrationsPanel(gtxMemory, snapshot)
	router.Frame(gtxMemory.Ops)

	if view.mcpNameEditor.Text() != "memory" || view.mcpCommandEditor.Text() != "npx" || !strings.Contains(view.mcpArgsEditor.Text(), "server-memory") {
		t.Fatalf("memory preset mismatch: name=%q cmd=%q args=%q", view.mcpNameEditor.Text(), view.mcpCommandEditor.Text(), view.mcpArgsEditor.Text())
	}
}

func TestMCPConfigurationCardWidthExpansion(t *testing.T) {
	view := newShell(newTheme("dark"))
	snapshot := controllerSnapshot{}
	view.clearMCPIntegrationEditors()

	var opCard op.Ops
	var router input.Router
	gtxCard := layout.Context{
		Ops:         &opCard,
		Constraints: layout.Constraints{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 520, Y: 800}},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	dimsCard := view.layoutMCPConfigurationCard(gtxCard, snapshot, nil, true)
	router.Frame(gtxCard.Ops)
	if dimsCard.Size.X != 520 {
		t.Fatalf("layoutMCPConfigurationCard width = %d, want 520", dimsCard.Size.X)
	}
}

func TestMCPIntegrationDeleteConfirmationFlow(t *testing.T) {
	view := newShell(newTheme("dark"))
	var removedName string
	view.onRemoveMCPIntegration = func(name string) {
		removedName = name
	}
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			Integrations: []desktopstate.MCPIntegrationState{
				{Name: "fetch", Command: "uvx", Args: []string{"mcp-server-fetch"}},
			},
		},
	}
	view.mcpFormVisible = true
	view.mcpSelectedName = "fetch"
	view.mcpNameEditor.SetText("fetch")
	view.mcpCommandEditor.SetText("uvx")

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutMCPIntegrationsPanel(gtx, snapshot)
	router.Frame(gtx.Ops)

	// Click remove integration
	view.mcpRemoveButton.Click()
	var opRemove op.Ops
	gtxRemove := layout.Context{
		Ops:         &opRemove,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutMCPIntegrationsPanel(gtxRemove, snapshot)
	router.Frame(gtxRemove.Ops)

	if !view.mcpConfirmDelete {
		t.Fatal("clicking remove integration did not activate delete confirmation mode")
	}

	// Click Cancel
	view.mcpCancelDeleteBtn.Click()
	var opCancel op.Ops
	gtxCancel := layout.Context{
		Ops:         &opCancel,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutMCPIntegrationsPanel(gtxCancel, snapshot)
	router.Frame(gtxCancel.Ops)

	if view.mcpConfirmDelete {
		t.Fatal("canceling delete did not exit delete confirmation mode")
	}

	// Click remove integration again and confirm
	view.mcpRemoveButton.Click()
	var opRemove2 op.Ops
	gtxRemove2 := layout.Context{
		Ops:         &opRemove2,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutMCPIntegrationsPanel(gtxRemove2, snapshot)
	router.Frame(gtxRemove2.Ops)

	view.mcpConfirmDeleteBtn.Click()
	var opConfirm op.Ops
	gtxConfirm := layout.Context{
		Ops:         &opConfirm,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutMCPIntegrationsPanel(gtxConfirm, snapshot)
	router.Frame(gtxConfirm.Ops)

	if removedName != "fetch" {
		t.Fatalf("onRemoveMCPIntegration was not called with 'fetch', got %q", removedName)
	}
	if view.mcpFormVisible {
		t.Fatal("form remained visible after deletion")
	}
}
