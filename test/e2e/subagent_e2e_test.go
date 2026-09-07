package e2e_test

import (
	"strings"
	"testing"
)

func TestE2ESubagentDelegationSuccess(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call delegate_task {"task":"Explore repository structure","profile":"int","timeout_seconds":30}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("delegation failed: %s %s", res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "spawned int-") || !strings.Contains(res.stdout, "int · queued") {
		t.Fatalf("missing async subagent handle: %s", res.stdout)
	}
}

func TestE2ESubagentDelegationInvalidArguments(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// 1. Missing task
	resMissing := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call delegate_task {"profile":"int"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if resMissing.exitCode == 0 || !strings.Contains(resMissing.stdout+resMissing.stderr, "missing property 'task'") {
		t.Fatalf("expected task required error, got: %s %s", resMissing.stdout, resMissing.stderr)
	}

	// 2. Unknown profile
	resProfile := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call delegate_task {"task":"Do work","profile":"unknown_profile_xyz"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if resProfile.exitCode == 0 || !strings.Contains(resProfile.stdout+resProfile.stderr, "value must be one of") {
		t.Fatalf("expected unknown profile error, got: %s %s", resProfile.stdout, resProfile.stderr)
	}
}

func TestE2ESubagentDelegationRejectsExcessiveTimeout(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call delegate_task {"task":"Do work","profile":"int","timeout_seconds":86401}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode == 0 || !strings.Contains(res.stdout+res.stderr, "timeout_seconds") {
		t.Fatalf("expected timeout validation error, got: %s %s", res.stdout, res.stderr)
	}
}
