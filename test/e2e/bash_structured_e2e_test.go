package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2EBashStructuredExecution(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}
	if err := os.MkdirAll(filepath.Join(ws, "nested", "work"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("cwd", func(t *testing.T) {
		res := runProton(t, runOptions{
			args: []string{"-y", "-p", `/call bash {"command":"pwd; printf marker > created.txt","cwd":"nested/work"}`},
			dir:  ws, env: env,
		})
		if res.exitCode != 0 {
			t.Fatalf("cwd bash failed: %s %s", res.stdout, res.stderr)
		}
		if _, err := os.Stat(filepath.Join(ws, "nested", "work", "created.txt")); err != nil {
			t.Fatalf("cwd output file missing: %v", err)
		}
		if _, err := os.Stat(filepath.Join(ws, "created.txt")); !os.IsNotExist(err) {
			t.Fatalf("bash wrote at workspace root, err=%v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		started := time.Now()
		res := runProton(t, runOptions{
			args: []string{"-y", "-p", `/call bash {"command":"printf before; sleep 5","timeout_seconds":1}`},
			dir:  ws, env: env, timeout: 4 * time.Second,
		})
		if res.exitCode == 0 {
			t.Fatalf("timeout bash unexpectedly succeeded: %s", res.stdout)
		}
		combined := res.stdout + res.stderr
		if !strings.Contains(combined, "deadline") {
			t.Fatalf("timeout result missing deadline classification: %s", combined)
		}
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			t.Fatalf("timeout took %s", elapsed)
		}
	})

	t.Run("stderr exit", func(t *testing.T) {
		res := runProton(t, runOptions{
			args: []string{"-y", "-p", `/call bash {"command":"printf out; printf err >&2; exit 7"}`},
			dir:  ws, env: env,
		})
		if res.exitCode == 0 {
			t.Fatal("non-zero bash unexpectedly succeeded")
		}
		combined := res.stdout + res.stderr
		for _, want := range []string{"out", "err", "7"} {
			if !strings.Contains(combined, want) {
				t.Fatalf("bash result missing %q: %s", want, combined)
			}
		}
	})
}
