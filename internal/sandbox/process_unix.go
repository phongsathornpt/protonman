//go:build !windows

package sandbox

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
	"time"
)

const commandWaitDelay = 2 * time.Second

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
