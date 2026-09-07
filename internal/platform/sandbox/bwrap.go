package sandbox

import (
	"context"
	"os/exec"
)

func bwrapCommand(ctx context.Context, bwrap string, profile Profile, dir string, cwd string, command string) *exec.Cmd {
	// Bubblewrap starts with an empty mount namespace. Expose the host root
	// read-only so the shell, dynamic loader, git, compilers, and normal system
	// tools remain usable, then over-mount only the workspace as writable when
	// the selected profile permits writes.
	args := []string{
		"--die-with-parent",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
	}
	if profile.ReadOnly {
		args = append(args, "--ro-bind", dir, dir)
	} else {
		args = append(args, "--bind", dir, dir)
	}
	args = append(args, "--chdir", cwd)
	if profile.RestrictNetwork {
		args = append(args, "--unshare-net")
	}
	args = append(args, "--", "sh", "-c", command)
	cmd := exec.CommandContext(ctx, bwrap, args...)
	cmd.Dir = cwd
	configureCommand(cmd)
	return cmd
}
