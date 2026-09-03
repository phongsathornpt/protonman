// Package checkpoint persists bounded workspace file snapshots outside the workspace.
package checkpoint

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/projectTHORN/proton/internal/workspace"
	"github.com/projectTHORN/proton/internal/tool"
)

const (
	checkpointVersion     = 1
	maxCheckpointFileSize = 16 * 1024 * 1024
	maxCheckpointSize     = 64 * 1024 * 1024
)

var (
	// ErrInvalidCheckpointID indicates that an ID cannot name a checkpoint file.
	ErrInvalidCheckpointID = errors.New("invalid checkpoint id")
	// ErrCheckpointNotFound indicates that no checkpoint exists for an ID.
	ErrCheckpointNotFound = errors.New("checkpoint not found")
	// ErrUnsupportedCheckpointTarget indicates that a target is not a regular file.
	ErrUnsupportedCheckpointTarget = errors.New("unsupported checkpoint target")
)

// FileStore stores checkpoint records as private, atomically written JSON files.
type FileStore struct {
	root      string
	workspace *workspace.Workspace
	mu        sync.Mutex
}

type record struct {
	Version       int            `json:"version"`
	ID            string         `json:"id"`
	WorkspaceRoot string         `json:"workspace_root"`
	CreatedAt     time.Time      `json:"created_at"`
	Entries       []fileSnapshot `json:"entries"`
}

type fileSnapshot struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Mode    uint32 `json:"mode,omitempty"`
	Content []byte `json:"content,omitempty"`
}

// NewFileStore creates a checkpoint store rooted outside the workspace.
func NewFileStore(root string, workspaceRoot *workspace.Workspace) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("checkpoint store root is required")
	}
	if workspaceRoot == nil {
		return nil, fmt.Errorf("checkpoint workspace is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve checkpoint store root: %w", err)
	}
	insideWorkspace, err := pathResolvesWithin(absoluteRoot, workspaceRoot.Root())
	if err != nil {
		return nil, fmt.Errorf("validate checkpoint store root: %w", err)
	}
	if insideWorkspace {
		return nil, fmt.Errorf("checkpoint store must be outside workspace")
	}
	return &FileStore{
		root:      filepath.Clean(absoluteRoot),
		workspace: workspaceRoot,
	}, nil
}

// Capture snapshots the supplied regular files before an edit.
func (s *FileStore) Capture(ctx context.Context, paths []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("before checkpoint capture: %w", err)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("checkpoint paths are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	resolvedPaths, err := s.resolveUniquePaths(ctx, paths)
	if err != nil {
		return "", err
	}
	entries := make([]fileSnapshot, 0, len(resolvedPaths))
	totalBytes := 0
	for _, path := range resolvedPaths {
		entry, bytesRead, err := snapshotFile(ctx, s.workspace, path)
		if err != nil {
			return "", err
		}
		totalBytes += bytesRead
		if totalBytes > maxCheckpointSize {
			return "", fmt.Errorf("checkpoint exceeds %d MiB", maxCheckpointSize/(1024*1024))
		}
		entries = append(entries, entry)
	}

	id, err := newCheckpointID()
	if err != nil {
		return "", err
	}
	checkpoint := record{
		Version:       checkpointVersion,
		ID:            id,
		WorkspaceRoot: s.workspace.Root(),
		CreatedAt:     time.Now().UTC(),
		Entries:       entries,
	}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return "", fmt.Errorf("encode checkpoint: %w", err)
	}
	if len(encoded) > maxCheckpointSize {
		return "", fmt.Errorf("encoded checkpoint exceeds %d MiB", maxCheckpointSize/(1024*1024))
	}

	if err := writePrivateAtomic(ctx, s.root, s.path(id), encoded); err != nil {
		return "", fmt.Errorf("persist checkpoint: %w", err)
	}
	return id, nil
}

// Restore replaces or removes the files recorded by a checkpoint.
func (s *FileStore) Restore(ctx context.Context, id string) error {
	if err := validateCheckpointID(id); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before checkpoint restore: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	checkpoint, err := s.load(id)
	if err != nil {
		return err
	}
	if checkpoint.Version != checkpointVersion {
		return fmt.Errorf("checkpoint version %d is unsupported", checkpoint.Version)
	}
	if filepath.Clean(checkpoint.WorkspaceRoot) != s.workspace.Root() {
		return fmt.Errorf("checkpoint belongs to a different workspace")
	}
	if checkpoint.ID != id {
		return fmt.Errorf("checkpoint record ID does not match its filename")
	}

	resolvedEntries, err := s.validateRestoreEntries(ctx, checkpoint.Entries)
	if err != nil {
		return err
	}
	for _, entry := range resolvedEntries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("during checkpoint restore: %w", err)
		}
		if entry.snapshot.Exists {
			if err := writeWorkspaceFile(ctx, s.workspace, entry.path, entry.snapshot); err != nil {
				return fmt.Errorf("restore %q: %w", entry.snapshot.Path, err)
			}
			continue
		}
		if err := removeWorkspaceFile(ctx, s.workspace, entry.path); err != nil {
			return fmt.Errorf("remove restored %q: %w", entry.snapshot.Path, err)
		}
	}
	return nil
}

