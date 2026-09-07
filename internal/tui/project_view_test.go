package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/appdirs"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/permission"
	projectdomain "github.com/projectTHORN/proton/internal/project"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func TestProjectCommandLoadsTrustedWorkspaceState(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = t.TempDir()
	protonDir := appdirs.ProjectRoot(m.workDir)
	if err := os.MkdirAll(filepath.Join(protonDir, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := appdirs.ProjectConfig(m.workDir)
	if err := os.WriteFile(configPath, []byte("[agent]\nprofile = \"dex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protonDir, "skills", "review", "SKILL.md"), []byte("# Review"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.projectTrusted = true
	m.projectConfigSources = []string{configPath}
	m.activeModel = "gemini-3.8-flash"
	m.agentProfile = "dex"

	cmd := m.executeCommand("/project")
	if cmd == nil || !m.bottom.has(projectViewID) {
		t.Fatal("/project did not open async project pane")
	}
	loaded := cmd()
	updated, _ := m.Update(loaded)
	m = updated.(*bubbleModel)

	rendered := m.bottom.find(projectViewID).(*projectPaneView).Render(m)
	for _, want := range []string{"Project Settings", "loaded · trusted", "1 detected", "gemini-3.8-flash", "dex"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("project pane missing %q: %q", want, rendered)
		}
	}
}

func TestProjectCommandShowsUntrustedLocalResources(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = t.TempDir()
	protonDir := appdirs.ProjectRoot(m.workDir)
	if err := os.MkdirAll(filepath.Join(protonDir, "skills", "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appdirs.ProjectConfig(m.workDir), []byte("[agent]\nprofile = \"dex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protonDir, "skills", "local", "SKILL.md"), []byte("# Local"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := m.executeCommand("/proton status")
	if cmd == nil {
		t.Fatal("/proton alias did not start project discovery")
	}
	updated, _ := m.Update(cmd())
	m = updated.(*bubbleModel)
	rendered := m.bottom.find(projectViewID).(*projectPaneView).Render(m)
	for _, want := range []string{"ignored · untrusted", "1 detected · inactive until trusted", "not trusted"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("project pane missing %q: %q", want, rendered)
		}
	}
}

func TestProjectReloadIgnoresStaleResult(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = t.TempDir()
	first := m.openProjectPane()
	view := m.bottom.find(projectViewID).(*projectPaneView)
	firstID := view.requestID
	second := view.reload(m)
	if view.requestID == firstID {
		t.Fatal("reload did not advance request id")
	}
	updated, _ := m.Update(first())
	m = updated.(*bubbleModel)
	view = m.bottom.find(projectViewID).(*projectPaneView)
	if !view.loading {
		t.Fatal("stale project result cleared active reload")
	}
	updated, _ = m.Update(second())
	m = updated.(*bubbleModel)
	if m.bottom.find(projectViewID).(*projectPaneView).loading {
		t.Fatal("latest project result did not finish reload")
	}
}

func TestProjectInitCreatesConfigAndReloadsPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = t.TempDir()

	initCmd := m.executeCommand("/project init")
	if initCmd == nil {
		t.Fatal("/project init returned nil command")
	}
	updated, reloadCmd := m.Update(initCmd())
	m = updated.(*bubbleModel)
	if reloadCmd == nil {
		t.Fatal("project init did not schedule workspace reload")
	}
	updated, _ = m.Update(reloadCmd())
	m = updated.(*bubbleModel)

	if _, err := os.Stat(appdirs.ProjectConfig(m.workDir)); err != nil {
		t.Fatalf("project config not created: %v", err)
	}
	view := m.bottom.find(projectViewID).(*projectPaneView)
	if !view.state.ConfigExists || view.loading {
		t.Fatalf("project pane not refreshed after init: %#v", view)
	}
	if !strings.Contains(view.Render(m), "Created "+appdirs.RootDirName+"/"+appdirs.ConfigFileName) {
		t.Fatalf("project pane missing init confirmation: %q", view.Render(m))
	}
}

func TestProjectPaneShowsConfigurationProvenance(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.projectConfigProvenance = map[string]config.ValueSource{
		config.FieldModelDefault:         config.SourceProject,
		config.FieldModelProvider:        config.SourceUser,
		config.FieldAgentProfile:         config.SourceProject,
		config.FieldAgentReasoningEffort: config.SourceUser,
		config.FieldAgentMaxToolCalls:    config.SourceDefault,
		config.FieldUIPermissionMode:     config.SourceUser,
	}
	m.activeModel = "model-x"
	m.activeProvider = "provider-x"
	m.agentProfile = "dex"
	view := &projectPaneView{state: projectdomain.State{Trusted: true}}
	rendered := view.Render(m)
	for _, want := range []string{
		"model-x · project",
		"provider-x · user",
		"dex · project",
		"auto · user",
		"ask · user",
		"default",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("project pane missing provenance %q: %q", want, rendered)
		}
	}
}

func TestProjectSetUpdatesTrustedRuntimeAndConfig(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = t.TempDir()
	m.projectTrusted = true
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"

	for _, tc := range []struct {
		command string
		field   string
	}{
		{"/project set agent dex", config.FieldAgentProfile},
		{"/project set thinking high", config.FieldAgentReasoningEffort},
		{"/project set tool-calls 33", config.FieldAgentMaxToolCalls},
		{"/project set permission always-approve", config.FieldUIPermissionMode},
	} {
		cmd := m.executeCommand(tc.command)
		if cmd == nil {
			t.Fatalf("%s returned nil", tc.command)
		}
		updated, follow := m.Update(cmd())
		m = updated.(*bubbleModel)
		if follow != nil {
			updated, _ = m.Update(follow())
			m = updated.(*bubbleModel)
		}
		if got := m.projectConfigProvenance[tc.field]; got != config.SourceProject {
			t.Fatalf("%s provenance = %q", tc.field, got)
		}
	}
	if m.agentProfile != "dex" || m.reasoningEffort != sdk.ReasoningHigh || m.maxToolCalls != 33 || m.service.Mode() != permission.ModeAlwaysApprove {
		t.Fatalf("project settings not applied to runtime: profile=%q reasoning=%q tool_calls=%d mode=%s", m.agentProfile, m.reasoningEffort, m.maxToolCalls, m.service.Mode())
	}
	snapshot, err := config.Load(context.Background(), config.Options{HomeDir: t.TempDir(), WorkDir: m.workDir, ProjectTrusted: true})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.Profile != "dex" || snapshot.Agent.ReasoningEffort != sdk.ReasoningHigh || snapshot.Agent.MaxToolCalls != 33 || snapshot.Mode != permission.ModeAlwaysApprove {
		t.Fatalf("project settings not persisted: %#v", snapshot)
	}
}

func TestProjectSetRejectsUntrustedWorkspace(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = t.TempDir()
	if cmd := m.executeCommand("/project set tool-calls 20"); cmd != nil {
		t.Fatal("untrusted project write returned command")
	}
	if _, err := os.Stat(appdirs.ProjectConfig(m.workDir)); !os.IsNotExist(err) {
		t.Fatalf("untrusted project write touched config: %v", err)
	}
	if got := plainTranscript(m); !strings.Contains(got, "read-only until the workspace is trusted") {
		t.Fatalf("missing trust rejection: %q", got)
	}
}

func TestProjectPaneDistinguishesDetectedFromLoadedConfig(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.projectTrusted = true
	view := &projectPaneView{state: projectdomain.State{Trusted: true, ConfigExists: true, ConfigLoaded: false}}
	if got := view.Render(m); !strings.Contains(got, "detected · restart for full reload") {
		t.Fatalf("project pane did not distinguish detected config from loaded config: %q", got)
	}
}
