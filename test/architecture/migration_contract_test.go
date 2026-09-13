package architecture_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNoDomainIshDirectory(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "internal", "domain-ish")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("internal/domain-ish directory must not exist")
	}
}

func TestAdapterToolDirectoryStructure(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal", "adapter", "out", "tool"))
	if err != nil {
		t.Fatalf("read internal/adapter/out/tool: %v", err)
	}
	expected := map[string]bool{
		"agent":   true,
		"builtin": true,
		"mcp":     true,
		"skill":   true,
		"todo":    true,
		"web":     true,
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			t.Errorf("unexpected entry in internal/adapter/out/tool: %s", entry.Name())
		}
	}
}

func TestNoToolBuiltinDirectory(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "internal", "tool", "builtin")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("internal/tool/builtin directory must not exist; moved to internal/adapter/out/tool/builtin")
	}
}

func TestNoRootMCPDirectory(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "internal", "mcp")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("internal/mcp directory must not exist at internal root; moved to internal/adapter/out/tool/mcp")
	}
}

func TestNoLingeringRootDirectories(t *testing.T) {
	root := repositoryRoot(t)
	formerRootDirs := []string{
		"acp", "agent", "agentprompt", "architecture", "checkpoint", "config", "headless", "mcp", "model",
		"modelprofile", "permission", "project", "sandbox", "session", "skill", "telemetry",
		"todo", "tool", "toolcall", "tui", "turn", "workspace",
		"buildinfo", "contextutil", "envconfig", "failure", "glob", "runtimepolicy",
	}
	for _, lingering := range formerRootDirs {
		path := filepath.Join(root, "internal", lingering)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("internal/%s must not exist at internal root; moved to Clean Architecture subpackages", lingering)
		}
	}
}
