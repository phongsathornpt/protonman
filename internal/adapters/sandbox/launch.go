// Package sandbox applies OS confinement to child processes.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	domainsandbox "github.com/projectTHORN/proton/internal/domain/sandbox"
)

// ErrUnavailable indicates that a requested profile cannot be enforced here.
var ErrUnavailable = errors.New("sandbox unavailable")

// Launcher starts a workspace command under the resolved profile.
type Launcher interface {
	Command(ctx context.Context, dir string, command string) (*exec.Cmd, error)
}

// OSLauncher wraps bash/sh with sandbox-exec (macOS) or bwrap/unshare (Linux).
type OSLauncher struct {
	Profile  domainsandbox.Profile
	LookPath func(string) (string, error)
}

// NewOSLauncher returns a launcher for the profile. Profile Off runs bare.
func NewOSLauncher(profile domainsandbox.Profile) *OSLauncher {
	return &OSLauncher{Profile: profile, LookPath: exec.LookPath}
}

// Command builds a confined child. Fail-closed when confinement is required
// but the host cannot apply it.
func (l *OSLauncher) Command(ctx context.Context, dir string, command string) (*exec.Cmd, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("before sandbox launch: %w", err)
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("sandbox command is required")
	}
	if !l.Profile.Confines() {
		return bareShell(ctx, dir, command), nil
	}
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("%w: workspace directory is required", ErrUnavailable)
	}

	lookPath := l.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	switch runtime.GOOS {
	case "darwin":
		if _, err := lookPath("sandbox-exec"); err != nil {
			return nil, fmt.Errorf("%w: sandbox-exec not found", ErrUnavailable)
		}
		profile := seatbeltProfile(l.Profile, dir)
		cmd := exec.CommandContext(ctx, "sandbox-exec", "-p", profile, "sh", "-c", command)
		cmd.Dir = dir
		return cmd, nil
	case "linux":
		if path, err := lookPath("bwrap"); err == nil {
			return bwrapCommand(ctx, path, l.Profile, dir, command), nil
		}
		if l.Profile.RestrictNetwork {
			if _, err := lookPath("unshare"); err == nil {
				cmd := exec.CommandContext(ctx, "unshare", "--net", "--", "sh", "-c", command)
				cmd.Dir = dir
				return cmd, nil
			}
			return nil, fmt.Errorf("%w: bwrap and unshare are missing", ErrUnavailable)
		}
		return bareShell(ctx, dir, command), nil
	default:
		return nil, fmt.Errorf("%w: %s is not supported", ErrUnavailable, runtime.GOOS)
	}
}

func bareShell(ctx context.Context, dir string, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	}
	cmd.Dir = dir
	return cmd
}

func bwrapCommand(ctx context.Context, bwrap string, profile domainsandbox.Profile, dir string, command string) *exec.Cmd {
	bind := "--bind"
	if profile.ReadOnly {
		bind = "--ro-bind"
	}
	args := []string{
		"--die-with-parent",
		"--dev", "/dev",
		"--proc", "/proc",
		bind, dir, dir,
		"--chdir", dir,
	}
	if profile.RestrictNetwork {
		args = append(args, "--unshare-net")
	}
	args = append(args, "--", "sh", "-c", command)
	cmd := exec.CommandContext(ctx, bwrap, args...)
	cmd.Dir = dir
	return cmd
}

func seatbeltProfile(profile domainsandbox.Profile, dir string) string {
	var builder strings.Builder
	builder.WriteString("(version 1)\n(allow default)\n")
	builder.WriteString("(allow file-read*)\n")
	builder.WriteString("(allow process-exec)\n")
	builder.WriteString("(allow process-fork)\n")
	builder.WriteString("(allow signal)\n")
	builder.WriteString("(allow sysctl-read)\n")
	if profile.ReadOnly {
		builder.WriteString("(deny file-write*)\n")
	} else {
		builder.WriteString("(allow file-write* (subpath " + seatbeltString(dir) + "))\n")
	}
	if profile.RestrictNetwork {
		builder.WriteString("(deny network*)\n")
	}
	return builder.String()
}

func seatbeltString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
