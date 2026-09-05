package e2e_test

import (
	"strings"
	"testing"
)

func TestE2ERollbackNonExistentCheckpoint(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call rollback_checkpoint {"checkpoint_id":"non_existent_cp_12345"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected failure when rolling back non-existent checkpoint, got exit 0")
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "not found") && !strings.Contains(combined, "checkpoint") {
		t.Fatalf("expected checkpoint not found error, got: %s", combined)
	}
}

func TestE2ESessionCustomIDCreationAndLoading(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	customID := "custom-test-session-xyz"

	// Create session with custom ID
	res1 := runProton(t, runOptions{
		args: []string{"-y", "--session", customID, "-p", `/call bash {"command":"echo 'created-custom'"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res1.exitCode != 0 || !strings.Contains(res1.stdout, "created-custom") {
		t.Fatalf("create session failed: %s %s", res1.stdout, res1.stderr)
	}

	// Resume session with custom ID
	res2 := runProton(t, runOptions{
		args: []string{"-y", "--resume", "--session", customID, "-p", `/call bash {"command":"echo 'resumed-custom'"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res2.exitCode != 0 || !strings.Contains(res2.stdout, "resumed-custom") {
		t.Fatalf("resume custom session failed: %s %s", res2.stdout, res2.stderr)
	}
}
