package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/projectTHORN/proton/internal/workspace"
)

func TestAtomicWriteResolvedRejectsParentSymlinkSwap(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	workspaceRoot, err := workspace.New(root, nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	parent := filepath.Join(root, "safe")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	resolved, err := workspaceRoot.Resolve(ctx, "safe/file.txt")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := os.Symlink(outside, parent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := atomicWriteResolved(ctx, workspaceRoot, resolved, []byte("escape")); err == nil {
		t.Fatal("atomicWriteResolved() error = nil, want symlink escape rejection")
	}
	_, err = os.Stat(filepath.Join(outside, "file.txt"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file exists or stat failed unexpectedly: %v", err)
	}
}

func TestAtomicWriteResolvedRejectsParentSymlinkInsideWorkspace(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspaceRoot, err := workspace.New(root, nil)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	realParent := filepath.Join(root, "real")
	if err := os.Mkdir(realParent, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	linkedParent := filepath.Join(root, "linked")
	if err := os.Symlink("real", linkedParent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	resolved, err := workspaceRoot.Resolve(ctx, "linked/file.txt")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	err = atomicWriteResolved(ctx, workspaceRoot, resolved, []byte("blocked"))
	if !errors.Is(err, workspace.ErrSymlinkPath) {
		t.Fatalf("atomicWriteResolved() error = %v, want symlink-path rejection", err)
	}
	if _, err := os.Stat(filepath.Join(realParent, "file.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("linked target was written or stat failed unexpectedly: %v", err)
	}
}
