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
	return exec.CommandContext(ctx, "sh", "-c", command), nil
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
