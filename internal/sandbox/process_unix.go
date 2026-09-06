//go:build !windows

package sandbox

import (
	"errors"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

const commandWaitDelay = runtimepolicy.SandboxCommandWaitDelay

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = commandWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		return err
	}
}
