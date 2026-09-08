package e2e_test

import (
	"os"
	"path/filepath"
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
