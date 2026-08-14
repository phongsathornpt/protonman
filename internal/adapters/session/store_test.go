package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/projectTHORN/proton/internal/domain/permission"
)

func TestFileStoreRoundTrip(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	want := State{
		PermissionMode: permission.ModeAlwaysApprove.String(),
	}
	if err := store.Save(context.Background(), "session-1", want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, found, err := store.Load(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !found {
		t.Fatal("Load() found = false, want true")
	}
	if got.Version != currentStateVersion {
		t.Fatalf("state version = %d, want %d", got.Version, currentStateVersion)
	}
	if got.PermissionMode != want.PermissionMode {
		t.Fatalf("permission mode = %q, want %q", got.PermissionMode, want.PermissionMode)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("updated_at is zero, want persisted timestamp")
	}
}

func TestFileStoreMissingStateIsNotAnError(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	_, found, err := store.Load(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if found {
		t.Fatal("Load() found = true, want false")
	}
}

func TestFileStoreRejectsPathTraversal(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	_, _, err = store.Load(context.Background(), "../escape")
	if !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("Load() error = %v, want invalid session id", err)
	}
}

func TestFileStoreUsesPrivateStateFile(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	if err := store.Save(context.Background(), "private", State{
		PermissionMode: permission.ModeAsk.String(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "private.json"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("state file mode = %#o, want %#o", got, 0o600)
	}
}
