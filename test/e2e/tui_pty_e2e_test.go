package e2e_test

import (
	"strings"
	"testing"
)

func TestE2ETUIWithoutForcedTTYFails(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Without PROTON_FORCE_TTY, running in non-terminal must fail
	res := runProton(t, runOptions{
		args: []string{},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected non-zero exit when launching TUI without terminal, got 0")
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "refusing to start the TUI without a terminal") {
		t.Fatalf("expected terminal refusal message, got: %s", combined)
	}
}
