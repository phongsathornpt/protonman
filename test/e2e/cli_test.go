package e2e_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIHelp(t *testing.T) {
	result := runProton(t, runOptions{
		args: []string{"--help"},
	})
	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", result.exitCode, result.stderr)
	}
	if !strings.Contains(result.stdout, "Usage:") || !strings.Contains(result.stdout, "proton") {
		t.Fatalf("stdout missing usage, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "--headless") {
		t.Fatalf("stdout missing --headless flag info, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "--acp") {
		t.Fatalf("stdout missing --acp flag info, got: %s", result.stdout)
	}
}

func TestCLIRefusesTUIWithoutTerminal(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	result := runProton(t, runOptions{
		args: []string{},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if result.exitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero when launched without a TTY")
	}
	combined := result.stdout + result.stderr
	if !strings.Contains(combined, "refusing to start the TUI without a terminal") {
		t.Fatalf("expected error message about TTY refusal, got: %s", combined)
	}
}

func TestCLIUnknownFlag(t *testing.T) {
	result := runProton(t, runOptions{
		args: []string{"--nonexistent-flag-xyz"},
	})
	if result.exitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero on unknown flag")
	}
	combined := result.stdout + result.stderr
	if !strings.Contains(combined, "not defined") && !strings.Contains(combined, "Usage:") {
		t.Fatalf("expected unknown flag error or usage, got: %s", combined)
	}
}

func TestCLIHeadlessOutputFormats(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	// 1. Plain Text output (default)
	textRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if textRes.exitCode != 0 {
		t.Fatalf("text run failed (code %d): %s\n%s", textRes.exitCode, textRes.stdout, textRes.stderr)
	}
	if !strings.Contains(textRes.stdout, "Hello Coding E2E") {
		t.Fatalf("text output missing file content: %s", textRes.stdout)
	}

	// 2. JSON output
	jsonRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt"}`, "--output", "json"},
		dir:  ws,
		env:  env,
	})
	if jsonRes.exitCode != 0 {
		t.Fatalf("json run failed (code %d): %s\n%s", jsonRes.exitCode, jsonRes.stdout, jsonRes.stderr)
	}

	lines := strings.Split(strings.TrimSpace(jsonRes.stdout), "\n")
	hasCall := false
	hasResult := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Fatalf("invalid json line %q: %v", line, err)
		}
		if evt["kind"] == "tool_call" {
			hasCall = true
		}
		if evt["kind"] == "tool_result" {
			hasResult = true
			if resMap, ok := evt["result"].(map[string]any); ok {
				if output, ok := resMap["output"].(string); ok {
					if !strings.Contains(output, "Hello Coding E2E") {
						t.Fatalf("json result output missing expected content: %s", output)
					}
				}
			}
		}
	}
	if !hasCall || !hasResult {
		t.Fatalf("json output missing call or result event: %s", jsonRes.stdout)
	}
}

func TestCLIHeadlessPromptFromStdin(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	res := runProton(t, runOptions{
		args:  []string{"--headless", "-y"},
		dir:   ws,
		stdin: `/call read_file {"path":"hello.txt"}` + "\n",
		env:   []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("stdin prompt failed (code %d): %s\n%s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "Hello Coding E2E") {
		t.Fatalf("output missing file contents: %s", res.stdout)
	}
}

func TestCLIModeFlags(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	// Mode deny should reject call
	denyRes := runProton(t, runOptions{
		args: []string{"--permission-mode", "deny", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if denyRes.exitCode == 0 {
		t.Fatalf("expected mode deny to exit non-zero, got 0; output: %s", denyRes.stdout)
	}
	combined := denyRes.stdout + denyRes.stderr
	if !strings.Contains(combined, "permission denied") && !strings.Contains(combined, "deny mode") {
		t.Fatalf("expected permission denied on mode deny, got: %s", combined)
	}

	// Mode always-approve should succeed
	allowRes := runProton(t, runOptions{
		args: []string{"--permission-mode", "always-approve", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if allowRes.exitCode != 0 {
		t.Fatalf("expected always-approve to succeed, got code %d: %s", allowRes.exitCode, allowRes.stderr)
	}
	if !strings.Contains(allowRes.stdout, "Hello Coding E2E") {
		t.Fatalf("expected file content: %s", allowRes.stdout)
	}
}
