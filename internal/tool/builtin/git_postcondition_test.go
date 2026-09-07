package builtin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

func TestParseGitConflictPaths(t *testing.T) {
	output := "UU a.go\x00AA b.go\x00DD c.go\x00AU d.go\x00UA e.go\x00DU f.go\x00UD g.go\x00 M clean.go\x00?? new.go\x00"
	want := []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go"}
	if got := parseGitConflictPaths(output); !slices.Equal(got, want) {
		t.Fatalf("parseGitConflictPaths() = %#v, want %#v", got, want)
	}
}

func TestBashClassifiesMergeConflictAndOwnsConflictPath(t *testing.T) {
	root := t.TempDir()
	runGitTestCommand(t, root, "init", "-q", "-b", "main")
	runGitTestCommand(t, root, "config", "user.email", "proton@example.test")
	runGitTestCommand(t, root, "config", "user.name", "Proton Test")
	writeGitTestFile(t, root, "conflict.txt", "base\n")
	runGitTestCommand(t, root, "add", "conflict.txt")
	runGitTestCommand(t, root, "commit", "-q", "-m", "base")
	runGitTestCommand(t, root, "checkout", "-q", "-b", "feature")
	writeGitTestFile(t, root, "conflict.txt", "feature\n")
	runGitTestCommand(t, root, "commit", "-qam", "feature")
	runGitTestCommand(t, root, "checkout", "-q", "main")
	writeGitTestFile(t, root, "conflict.txt", "main\n")
	runGitTestCommand(t, root, "commit", "-qam", "main")

	ws, err := workspace.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithMutationSession(context.Background())
	result, err := NewBash(ws, &recordingLauncher{}).Execute(ctx, newJSONCall(t, "merge-conflict", "bash", map[string]any{"command": "git merge feature"}))
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeConflict {
		t.Fatalf("merge error = %v, want conflict", err)
	}
	if result.ExitCode == nil || *result.ExitCode == 0 {
		t.Fatalf("merge exit code = %#v, want non-zero", result.ExitCode)
	}
	if !strings.Contains(toolErr.Error(), "conflict.txt") {
		t.Fatalf("conflict error = %q, want path", toolErr.Error())
	}
	status := runGitTestCommand(t, root, "status", "--porcelain=v1")
	if !strings.Contains(status, "UU conflict.txt") {
		t.Fatalf("git status = %q, want unmerged conflict", status)
	}
	conflictPath := filepath.Join(root, "conflict.txt")
	if err := ws.GuardWholeFileMutation(ctx, conflictPath); err != nil {
		t.Fatalf("agent-created conflict path was not owned: %v", err)
	}
}

func runGitTestCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func writeGitTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