func (s *FileStore) resolveUniquePaths(ctx context.Context, paths []string) ([]string, error) {
	unique := make(map[string]struct{}, len(paths))
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		resolvedPath, err := s.workspace.Resolve(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("checkpoint path %q: %w", path, err)
		}
		if _, exists := unique[resolvedPath]; exists {
			continue
		}
		unique[resolvedPath] = struct{}{}
		resolved = append(resolved, resolvedPath)
	}
	sort.Strings(resolved)
	return resolved, nil
}

type resolvedSnapshot struct {
	path     string
	snapshot fileSnapshot
}

func (s *FileStore) validateRestoreEntries(ctx context.Context, entries []fileSnapshot) ([]resolvedSnapshot, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("checkpoint contains no files")
	}
	resolved := make([]resolvedSnapshot, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	totalBytes := 0
	for _, entry := range entries {
		cleanPath := filepath.Clean(entry.Path)
		isParentPath := cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator))
		if filepath.IsAbs(entry.Path) || cleanPath == "." || isParentPath {
			return nil, fmt.Errorf("checkpoint contains unsafe path %q", entry.Path)
		}
		path, err := s.workspace.Resolve(ctx, entry.Path)
		if err != nil {
			return nil, fmt.Errorf("validate checkpoint path %q: %w", entry.Path, err)
		}
		if _, exists := seen[path]; exists {
			return nil, fmt.Errorf("checkpoint contains duplicate path %q", entry.Path)
		}
		seen[path] = struct{}{}
		if !entry.Exists {
			resolved = append(resolved, resolvedSnapshot{path: path, snapshot: entry})
			continue
		}
		if len(entry.Content) > maxCheckpointFileSize {
			return nil, fmt.Errorf("checkpoint file %q exceeds %d MiB", entry.Path, maxCheckpointFileSize/(1024*1024))
		}
		totalBytes += len(entry.Content)
		if totalBytes > maxCheckpointSize {
			return nil, fmt.Errorf("checkpoint exceeds %d MiB", maxCheckpointSize/(1024*1024))
		}
		resolved = append(resolved, resolvedSnapshot{path: path, snapshot: entry})
	}
	return resolved, nil
}

func snapshotFile(ctx context.Context, workspaceRoot *workspace.Workspace, path string) (fileSnapshot, int, error) {
	relativePath, err := checkpointRelativePath(workspaceRoot, path)
	if err != nil {
		return fileSnapshot{}, 0, err
	}
	parentRoot, base, err := workspaceRoot.OpenParentNoSymlinks(ctx, path, false)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{Path: filepath.ToSlash(relativePath)}, 0, nil
	}
	if err != nil {
		return fileSnapshot{}, 0, err
	}
	defer parentRoot.Close()

	info, err := parentRoot.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{Path: filepath.ToSlash(relativePath)}, 0, nil
	}
	if err != nil {
		return fileSnapshot{}, 0, fmt.Errorf("stat checkpoint target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fileSnapshot{}, 0, fmt.Errorf("%w: checkpoint target %q", workspace.ErrSymlinkPath, relativePath)
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, 0, fmt.Errorf("%w: %q", ErrUnsupportedCheckpointTarget, relativePath)
	}
	file, err := parentRoot.Open(base)
	if err != nil {
		return fileSnapshot{}, 0, fmt.Errorf("open checkpoint target: %w", err)
	}
	openedInfo, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return fileSnapshot{}, 0, fmt.Errorf("stat opened checkpoint target: %w", statErr)
	}
	if !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return fileSnapshot{}, 0, fmt.Errorf("checkpoint target %q changed during validation", relativePath)
	}
	if !openedInfo.Mode().IsRegular() {
		_ = file.Close()
		return fileSnapshot{}, 0, fmt.Errorf("%w: %q", ErrUnsupportedCheckpointTarget, relativePath)
	}
	if openedInfo.Size() > maxCheckpointFileSize {
		_ = file.Close()
		return fileSnapshot{}, 0, fmt.Errorf(
			"checkpoint file %q exceeds %d MiB",
			relativePath,
			maxCheckpointFileSize/(1024*1024),
		)
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, maxCheckpointFileSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return fileSnapshot{}, 0, fmt.Errorf("read checkpoint target: %w", readErr)
	}
	if closeErr != nil {
		return fileSnapshot{}, 0, fmt.Errorf("close checkpoint target: %w", closeErr)
	}
	if len(contents) > maxCheckpointFileSize {
		return fileSnapshot{}, 0, fmt.Errorf("checkpoint file %q exceeds %d MiB", relativePath, maxCheckpointFileSize/(1024*1024))
	}
	if err := ctx.Err(); err != nil {
		return fileSnapshot{}, 0, fmt.Errorf("after reading checkpoint target: %w", err)
	}
	return fileSnapshot{
		Path:    filepath.ToSlash(relativePath),
		Exists:  true,
		Mode:    uint32(openedInfo.Mode().Perm()),
		Content: contents,
	}, len(contents), nil
}

