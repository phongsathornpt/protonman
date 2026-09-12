package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
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

func TestWorkspaceReadRoots(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	skillDir := t.TempDir()
	outsideDir := t.TempDir()

	ws, err := New(root, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	skillFile := filepath.Join(skillDir, "prompt.txt")
	if err := os.WriteFile(skillFile, []byte("skill prompt content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Before adding read root, access is denied
	if _, err := ws.ResolveRead(ctx, skillFile); !errors.Is(err, ErrOutsideWorkspace) {
		t.Fatalf("ResolveRead() before AddReadRoot error = %v, want outside workspace", err)
	}

	// Add read root
	if err := ws.AddReadRoot(skillDir); err != nil {
		t.Fatalf("AddReadRoot() error = %v", err)
	}

	// ResolveRead should now succeed for absolute path in read root
	resolved, err := ws.ResolveRead(ctx, skillFile)
	if err != nil {
		t.Fatalf("ResolveRead() error = %v", err)
	}
	if resolved != skillFile {
		t.Fatalf("ResolveRead() = %q, want %q", resolved, skillFile)
	}

	// ResolveRead should fall back to read root for relative path not present in primary root
	resolvedRel, err := ws.ResolveRead(ctx, "prompt.txt")
	if err != nil {
		t.Fatalf("ResolveRead(prompt.txt) error = %v", err)
	}
	if resolvedRel != skillFile {
		t.Fatalf("ResolveRead(prompt.txt) = %q, want %q", resolvedRel, skillFile)
	}

	// Mutating Resolve must STILL reject the skill path (strict write isolation)
	if _, err := ws.Resolve(ctx, skillFile); !errors.Is(err, ErrOutsideWorkspace) {
		t.Fatalf("Resolve() on readRoot path error = %v, want ErrOutsideWorkspace", err)
	}

	// RelRead returns relative path to read root
	rel, err := ws.RelRead(skillFile)
	if err != nil {
		t.Fatalf("RelRead() error = %v", err)
	}
	if rel != "prompt.txt" {
		t.Fatalf("RelRead() = %q, want %q", rel, "prompt.txt")
	}

	// Symlink escape within read root must be rejected
	secretFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(skillDir, "leak_link")
	if err := os.Symlink(secretFile, linkPath); err == nil {
		if _, err := ws.ResolveRead(ctx, linkPath); !errors.Is(err, ErrOutsideWorkspace) {
			t.Fatalf("ResolveRead() symlink escape error = %v, want ErrOutsideWorkspace", err)
		}
	}
}

func TestResolveExistingReadClassifiesMissingPath(t *testing.T) {
	ws, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ws.ResolveExistingRead(context.Background(), "missing/file.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ResolveExistingRead() error = %v, want os.ErrNotExist", err)
	}
	failure := tool.FailureFromError(err)
	if failure == nil || failure.Code != tool.ErrorCodeNotFound {
		t.Fatalf("ResolveExistingRead() failure = %#v, want not_found", failure)
	}
}

func TestResolveExistingReadAcceptsWorkspaceAndReadRootTargets(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	workspaceFile := filepath.Join(ws.Root(), "inside.txt")
	if err := os.WriteFile(workspaceFile, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ws.ResolveExistingRead(context.Background(), "inside.txt"); err != nil || got != workspaceFile {
		t.Fatalf("workspace target = %q, %v; want %q", got, err, workspaceFile)
	}

	readRoot := t.TempDir()
	readFile := filepath.Join(readRoot, "skill.txt")
	if err := os.WriteFile(readFile, []byte("skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ws.AddReadRoot(readRoot); err != nil {
		t.Fatal(err)
	}
	if got, err := ws.ResolveExistingRead(context.Background(), "skill.txt"); err != nil || got != readFile {
		t.Fatalf("read-root target = %q, %v; want %q", got, err, readFile)
	}
}
