package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/appdirs"
	"github.com/projectTHORN/proton/internal/permission"
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
