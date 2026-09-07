package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReadFileRejectsParentSymlinkSwap(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	workspaceRoot, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(root, "safe")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "file.txt")
	if err := os.WriteFile(path, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := workspaceRoot.ResolveRead(ctx, "safe/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, parent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	file, err := workspaceRoot.OpenReadFile(ctx, resolved)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Fatal("OpenReadFile() allowed swapped parent symlink outside workspace")
	}
}

func TestOpenParentNoSymlinksCreatesAndPinsNestedParents(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspaceRoot, err := New(root, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	path := filepath.Join(workspaceRoot.Root(), "a", "b", "file.txt")

	parent, base, err := workspaceRoot.OpenParentNoSymlinks(ctx, path, true)
	if err != nil {
		t.Fatalf("OpenParentNoSymlinks() error = %v", err)
	}
	defer parent.Close()
	if base != "file.txt" {
		t.Fatalf("base = %q, want file.txt", base)
	}
	file, err := parent.OpenFile(base, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("created file stat error = %v", err)
	}
}

func TestOpenParentNoSymlinksRejectsInWorkspaceSymlinkParent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspaceRoot, err := New(root, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Symlink("real", filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	resolved, err := workspaceRoot.Resolve(ctx, "linked/file.txt")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	parent, _, err := workspaceRoot.OpenParentNoSymlinks(ctx, resolved, false)
	if parent != nil {
		_ = parent.Close()
	}
	if !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("OpenParentNoSymlinks() error = %v, want symlink-path rejection", err)
	}
}
