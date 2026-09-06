package builtin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/sandbox"
)

type recordingLauncher struct {
	dir     string
	cwd     string
	command string
}

func (l *recordingLauncher) Command(ctx context.Context, dir string, command string) (*exec.Cmd, error) {
	return l.CommandInDir(ctx, dir, dir, command)
}

func (l *recordingLauncher) CommandInDir(ctx context.Context, dir string, cwd string, command string) (*exec.Cmd, error) {
	l.dir = dir
	l.cwd = cwd
	l.command = command
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = cwd
	return cmd, nil
}

func TestBashUsesSandboxLauncher(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	launcher := &recordingLauncher{}
	handler := NewBash(workspaceRoot, launcher)
	result, err := handler.Execute(context.Background(), newJSONCall(t, "bash-1", "bash", map[string]any{
		"command": "printf sandboxed",
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if launcher.command != "printf sandboxed" {
		t.Fatalf("launcher command = %q", launcher.command)
	}
	if launcher.dir != workspaceRoot.Root() {
		t.Fatalf("launcher dir = %q, want workspace", launcher.dir)
	}
	if result.Output != "sandboxed" {
		t.Fatalf("output = %q", result.Output)
	}
}

var _ sandbox.DirectoryLauncher = (*recordingLauncher)(nil)

func TestBashRequiresLauncherFailClosed(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	handler := NewBash(workspaceRoot)
	_, err := handler.Execute(context.Background(), newJSONCall(t, "bash-nil", "bash", map[string]any{
		"command": "echo should-not-run",
	}))
	if err == nil {
		t.Fatal("Execute() error = nil, want launcher-required error")
	}
}

func TestBashTruncatesLargeOutput(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	handler := NewBash(workspaceRoot, &recordingLauncher{})
	// Output ~2.5 MiB of data which exceeds maxBashOutputBytes (2 MiB)
	result, err := handler.Execute(context.Background(), newJSONCall(t, "bash-trunc", "bash", map[string]any{
		"command": "python3 -c 'print(\"A\" * (2 * 1024 * 1024 + 1024))' || head -c 2100000 /dev/zero | tr '\\0' 'A'",
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Truncated {
		t.Fatalf("result.Truncated = false, want true")
	}
	if len(result.Output) > maxBashOutputBytes+100 {
		t.Fatalf("output length = %d exceeds max bound", len(result.Output))
	}
}

func TestBashRejectsOversizedCommand(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	launcher := &recordingLauncher{}
	handler := NewBash(workspaceRoot, launcher)
	_, err := handler.Execute(context.Background(), newJSONCall(t, "bash-large", "bash", map[string]any{
		"command": strings.Repeat("x", maxBashCommandBytes+1),
	}))
	if err == nil {
		t.Fatal("Execute() error = nil, want oversized command error")
	}
	if launcher.command != "" {
		t.Fatalf("launcher command = %q, want no process launch", launcher.command)
	}
}

func TestBashRejectsOversizedArguments(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	launcher := &recordingLauncher{}
	handler := NewBash(workspaceRoot, launcher)
	_, err := handler.Execute(context.Background(), newJSONCall(t, "bash-large-args", "bash", map[string]any{
		"command": strings.Repeat("x", maxBashArgumentBytes),
	}))
	if err == nil {
		t.Fatal("Execute() error = nil, want oversized argument error")
	}
	if launcher.command != "" {
		t.Fatalf("launcher command = %q, want no process launch", launcher.command)
	}
}

func TestBashPreservesDeadlineExceeded(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	profile, err := sandbox.NewProfile(sandbox.NameOff, workspaceRoot.Root())
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	handler := NewBash(workspaceRoot, sandbox.NewOSLauncher(profile))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	startedAt := time.Now()
	result, err := handler.Execute(ctx, newJSONCall(t, "bash-deadline", "bash", map[string]any{
		"command": "printf before-timeout; sleep 5 & wait",
	}))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v, want deadline exceeded", err)
	}
	if !strings.Contains(err.Error(), "bash command deadline exceeded") {
		t.Fatalf("Execute() error = %q, want deadline-specific context", err)
	}
	if !strings.Contains(result.Output, "before-timeout") {
		t.Fatalf("output = %q, want partial output before deadline", result.Output)
	}
	if elapsed := time.Since(startedAt); elapsed > 2*time.Second {
		t.Fatalf("deadline execution took %s, want under 2s", elapsed)
	}
}

func TestBashReportsProvenAffectedPaths(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	handler := NewBash(workspaceRoot, &recordingLauncher{})
	result, err := handler.Execute(context.Background(), newJSONCall(t, "bash-path", "bash", map[string]any{
		"command": "printf changed > TODO.md",
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.AffectedPaths) != 1 || result.AffectedPaths[0] != "TODO.md" {
		t.Fatalf("AffectedPaths = %#v, want [TODO.md]", result.AffectedPaths)
	}
}

func TestBashRunsInValidatedWorkspaceCwd(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	if err := os.MkdirAll(filepath.Join(workspaceRoot.Root(), "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := &recordingLauncher{}
	handler := NewBash(workspaceRoot, launcher)
	result, err := handler.Execute(context.Background(), newJSONCall(t, "bash-cwd", "bash", map[string]any{
		"command": "pwd; touch changed.txt",
		"cwd":     "sub",
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if launcher.dir != workspaceRoot.Root() {
		t.Fatalf("sandbox root = %q, want %q", launcher.dir, workspaceRoot.Root())
	}
	wantCwd := filepath.Join(workspaceRoot.Root(), "sub")
	if launcher.cwd != wantCwd {
		t.Fatalf("cwd = %q, want %q", launcher.cwd, wantCwd)
	}
	if !strings.Contains(result.Output, wantCwd) {
		t.Fatalf("output = %q, want cwd", result.Output)
	}
}

func TestBashNormalizesAffectedPathsAgainstCwd(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	if err := os.MkdirAll(filepath.Join(workspaceRoot.Root(), "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	handler := NewBash(workspaceRoot, &recordingLauncher{})
	result, err := handler.Execute(context.Background(), newJSONCall(t, "bash-cwd-path", "bash", map[string]any{
		"command": "touch changed.txt",
		"cwd":     "sub",
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.AffectedPaths) != 1 || result.AffectedPaths[0] != filepath.Join("sub", "changed.txt") {
		t.Fatalf("AffectedPaths = %#v", result.AffectedPaths)
	}
}

func TestBashRejectsCwdOutsideWorkspace(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	handler := NewBash(workspaceRoot, &recordingLauncher{})
	_, err := handler.Execute(context.Background(), newJSONCall(t, "bash-cwd-out", "bash", map[string]any{
		"command": "pwd",
		"cwd":     "../outside",
	}))
	if err == nil {
		t.Fatal("Execute() error = nil, want workspace boundary error")
	}
}

func TestBashPerCallTimeoutCannotRunPastRequestedBudget(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	profile, err := sandbox.NewProfile(sandbox.NameOff, workspaceRoot.Root())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewBash(workspaceRoot, sandbox.NewOSLauncher(profile))
	started := time.Now()
	_, err = handler.Execute(context.Background(), newJSONCall(t, "bash-timeout", "bash", map[string]any{
		"command":         "sleep 5",
		"timeout_seconds": 1,
	}))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("elapsed = %v, want bounded near 1s", elapsed)
	}
}
