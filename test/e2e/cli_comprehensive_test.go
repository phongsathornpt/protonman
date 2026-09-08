package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ECLIHelpAndUsage(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		res := runProton(t, runOptions{args: []string{flag}})
		if res.exitCode != 0 {
			t.Fatalf("run(%s) exit code = %d, want 0", flag, res.exitCode)
		}
		if !strings.Contains(res.stdout, "Usage:") || !strings.Contains(res.stdout, "Session flags:") {
			t.Fatalf("run(%s) missing usage output: %s", flag, res.stdout)
		}
	}
}

func TestE2ECLIInvalidFlagCombinations(t *testing.T) {
	// Unknown flag
	res := runProton(t, runOptions{args: []string{"--nonexistent-flag-random-123"}})
	if res.exitCode == 0 {
		t.Fatal("expected failure on unknown flag")
	}

	// Conflict: resume and new-session
	res = runProton(t, runOptions{args: []string{"--resume", "--new-session"}})
	if res.exitCode == 0 || !strings.Contains(res.stdout+res.stderr, "cannot specify both") {
		t.Fatalf("expected mutual exclusion error, got: %s %s", res.stdout, res.stderr)
	}
}

func TestE2ECLIHeadlessPromptVariations(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	// 1. -p flag
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo 'flag-p'"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "flag-p") {
		t.Fatalf("-p run failed: %s %s", res.stdout, res.stderr)
	}

	// 2. --prompt alias
	res = runProton(t, runOptions{
		args: []string{"-y", "--prompt", `/call bash {"command":"echo 'flag-prompt'"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "flag-prompt") {
		t.Fatalf("--prompt run failed: %s %s", res.stdout, res.stderr)
	}

	// 3. Positional argument prompt
	res = runProton(t, runOptions{
		args: []string{"-y", `/call bash {"command":"echo 'positional-prompt'"` + "}"},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "positional-prompt") {
		t.Fatalf("positional prompt failed: %s %s", res.stdout, res.stderr)
	}

	// 4. Stdin prompt via --headless
	res = runProton(t, runOptions{
		args:  []string{"-y", "--headless"},
		dir:   ws,
		stdin: `/call bash {"command":"echo 'stdin-prompt'"}` + "\n",
		env:   env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "stdin-prompt") {
		t.Fatalf("--headless stdin prompt failed: %s %s", res.stdout, res.stderr)
	}

	// 5. Empty stdin prompt via --headless
	res = runProton(t, runOptions{
		args:  []string{"-y", "--headless"},
		dir:   ws,
		stdin: "   \n\n  ",
		env:   env,
	})
	if res.exitCode == 0 || !strings.Contains(res.stdout+res.stderr, "empty") {
		t.Fatalf("expected empty prompt failure, got: %s %s", res.stdout, res.stderr)
	}
}

func TestE2ECLIOutputFormats(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	// Text format (default)
	res := runProton(t, runOptions{
		args: []string{"-y", "--output", "text", "-p", `/call bash {"command":"echo 'txt-mode'"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "txt-mode") {
		t.Fatalf("text output failed: %s", res.stdout)
	}

	// JSON format
	res = runProton(t, runOptions{
		args: []string{"-y", "--output", "json", "-p", `/call bash {"command":"echo 'json-mode'"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 {
		t.Fatalf("json output failed: %s", res.stdout)
	}

	lines := strings.Split(strings.TrimSpace(res.stdout), "\n")
	hasToolResult := false
	for _, l := range lines {
		var evt map[string]any
		if err := json.Unmarshal([]byte(l), &evt); err == nil {
			if evt["kind"] == "tool_result" {
				hasToolResult = true
			}
		}
	}
	if !hasToolResult {
		t.Fatalf("expected tool_result event in json output: %s", res.stdout)
	}
}

func TestE2ECLIAgentProfiles(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	profiles := []string{"pow", "int", "dex"}
	for _, prof := range profiles {
		res := runProton(t, runOptions{
			args: []string{"-y", "-a", prof, "-p", `/call bash {"command":"echo '` + prof + `'"}`},
			dir:  ws,
			env:  env,
		})
		if res.exitCode != 0 || !strings.Contains(res.stdout, prof) {
			t.Fatalf("agent profile %s failed: %s %s", prof, res.stdout, res.stderr)
		}
	}

	// Invalid profile
	res := runProton(t, runOptions{
		args: []string{"-y", "-a", "wizard-mage", "-p", "test"},
		dir:  ws,
		env:  env,
	})
	if res.exitCode == 0 || !strings.Contains(res.stdout+res.stderr, "unknown agent profile") {
		t.Fatalf("expected unknown agent profile error, got: %s %s", res.stdout, res.stderr)
	}
}

func TestE2ECLISandboxAndTelemetryEnv(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// PROTON_SANDBOX env
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo 'sandbox-env'"}`},
		dir:  ws,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTON_SANDBOX=workspace",
		},
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "sandbox-env") {
		t.Fatalf("PROTON_SANDBOX run failed: %s %s", res.stdout, res.stderr)
	}

	// PROTON_TELEMETRY=stderr
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo 'telemetry-env'"}`},
		dir:  ws,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTON_TELEMETRY=stderr",
		},
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "telemetry-env") {
		t.Fatalf("PROTON_TELEMETRY=stderr run failed: %s %s", res.stdout, res.stderr)
	}

	// Invalid PROTON_TELEMETRY
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", "test"},
		dir:  ws,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTON_TELEMETRY=invalid_sink_xyz",
		},
	})
	if res.exitCode == 0 || !strings.Contains(res.stdout+res.stderr, "unsupported PROTONMAN_TELEMETRY") {
		t.Fatalf("expected telemetry error, got: %s %s", res.stdout, res.stderr)
	}
}

func TestE2ECLITodoFileParsing(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Write TODO.md in workspace
	todoPath := filepath.Join(ws, "TODO.md")
	_ = os.WriteFile(todoPath, []byte("- [ ] First Item\n- [x] Second Item Completed\n"), 0o644)

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo 'todo-loaded'"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "todo-loaded") {
		t.Fatalf("run with TODO.md failed: %s %s", res.stdout, res.stderr)
	}
}
