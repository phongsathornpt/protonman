package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMutationGuardBlocksDirtyUnownedPath(t *testing.T) {
	ws := newGitWorkspace(t)
	path := filepath.Join(ws.Root(), "tracked.txt")
	if err := os.WriteFile(path, []byte("user change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := WithMutationSession(context.Background())
	if err := ws.GuardWholeFileMutation(ctx, path); !errors.Is(err, ErrPreexistingWorkspaceChange) {
		t.Fatalf("GuardWholeFileMutation() error = %v, want pre-existing change", err)
	}
}

func TestMutationGuardAllowsCleanAndUnrelatedDirtyPaths(t *testing.T) {
	ws := newGitWorkspace(t)
	dirty := filepath.Join(ws.Root(), "tracked.txt")
	clean := filepath.Join(ws.Root(), "clean.txt")
	if err := os.WriteFile(dirty, []byte("user change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := WithMutationSession(context.Background())
	if err := ws.GuardWholeFileMutation(ctx, clean); err != nil {
		t.Fatalf("clean path blocked by unrelated dirty file: %v", err)
	}
}
func TestMutationGuardAllowsOwnedPathAndDetectsLaterUserChange(t *testing.T) {
	ws := newGitWorkspace(t)
	tracked := filepath.Join(ws.Root(), "tracked.txt")
	clean := filepath.Join(ws.Root(), "clean.txt")
	ctx := WithMutationSession(context.Background())

	ws.MarkMutationOwned(ctx, tracked)
	if err := os.WriteFile(tracked, []byte("agent change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ws.GuardWholeFileMutation(ctx, tracked); err != nil {
		t.Fatalf("owned path blocked: %v", err)
	}

	if err := os.WriteFile(clean, []byte("late user change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ws.GuardWholeFileMutation(ctx, clean); !errors.Is(err, ErrPreexistingWorkspaceChange) {
		t.Fatalf("late user change error = %v, want pre-existing change", err)
	}
}

func TestMutationGuardAllowsNonGitWorkspace(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ws.GuardWholeFileMutation(WithMutationSession(context.Background()), path); err != nil {
		t.Fatalf("non-git workspace unexpectedly blocked: %v", err)
	}
}
func newGitWorkspace(t *testing.T) *Workspace {
	t.Helper()
	root := t.TempDir()
	for _, pair := range [][2]string{{"tracked.txt", "original\n"}, {"clean.txt", "clean\n"}} {
		if err := os.WriteFile(filepath.Join(root, pair[0]), []byte(pair[1]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, root, "init", "--quiet")
	gitRun(t, root, "config", "user.email", "proton@test.invalid")
	gitRun(t, root, "config", "user.name", "Proton Test")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "--quiet", "-m", "baseline")
	ws, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", cmdArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
