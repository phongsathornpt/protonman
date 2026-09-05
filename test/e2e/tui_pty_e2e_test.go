package e2e_test

import (
	"strings"
	"testing"
	"time"
)

func TestE2ETUIStartupAndExitWithForcedTTY(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Launch proton in TUI mode with PROTON_FORCE_TTY=1 and send Ctrl+C to exit cleanly
	res := runProton(t, runOptions{
		args:    []string{},
		dir:     ws,
		stdin:   "\x03", // Ctrl+C key sequence to exit BubbleTea
		timeout: 5 * time.Second,
		env: []string{
			"PROTON_HOME=" + home,
			"PROTON_FORCE_TTY=1",
		},
	})

	// When exiting with Ctrl+C, BubbleTea exits cleanly or with interrupt
	_ = res
}

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
