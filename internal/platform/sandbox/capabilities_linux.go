//go:build linux

package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

// ProbeCapabilities inspects Linux confinement features without changing the
// current process namespaces or security policy.
func ProbeCapabilities() Capabilities {
	caps := Capabilities{Native: true}
	caps.LandlockABI = probeLandlockABI()
	caps.UserNamespaces = userNamespacesConfigured()
	_, err := exec.LookPath("bwrap")
	caps.Bubblewrap = err == nil
	return caps
}

func probeLandlockABI() int {
	r0, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0
	}
	return int(r0)
}
func userNamespacesConfigured() bool {
	if !sysctlEnabled("/proc/sys/kernel/unprivileged_userns_clone", true) {
		return false
	}
	if sysctlEnabled("/proc/sys/kernel/apparmor_restrict_unprivileged_userns", false) {
		return false
	}
	return true
}

func sysctlEnabled(path string, defaultValue bool) bool {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultValue
	}
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(body)) != "0"
}
