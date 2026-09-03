package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceResolveRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	workspace, err := New(root, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := workspace.Resolve(context.Background(), "../outside.txt"); !errors.Is(err, ErrOutsideWorkspace) {
		t.Fatalf("Resolve() traversal error = %v, want outside workspace", err)
	}

	linkPath := filepath.Join(root, "linked")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := workspace.Resolve(context.Background(), "linked/secret.txt"); !errors.Is(err, ErrOutsideWorkspace) {
		t.Fatalf("Resolve() symlink error = %v, want outside workspace", err)
	}
}

func TestWorkspaceProtectedPathsCoverDescendantsAndGlobs(t *testing.T) {
	root := t.TempDir()
	workspace, err := New(root, []string{".env", "secrets", "**/*.pem"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "exact file", path: ".env"},
		{name: "protected directory", path: "secrets/token.txt"},
		{name: "root doublestar glob", path: "server.pem"},
		{name: "one-level doublestar glob", path: "certs/server.pem"},
		{name: "nested doublestar glob", path: "certs/dev/server.pem"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := workspace.Resolve(context.Background(), test.path)
			if !errors.Is(err, ErrProtectedPath) {
				t.Fatalf("Resolve() error = %v, want protected path", err)
			}
		})
	}
}

func TestWorkspaceAllowsNewPathInsideRoot(t *testing.T) {
	workspace, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	path, err := workspace.Resolve(context.Background(), "new/nested/file.txt")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !isWithin(workspace.Root(), path) {
		t.Fatalf("resolved path %q escaped root %q", path, workspace.Root())
	}
}
