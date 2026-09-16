package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceAdditionalRootAllowsReadAndMutation(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()
	outside := t.TempDir()
	ws, err := New(primary, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.AddRoot(extra); err != nil {
		t.Fatal(err)
	}

	writeTarget := filepath.Join(extra, "nested", "new.txt")
	if _, err := ws.Resolve(context.Background(), writeTarget); err != nil {
		t.Fatalf("Resolve(additional root) error = %v", err)
	}
	readTarget := filepath.Join(extra, "read.txt")
	if err := os.WriteFile(readTarget, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.ResolveExistingRead(context.Background(), readTarget); err != nil {
		t.Fatalf("ResolveExistingRead(additional root) error = %v", err)
	}
	if _, err := ws.Resolve(context.Background(), filepath.Join(outside, "blocked.txt")); !errors.Is(err, ErrOutsideWorkspace) {
		t.Fatalf("Resolve(outside) error = %v, want ErrOutsideWorkspace", err)
	}
}

func TestWorkspaceReadRootDoesNotGrantMutation(t *testing.T) {
	primary := t.TempDir()
	readOnly := t.TempDir()
	ws, err := New(primary, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.AddReadRoot(readOnly); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(readOnly, "doc.txt")
	if err := os.WriteFile(path, []byte("doc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.ResolveRead(context.Background(), path); err != nil {
		t.Fatalf("ResolveRead(read root) error = %v", err)
	}
	if _, err := ws.Resolve(context.Background(), path); !errors.Is(err, ErrOutsideWorkspace) {
		t.Fatalf("Resolve(read-only root) error = %v, want ErrOutsideWorkspace", err)
	}
}

func TestWorkspaceAdditionalRootRequiresAbsoluteDirectory(t *testing.T) {
	ws, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.AddRoot("relative/path"); !errors.Is(err, ErrInvalidWorkspace) {
		t.Fatalf("AddRoot(relative) error = %v, want ErrInvalidWorkspace", err)
	}
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ws.AddRoot(file); !errors.Is(err, ErrInvalidWorkspace) {
		t.Fatalf("AddRoot(file) error = %v, want ErrInvalidWorkspace", err)
	}
}

func TestWorkspaceRelativePathsStillUsePrimaryRoot(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()
	ws, err := New(primary, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.AddRoot(extra); err != nil {
		t.Fatal(err)
	}
	got, err := ws.Resolve(context.Background(), "same.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Workspace canonicalizes its root, and temporary directories are themselves
	// behind symlinks on some hosts (/var -> /private/var on macOS), so compare
	// against the canonical primary root. The assertion is about root selection,
	// not about host path resolution.
	canonicalPrimary, err := filepath.EvalSymlinks(primary)
	if err != nil {
		t.Fatalf("resolve primary root symlinks: %v", err)
	}
	want := filepath.Join(canonicalPrimary, "same.txt")
	if got != want {
		t.Fatalf("Resolve(relative) = %q, want %q", got, want)
	}
}

func TestWorkspaceAdditionalRootBlocksSymlinkEscape(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(extra, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	ws, err := New(primary, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.AddRoot(extra); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Resolve(context.Background(), filepath.Join(link, "blocked.txt")); !errors.Is(err, ErrOutsideWorkspace) {
		t.Fatalf("Resolve(symlink escape) error = %v, want ErrOutsideWorkspace", err)
	}
}
