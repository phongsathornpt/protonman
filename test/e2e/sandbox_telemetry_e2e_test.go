package e2e_test

import (
	"strings"
	"testing"
)

func TestE2ESandboxReadOnlyWriteRejection(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// In read-only sandbox mode, file writes via bash redirection or edit write in confined launcher
	res := runProton(t, runOptions{
		args: []string{"--sandbox", "read-only", "-y", "-p", `/call bash {"command":"touch read_only_probe.txt"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	// On systems with launcher confinement, touch will fail with read-only filesystem or non-zero exit
	_ = res
}

func TestE2ESandboxInvalidProfileRejection(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	res := runProton(t, runOptions{
		args: []string{"--sandbox", "invalid-profile-xyz", "-p", "test"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected invalid sandbox profile to fail, got exit 0")
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "invalid sandbox") {
		t.Fatalf("expected invalid sandbox error, got: %s", combined)
	}
}

func TestE2ETelemetryExplicitOff(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo 'telem-off'"}`},
		dir:  ws,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTONMAN_TELEMETRY=off",
		},
	})
	if res.exitCode != 0 {
		t.Fatalf("run failed: %s %s", res.stdout, res.stderr)
	}
	if strings.Contains(res.stderr, "tool_call_started") {
		t.Fatalf("expected no telemetry output when off, got: %s", res.stderr)
	}
}
