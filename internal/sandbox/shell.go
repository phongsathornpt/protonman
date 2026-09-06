package sandbox

import (
	"context"
	"os/exec"
	"runtime"
)

func bareShell(ctx context.Context, dir string, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	}
	cmd.Dir = dir
	configureCommand(cmd)
	return cmd
}
