package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ECanonicalProtonmanHomeWritesNewNamespace(t *testing.T) {
	ws := newTestWorkspace(t)
	home := t.TempDir()
	sessionID := "protonman-home-session"

	result := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTONMAN_SESSION_ID=" + sessionID,
		},
	})
	if result.exitCode != 0 {
		t.Fatalf("canonical Protonman run failed: %s\n%s", result.stdout, result.stderr)
	}

	statePath := filepath.Join(home, ".protonman", "sessions", sessionID, "state.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("canonical session missing at %s: %v", statePath, err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".proton")); !os.IsNotExist(err) {
		t.Fatalf("canonical run unexpectedly created legacy .proton directory: %v", err)
	}
}

func TestE2EStartupFromUserHomeUsesUserScopeOnly(t *testing.T) {
	home := newTestWorkspace(t)
	protonmanDir := filepath.Join(home, ".protonman")
	if err := os.MkdirAll(protonmanDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protonmanDir, "config.toml"), []byte("[agent]\nmax_tool_calls = 17\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "home-workspace-session"

	result := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  home,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTONMAN_SESSION_ID=" + sessionID,
		},
	})
	if result.exitCode != 0 {
		t.Fatalf("startup from user home failed: stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	for _, unexpected := range []string{
		"ignored untrusted project config",
		"checkpoint store must be outside workspace",
		"checkpoint store inside workspace must be reserved Protonman internal state",
	} {
		if strings.Contains(result.stderr, unexpected) {
			t.Fatalf("stderr contains %q: %s", unexpected, result.stderr)
		}
	}
	statePath := filepath.Join(protonmanDir, "sessions", sessionID, "state.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("session state missing after home startup: %v", err)
	}
}
