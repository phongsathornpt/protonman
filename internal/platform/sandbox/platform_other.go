//go:build !linux && !darwin

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

func platformCommand(ctx context.Context, _ *OSLauncher, _, _, _ string, startedAt time.Time, _ func(string) (string, error)) (*exec.Cmd, error) {
	err := fmt.Errorf("%w: %s is not supported", ErrUnavailable, runtime.GOOS)
	logSandboxFailure(ctx, startedAt, "platform", err)
	return nil, err
}
