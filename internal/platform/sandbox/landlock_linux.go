//go:build linux

package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const landlockRulePathBeneath = 1

func landlockHandledAccess(abi int) uint64 {
	access := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM)
	if abi >= 2 {
		access |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		access |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 5 {
		access |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return access
}

func landlockReadAccess() uint64 {
	return unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
}

func applyLandlock(workspace string, readOnly bool) error {
	abi := probeLandlockABI()
	if abi == 0 {
		return fmt.Errorf("%w: Landlock is unavailable", ErrUnavailable)
	}
	handled := landlockHandledAccess(abi)
	attr := unix.LandlockRulesetAttr{Access_fs: handled}
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("create Landlock ruleset: %w", errno)
	}
	rulesetFD := int(fd)
	defer unix.Close(rulesetFD)

	if err := addLandlockPathRule(rulesetFD, "/", landlockReadAccess()); err != nil {
		return err
	}
	workspaceAccess := landlockReadAccess()
	if !readOnly {
		workspaceAccess = handled
	}
	if err := addLandlockPathRule(rulesetFD, workspace, workspaceAccess); err != nil {
		return err
	}
	for _, device := range []string{"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"} {
		if err := addLandlockPathRule(rulesetFD, device, uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE|unix.LANDLOCK_ACCESS_FS_WRITE_FILE)); err != nil {
			return err
		}
	}

	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("enable no_new_privs: %w", err)
	}
	_, _, errno = unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(rulesetFD), 0, 0)
	if errno != 0 {
		return fmt.Errorf("restrict process with Landlock: %w", errno)
	}
	return nil
}

func addLandlockPathRule(rulesetFD int, path string, allowed uint64) error {
	pathFD, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open Landlock path %q: %w", path, err)
	}
	defer unix.Close(pathFD)
	attr := unix.LandlockPathBeneathAttr{Allowed_access: allowed, Parent_fd: int32(pathFD)}
	_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(rulesetFD), landlockRulePathBeneath, uintptr(unsafe.Pointer(&attr)), 0, 0, 0)
	if errno != 0 {
		if errors.Is(errno, unix.EINVAL) {
			return fmt.Errorf("add Landlock rule for %q: unsupported access mask: %w", path, errno)
		}
		return fmt.Errorf("add Landlock rule for %q: %w", path, errno)
	}
	return nil
}

func execSandboxShell(cwd, command string) error {
	shell, err := execLookPath("sh")
	if err != nil {
		return fmt.Errorf("resolve shell: %w", err)
	}
	if err := os.Chdir(cwd); err != nil {
		return fmt.Errorf("change sandbox cwd: %w", err)
	}
	return unix.Exec(shell, []string{"sh", "-c", command}, os.Environ())
}

var execLookPath = func(file string) (string, error) {
	return exec.LookPath(file)
}

func nativeLandlockCommand(ctx context.Context, profile Profile, dir, cwd, command string) (*exec.Cmd, error) {
	policy := bootstrapPolicy{Workspace: dir, Cwd: cwd, Command: command, ReadOnly: profile.ReadOnly}
	body, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("encode sandbox bootstrap policy: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve Protonman executable: %w", err)
	}
	cmd := exec.CommandContext(ctx, executable)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), sandboxBootstrapEnv+"="+base64.RawURLEncoding.EncodeToString(body))
	configureCommand(cmd)
	if profile.RestrictNetwork {
		uid, gid := os.Getuid(), os.Getgid()
		cmd.SysProcAttr.Cloneflags |= unix.CLONE_NEWUSER | unix.CLONE_NEWNET
		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: uid, Size: 1}}
		cmd.SysProcAttr.GidMappingsEnableSetgroups = false
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: gid, Size: 1}}
	}
	return cmd, nil
}
