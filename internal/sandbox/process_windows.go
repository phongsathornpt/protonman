//go:build windows

package sandbox

import (
	"os/exec"
	"time"
)

const commandWaitDelay = 2 * time.Second

func configureCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = commandWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
