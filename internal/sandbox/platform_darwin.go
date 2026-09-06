//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

func platformCommand(ctx context.Context, launcher *OSLauncher, dir, cwd, command string, startedAt time.Time, lookPath func(string) (string, error)) (*exec.Cmd, error) {
	if _, err := lookPath("sandbox-exec"); err != nil {
		logSandboxFailure(ctx, startedAt, "launcher_lookup", err)
		return nil, fmt.Errorf("%w: sandbox-exec not found", ErrUnavailable)
	}
	profile := seatbeltProfile(launcher.Profile, dir)
	cmd := exec.CommandContext(ctx, "sandbox-exec", "-p", profile, "sh", "-c", command)
	cmd.Dir = cwd
	configureCommand(cmd)
	logSandboxSelected(ctx, startedAt, "sandbox-exec", cmd.Path)
	return cmd, nil
}
