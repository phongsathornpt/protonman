//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

func platformCommand(ctx context.Context, launcher *OSLauncher, dir, cwd, command string, startedAt time.Time, lookPath func(string) (string, error)) (*exec.Cmd, error) {
	caps := ProbeCapabilities()
	if caps.LandlockABI > 0 && (!launcher.Profile.RestrictNetwork || caps.UserNamespaces) {
		cmd, err := nativeLandlockCommand(ctx, launcher.Profile, dir, cwd, command)
		if err != nil {
			logSandboxFailure(ctx, startedAt, "native_landlock", err)
			return nil, err
		}
		logSandboxSelected(ctx, startedAt, "landlock", cmd.Path)
		return cmd, nil
	}
	if path, err := lookPath("bwrap"); err == nil {
		cmd := bwrapCommand(ctx, path, launcher.Profile, dir, cwd, command)
		logSandboxSelected(ctx, startedAt, "bwrap", cmd.Path)
		return cmd, nil
	}
	err := fmt.Errorf("%w: bwrap is required for filesystem confinement (native probe: landlock_abi=%d user_namespaces=%t)", ErrUnavailable, caps.LandlockABI, caps.UserNamespaces)
	logSandboxFailure(ctx, startedAt, "launcher_lookup", err)
	return nil, err
}