func (s *FileStore) load(id string) (record, error) {
	file, err := os.Open(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return record{}, tool.WrapToolError(
			tool.ErrorCodeNotFound,
			fmt.Sprintf("%s: %s", ErrCheckpointNotFound, id),
			ErrCheckpointNotFound,
		)
	}
	if err != nil {
		return record{}, fmt.Errorf("open checkpoint: %w", err)
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, maxCheckpointSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return record{}, fmt.Errorf("read checkpoint: %w", readErr)
	}
	if closeErr != nil {
		return record{}, fmt.Errorf("close checkpoint: %w", closeErr)
	}
	if len(contents) > maxCheckpointSize {
		return record{}, fmt.Errorf("checkpoint exceeds %d MiB", maxCheckpointSize/(1024*1024))
	}
	var checkpoint record
	if err := json.Unmarshal(contents, &checkpoint); err != nil {
		return record{}, fmt.Errorf("decode checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (s *FileStore) path(id string) string {
	return filepath.Join(s.root, id+".json")
}

func isPathWithin(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func pathResolvesWithin(path string, root string) (bool, error) {
	ancestor := filepath.Clean(path)
	for {
		_, err := os.Lstat(ancestor)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return false, err
			}
			return isPathWithin(root, resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return false, fmt.Errorf("no existing ancestor for %q", path)
		}
		ancestor = parent
	}
}

func newCheckpointID() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate checkpoint ID: %w", err)
	}
	return fmt.Sprintf("checkpoint-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(random[:])), nil
}

func validateCheckpointID(id string) error {
	if strings.TrimSpace(id) == "" || filepath.Base(id) != id || !strings.HasPrefix(id, "checkpoint-") {
		return fmt.Errorf("%w: %q", ErrInvalidCheckpointID, id)
	}
	return nil
}

func writePrivateAtomic(ctx context.Context, root string, destination string, contents []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before checkpoint write: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create checkpoint store: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return fmt.Errorf("protect checkpoint store: %w", err)
	}
	temporary, err := os.CreateTemp(root, ".checkpoint-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary checkpoint: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("protect temporary checkpoint: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync checkpoint: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close checkpoint: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before installing checkpoint: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("install checkpoint: %w", err)
	}
	return nil
}

func writeWorkspaceFile(
	ctx context.Context,
	workspaceRoot *workspace.Workspace,
	path string,
	snapshot fileSnapshot,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before restoring file: %w", err)
	}
	parentRoot, base, err := workspaceRoot.OpenParentNoSymlinks(ctx, path, true)
	if err != nil {
		return err
	}
	defer parentRoot.Close()
	if info, statErr := parentRoot.Lstat(base); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: restore target %q", workspace.ErrSymlinkPath, path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: restore target %q is not a regular file", ErrUnsupportedCheckpointTarget, path)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat restore target: %w", statErr)
	}

	temporary, temporaryName, err := createCheckpointRootTemp(parentRoot, ".proton-restore-")
	if err != nil {
		return fmt.Errorf("create restore temporary: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = parentRoot.Remove(temporaryName)
	}()
	mode := os.FileMode(snapshot.Mode)
	if mode == 0 {
		mode = 0o644
	}
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return fmt.Errorf("set restore mode: %w", err)
	}
	if _, err := temporary.Write(snapshot.Content); err != nil {
		return fmt.Errorf("write restore contents: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync restore contents: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close restore temporary: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before installing restore: %w", err)
	}
	if err := parentRoot.Rename(temporaryName, base); err != nil {
		return fmt.Errorf("install restore: %w", err)
	}
	return nil
}

func removeWorkspaceFile(ctx context.Context, workspaceRoot *workspace.Workspace, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before removing restored file: %w", err)
	}
	parentRoot, base, err := workspaceRoot.OpenParentNoSymlinks(ctx, path, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer parentRoot.Close()
	info, err := parentRoot.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat restore removal: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: restore target %q is a directory", ErrUnsupportedCheckpointTarget, path)
	}
	if err := parentRoot.Remove(base); err != nil {
		return fmt.Errorf("remove restore target: %w", err)
	}
	return nil
}

func checkpointRelativePath(workspaceRoot *workspace.Workspace, path string) (string, error) {
	relative, err := filepath.Rel(workspaceRoot.Root(), filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("relative checkpoint path: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("checkpoint path %q is outside workspace", path)
	}
	return relative, nil
}

func createCheckpointRootTemp(root *os.Root, prefix string) (*os.File, string, error) {
	for attempt := 0; attempt < 16; attempt++ {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", fmt.Errorf("generate restore temporary name: %w", err)
		}
		name := prefix + hex.EncodeToString(random[:])
		file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("could not allocate a unique restore temporary file")
}

var _ Store = (*FileStore)(nil)
