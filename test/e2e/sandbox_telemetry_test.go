package e2e_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestE2ESandboxFlagNetworkRestriction(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// In strict mode, network is blocked for web fetch
	res := runProton(t, runOptions{
		args: []string{
			"--sandbox", "strict",
			"-y",
			"-p", `/call web {"action":"fetch","url":"http://example.com"}`,
		},
		dir: ws,
		env: []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected strict sandbox to block web fetch, got exit 0: %s", res.stdout)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "child network is blocked") && !strings.Contains(combined, "network denied") {
		t.Fatalf("expected network denied error, got: %s", combined)
	}
}

func TestE2ESandboxWorkspaceCommandExecution(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Workspace sandbox mode should allow bash command inside workspace
	res := runProton(t, runOptions{
		args: []string{
			"--sandbox", "workspace",
			"-y",
			"-p", `/call bash {"command":"pwd"}`,
		},
		dir: ws,
		env: []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("workspace sandbox failed (code %d): %s\n%s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, ws) {
		t.Fatalf("expected pwd output to contain workspace %s, got: %s", ws, res.stdout)
	}
}

func TestE2ETelemetryEmissionAndRedaction(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read {"path":"hello.txt"}`},
		dir:  ws,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTONMAN_TELEMETRY=stderr",
		},
	})
	if res.exitCode != 0 {
		t.Fatalf("run with telemetry failed (code %d): %s\n%s", res.exitCode, res.stdout, res.stderr)
	}

	// Verify telemetry events were printed to stderr as JSON
	lines := strings.Split(strings.TrimSpace(res.stderr), "\n")
	foundStarted := false
	foundPermission := false
	foundResult := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var logEntry map[string]any
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			// Might be a non-JSON warning or prefix
			continue
		}
		eventKind, _ := logEntry["event_kind"].(string)
		switch eventKind {
		case "tool_call_started":
			foundStarted = true
		case "permission_resolved":
			foundPermission = true
		case "tool_call_completed":
			foundResult = true
		}

		// Security invariant: raw tool arguments and file paths must NEVER appear in telemetry
		if strings.Contains(line, "hello.txt") {
			t.Fatalf("telemetry leaked sensitive argument 'hello.txt': %s", line)
		}
	}

	if !foundStarted || !foundPermission || !foundResult {
		t.Fatalf("telemetry missing expected lifecycle events (started=%v, permission=%v, completed=%v); stderr:\n%s",
			foundStarted, foundPermission, foundResult, res.stderr)
	}
}
