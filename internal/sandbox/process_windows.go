//go:build windows

package sandbox

import (
	"errors"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"os"
	"os/exec"
	"strconv"
)

const commandWaitDelay = runtimepolicy.SandboxCommandWaitDelay

func configureCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = commandWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pid := strconv.Itoa(cmd.Process.Pid)
		taskkillErr := exec.Command("taskkill", "/T", "/F", "/PID", pid).Run()
		if taskkillErr == nil {
			return nil
		}
		killErr := cmd.Process.Kill()
		if killErr == nil || errors.Is(killErr, os.ErrProcessDone) {
			return nil
		}
		return errors.Join(taskkillErr, killErr)
	}
}
