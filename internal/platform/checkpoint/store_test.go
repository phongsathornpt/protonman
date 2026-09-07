package checkpoint

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/projectTHORN/proton/internal/core/tool"

	"github.com/projectTHORN/proton/internal/core/workspace"
)

func TestFileStoreCapturesAndRestoresDurably(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	storeRoot := filepath.Join(t.TempDir(), "checkpoints")
	store := newTestStore(t, storeRoot, workspaceRoot)
	path := filepath.Join(workspaceRoot.Root(), "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	id, err := store.Capture(context.Background(), []string{path})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(storeRoot, id+".json")); err != nil {
		t.Fatalf("checkpoint record stat error = %v", err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("mutate file error = %v", err)
	}

	rehydrated := newTestStore(t, storeRoot, workspaceRoot)
	if err := rehydrated.Restore(context.Background(), id); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(contents), "before\n"; got != want {
		t.Fatalf("restored contents = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("restored mode = %o, want %o", got, want)
	}
}

func TestFileStoreRestoresFilesCreatedAfterCapture(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	store := newTestStore(t, filepath.Join(t.TempDir(), "checkpoints"), workspaceRoot)
	path := filepath.Join(workspaceRoot.Root(), "new.txt")

	id, err := store.Capture(context.Background(), []string{path})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("created after checkpoint"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := store.Restore(context.Background(), id); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restored created file stat error = %v, want not-exist", err)
	}
}

func TestFileStoreRestoreRejectsNewerUnownedWorkspaceChanges(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	path := filepath.Join(workspaceRoot.Root(), "tracked.txt")
	if err := os.WriteFile(path, []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRunCheckpoint(t, workspaceRoot.Root(), "init", "--quiet")
	gitRunCheckpoint(t, workspaceRoot.Root(), "config", "user.email", "proton@test.invalid")
	gitRunCheckpoint(t, workspaceRoot.Root(), "config", "user.name", "Proton Test")
	gitRunCheckpoint(t, workspaceRoot.Root(), "add", ".")
	gitRunCheckpoint(t, workspaceRoot.Root(), "commit", "--quiet", "-m", "baseline")

	store := newTestStore(t, filepath.Join(t.TempDir(), "checkpoints"), workspaceRoot)
	id, err := store.Capture(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("newer user change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithMutationSession(context.Background())
	err = store.Restore(ctx, id)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodePreexistingWorkspaceChange {
		t.Fatalf("Restore() error = %v, want preexisting_workspace_change", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "newer user change\n" {
		t.Fatalf("restore overwrote newer user change: %q", contents)
	}
}

func TestFileStoreRejectsUnsafeTargetsAndIDs(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, []string{".env"})
	store := newTestStore(t, filepath.Join(t.TempDir(), "checkpoints"), workspaceRoot)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	directory := filepath.Join(workspaceRoot.Root(), "directory")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	for _, test := range []struct {
		name string
		path string
		want error
	}{
		{name: "protected", path: filepath.Join(workspaceRoot.Root(), ".env"), want: workspace.ErrProtectedPath},
		{name: "outside", path: outside, want: workspace.ErrOutsideWorkspace},
		{name: "directory", path: directory, want: ErrUnsupportedCheckpointTarget},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Capture(context.Background(), []string{test.path})
			if !errors.Is(err, test.want) {
				t.Fatalf("Capture() error = %v, want errors.Is(..., %v)", err, test.want)
			}
		})
	}

	if err := store.Restore(context.Background(), "../../escape"); !errors.Is(err, ErrInvalidCheckpointID) {
		t.Fatalf("Restore() unsafe ID error = %v, want invalid ID", err)
	}
	if err := store.Restore(context.Background(), "checkpoint-missing"); !errors.Is(err, ErrCheckpointNotFound) {
		t.Fatalf("Restore() missing ID error = %v, want not-found", err)
	}
}

func TestFileStoreRejectsSymlinkedStoreInsideWorkspace(t *testing.T) {
	workspaceRoot := newTestWorkspace(t, nil)
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(workspaceRoot.Root(), alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := NewFileStore(filepath.Join(alias, "checkpoints"), workspaceRoot)
	if err == nil {
		t.Fatal("NewFileStore() error = nil, want workspace-bound store rejection")
	}
}

func TestSnapshotFileRejectsParentSymlinkSwap(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside secret"), 0o600); err != nil {
		t.Fatalf("outside WriteFile() error = %v", err)
	}
	if err := os.Symlink(outside, parent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, _, err := snapshotFile(ctx, workspaceRoot, resolved); err == nil {
		t.Fatal("snapshotFile() error = nil, want symlink escape rejection")
	}
}

func TestWriteWorkspaceFileRejectsParentSymlinkSwap(t *testing.T) {
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

	err = writeWorkspaceFile(ctx, workspaceRoot, resolved, fileSnapshot{
		Path:    "safe/file.txt",
		Exists:  true,
		Mode:    0o600,
		Content: []byte("restored secret"),
	})
	if err == nil {
		t.Fatal("writeWorkspaceFile() error = nil, want symlink escape rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file exists or stat failed unexpectedly: %v", err)
	}
}

func gitRunCheckpoint(t *testing.T, root string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	if output, err := exec.Command("git", cmdArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func newTestStore(t *testing.T, root string, workspaceRoot *workspace.Workspace) *FileStore {
	t.Helper()
	store, err := NewFileStore(root, workspaceRoot)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	return store
}

func newTestWorkspace(t *testing.T, protected []string) *workspace.Workspace {
	t.Helper()
	workspaceRoot, err := workspace.New(t.TempDir(), protected)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	return workspaceRoot
}
