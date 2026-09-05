// Package sandbox applies OS confinement to child processes.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/telemetry"
)

// ErrUnavailable indicates that a requested profile cannot be enforced here.
var ErrUnavailable = errors.New("sandbox unavailable")

// Launcher starts a workspace command under the resolved profile.
type Launcher interface {
	Command(ctx context.Context, dir string, command string) (*exec.Cmd, error)
}

// OSLauncher wraps bash/sh with sandbox-exec (macOS) or bwrap/unshare (Linux).
type OSLauncher struct {
	Profile  Profile
	LookPath func(string) (string, error)
}

// NewOSLauncher returns a launcher for the profile. Profile Off runs bare.
func NewOSLauncher(profile Profile) *OSLauncher {
	return &OSLauncher{Profile: profile, LookPath: exec.LookPath}
}

// Command builds a confined child. Fail-closed when confinement is required
// but the host cannot apply it.
func (l *OSLauncher) Command(ctx context.Context, dir string, command string) (*exec.Cmd, error) {
	startedAt := time.Now()
	trimmedCommand := strings.TrimSpace(command)
	slog.DebugContext(ctx, "sandbox command requested",
		"profile", l.Profile.Name.String(),
		"confined", l.Profile.Confines(),
		"workspace_fingerprint", telemetry.Fingerprint(dir),
		"command_bytes", len(trimmedCommand),
		"command_fingerprint", telemetry.Fingerprint(trimmedCommand),
	)
	if err := ctx.Err(); err != nil {
		logSandboxFailure(ctx, startedAt, "before_launch", err)
		return nil, fmt.Errorf("before sandbox launch: %w", err)
	}
	command = trimmedCommand
	if command == "" {
		logSandboxFailure(ctx, startedAt, "arguments", errors.New("command_missing"))
		return nil, fmt.Errorf("sandbox command is required")
	}
	if !l.Profile.Confines() {
		cmd := bareShell(ctx, dir, command)
		logSandboxSelected(ctx, startedAt, "shell", cmd.Path)
		return cmd, nil
	}
	if strings.TrimSpace(dir) == "" {
		logSandboxFailure(ctx, startedAt, "workspace", errors.New("workspace_missing"))
		return nil, fmt.Errorf("%w: workspace directory is required", ErrUnavailable)
	}

	lookPath := l.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	switch runtime.GOOS {
	case "darwin":
		if _, err := lookPath("sandbox-exec"); err != nil {
			logSandboxFailure(ctx, startedAt, "launcher_lookup", err)
			return nil, fmt.Errorf("%w: sandbox-exec not found", ErrUnavailable)
		}
		profile := seatbeltProfile(l.Profile, dir)
		cmd := exec.CommandContext(ctx, "sandbox-exec", "-p", profile, "sh", "-c", command)
		cmd.Dir = dir
		configureCommand(cmd)
		logSandboxSelected(ctx, startedAt, "sandbox-exec", cmd.Path)
		return cmd, nil
	case "linux":
		if path, err := lookPath("bwrap"); err == nil {
			cmd := bwrapCommand(ctx, path, l.Profile, dir, command)
			logSandboxSelected(ctx, startedAt, "bwrap", cmd.Path)
			return cmd, nil
		}
		// unshare can isolate networking, but it cannot enforce the filesystem
		// boundary required by every confining Proton profile. Never silently
		// downgrade a requested workspace/read-only/strict sandbox to a bare
		// shell.
		err := fmt.Errorf("%w: bwrap is required for filesystem confinement", ErrUnavailable)
		logSandboxFailure(ctx, startedAt, "launcher_lookup", err)
		return nil, err
	default:
		err := fmt.Errorf("%w: %s is not supported", ErrUnavailable, runtime.GOOS)
		logSandboxFailure(ctx, startedAt, "platform", err)
		return nil, err
	}
}

func logSandboxSelected(ctx context.Context, startedAt time.Time, launcher string, path string) {
	slog.DebugContext(ctx, "sandbox command selected",
		"launcher", launcher,
		"executable", filepath.Base(path),
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
}

func logSandboxFailure(ctx context.Context, startedAt time.Time, phase string, err error) {
	slog.DebugContext(ctx, "sandbox command rejected",
		"phase", phase,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"error_type", fmt.Sprintf("%T", err),
		"context_error", ctx.Err() != nil,
	)
}

func bareShell(ctx context.Context, dir string, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	}
	cmd.Dir = dir
	configureCommand(cmd)
	return cmd
}

func bwrapCommand(ctx context.Context, bwrap string, profile Profile, dir string, command string) *exec.Cmd {
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
	args = append(args, "--chdir", dir)
	if profile.RestrictNetwork {
		args = append(args, "--unshare-net")
	}
	args = append(args, "--", "sh", "-c", command)
	cmd := exec.CommandContext(ctx, bwrap, args...)
	cmd.Dir = dir
	configureCommand(cmd)
	return cmd
}

func seatbeltProfile(profile Profile, dir string) string {
	var builder strings.Builder
	builder.WriteString("(version 1)\n(allow default)\n")
	builder.WriteString("(allow file-read*)\n")
	builder.WriteString("(allow process-exec)\n")
	builder.WriteString("(allow process-fork)\n")
	builder.WriteString("(allow signal)\n")
	builder.WriteString("(allow sysctl-read)\n")
	builder.WriteString("(deny file-write*\n")
	builder.WriteString("  (require-all\n")
	// Seatbelt deny rules take precedence over allow rules. A writable
	// workspace therefore has to be excluded from the deny predicate itself;
	// a later `(allow file-write* (subpath ...))` cannot override a global
	// deny. Read-only profiles intentionally omit this exclusion.
	if !profile.ReadOnly {
		builder.WriteString("    (require-not (subpath " + seatbeltString(dir) + "))\n")
	}
	// Keep the small set of standard writable character devices usable. Basic
	// shell/tool execution commonly redirects to /dev/null or reads randomness;
	// denying these is unrelated to workspace filesystem confinement.
	for _, device := range []string{"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"} {
		builder.WriteString("    (require-not (literal " + seatbeltString(device) + "))\n")
	}
	builder.WriteString("  )\n")
	builder.WriteString(")\n")
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
