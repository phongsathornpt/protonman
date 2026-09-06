// Package sandbox applies OS confinement to child processes.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/telemetry"
)

// ErrUnavailable indicates that a requested profile cannot be enforced here.
var ErrUnavailable = errors.New("sandbox unavailable")

// Launcher starts a workspace command under the resolved profile. Implementations
// must bind the returned command to ctx and configure cancellation so the
// command tree stops when ctx is canceled or reaches its deadline.
type Launcher interface {
	Command(ctx context.Context, dir string, command string) (*exec.Cmd, error)
}

// DirectoryLauncher preserves the workspace confinement root while selecting a
// validated working directory inside it.
type DirectoryLauncher interface {
	Launcher
	CommandInDir(ctx context.Context, workspaceRoot string, cwd string, command string) (*exec.Cmd, error)
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
	return l.CommandInDir(ctx, dir, dir, command)
}

// CommandInDir builds a confined child rooted at workspaceRoot and starts it in cwd.
func (l *OSLauncher) CommandInDir(ctx context.Context, workspaceRoot string, cwd string, command string) (*exec.Cmd, error) {
	startedAt := time.Now()
	dir := filepath.Clean(workspaceRoot)
	cwd = filepath.Clean(cwd)
	if rel, err := filepath.Rel(dir, cwd); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%w: working directory escapes workspace", ErrUnavailable)
	}
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
		cmd := bareShell(ctx, cwd, command)
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

	return platformCommand(ctx, l, dir, cwd, command, startedAt, lookPath)
}
