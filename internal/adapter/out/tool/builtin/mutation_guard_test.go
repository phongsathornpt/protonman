package builtin

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/core/workspace"
)

func TestWriteFileRejectsDirtyUnownedPath(t *testing.T) {
	ws := newCommittedWorkspace(t)
	path := filepath.Join(ws.Root(), "tracked.txt")
	if err := os.WriteFile(path, []byte("user wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithMutationSession(context.Background())
	_, err := NewWriteFile(ws, &recordingCheckpointStore{id: "guard"}).Execute(ctx,
		newJSONCall(t, "write-guard", "write_file", map[string]any{"file_path": "tracked.txt", "content": "agent overwrite\n"}))
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodePreexistingWorkspaceChange {
		t.Fatalf("write error = %v, want preexisting workspace change", err)
	}
	if got := string(readTestFile(t, ws.Root(), "tracked.txt")); got != "user wip\n" {
		t.Fatalf("dirty file changed: %q", got)
	}
}
func TestContextualEditClaimsDirtyPathForLaterOverwrite(t *testing.T) {
	ws := newCommittedWorkspace(t)
	path := filepath.Join(ws.Root(), "tracked.txt")
	if err := os.WriteFile(path, []byte("user wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithMutationSession(context.Background())
	replace := NewSearchReplace(ws, &recordingCheckpointStore{id: "replace"})
	if _, err := replace.Execute(ctx, newJSONCall(t, "replace-claim", "search_replace", map[string]any{
		"file_path": "tracked.txt", "old_string": "user", "new_string": "agent+user",
	})); err != nil {
		t.Fatalf("contextual edit failed: %v", err)
	}
	write := NewWriteFile(ws, &recordingCheckpointStore{id: "write"})
	current := readTestFile(t, ws.Root(), "tracked.txt")
	digest := sha256.Sum256(current)
	if _, err := write.Execute(ctx, newJSONCall(t, "write-owned", "write_file", map[string]any{
		"file_path": "tracked.txt", "content": "owned overwrite\n", "expected_sha256": fmt.Sprintf("%x", digest[:]),
	})); err != nil {
		t.Fatalf("owned overwrite blocked: %v", err)
	}
}

func TestApplyPatchDeleteRejectsDirtyUnownedPath(t *testing.T) {
	ws := newCommittedWorkspace(t)
	path := filepath.Join(ws.Root(), "tracked.txt")
	if err := os.WriteFile(path, []byte("user wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithMutationSession(context.Background())
	patch := "*** Begin Patch\n*** Delete File: tracked.txt\n*** End Patch"
	_, err := NewApplyPatch(ws, &recordingCheckpointStore{id: "patch"}).Execute(ctx,
		newJSONCall(t, "patch-delete", "apply_patch", map[string]any{"patch": patch}))
	if !errors.Is(err, workspace.ErrPreexistingWorkspaceChange) {
		t.Fatalf("delete error = %v, want pre-existing workspace change", err)
	}
}
func TestBashRejectsKnownDirtyMutation(t *testing.T) {
	ws := newCommittedWorkspace(t)
	path := filepath.Join(ws.Root(), "tracked.txt")
	if err := os.WriteFile(path, []byte("user wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithMutationSession(context.Background())
	_, err := NewBash(ws, &recordingLauncher{}).Execute(ctx,
		newJSONCall(t, "bash-delete", "bash", map[string]any{"command": "rm tracked.txt"}))
	if !errors.Is(err, workspace.ErrPreexistingWorkspaceChange) {
		t.Fatalf("bash mutation error = %v, want pre-existing workspace change", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("dirty path was removed: %v", statErr)
	}
}

func newCommittedWorkspace(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "tracked.txt", "original\n")
	writeTestFile(t, ws.Root(), "clean.txt", "clean\n")
	gitRunBuiltin(t, ws.Root(), "init", "--quiet")
	gitRunBuiltin(t, ws.Root(), "config", "user.email", "proton@test.invalid")
	gitRunBuiltin(t, ws.Root(), "config", "user.name", "Proton Test")
	gitRunBuiltin(t, ws.Root(), "add", ".")
	gitRunBuiltin(t, ws.Root(), "commit", "--quiet", "-m", "baseline")
	return ws
}

func gitRunBuiltin(t *testing.T, root string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	if output, err := exec.Command("git", cmdArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
