package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReservedInternalPathIsDeniedToWorkspaceAccess(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	internal := filepath.Join(root, ".protonman")
	if err := ws.ReserveInternalPath(internal); err != nil {
		t.Fatal(err)
	}

	_, err = ws.Resolve(context.Background(), filepath.Join(".protonman", "config.toml"))
	if !errors.Is(err, ErrProtectedPath) {
		t.Fatalf("Resolve(internal) error = %v, want protected path", err)
	}
	_, err = ws.ResolveRead(context.Background(), filepath.Join(".protonman", "config.toml"))
	if !errors.Is(err, ErrProtectedPath) {
		t.Fatalf("ResolveRead(internal) error = %v, want protected path", err)
	}
}

func TestReservedInternalPathRejectsWorkspaceSymlinkAlias(t *testing.T) {
	root := t.TempDir()
	internal := filepath.Join(root, ".protonman")
	if err := os.Mkdir(internal, 0o700); err != nil {
		t.Fatal(err)
	}
	ws, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.ReserveInternalPath(internal); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "internal-link")
	if err := os.Symlink(internal, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err = ws.ResolveRead(context.Background(), filepath.Join("internal-link", "config.toml"))
	if !errors.Is(err, ErrProtectedPath) {
		t.Fatalf("ResolveRead(alias) error = %v, want protected path", err)
	}
}
