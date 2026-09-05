package builtin

import (
	"context"
	"os/exec"
	"testing"

	"github.com/projectTHORN/proton/internal/sandbox"
)

type recordingLauncher struct {
	dir     string
	command string
}

func (l *recordingLauncher) Command(ctx context.Context, dir string, command string) (*exec.Cmd, error) {
	l.dir = dir
	l.command = command
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
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

var _ sandbox.Launcher = (*recordingLauncher)(nil)

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
