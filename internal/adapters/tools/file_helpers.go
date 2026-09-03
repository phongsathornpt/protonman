package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/projectTHORN/proton/internal/workspace"
)

const maxEditFileBytes = 16 * 1024 * 1024

func readEditFile(ctx context.Context, workspaceRoot *workspace.Workspace, path string) ([]byte, bool, error) {
	parentRoot, base, err := workspaceRoot.OpenParentNoSymlinks(ctx, path, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer parentRoot.Close()

	info, err := parentRoot.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect edit target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("%w: edit target %q", workspace.ErrSymlinkPath, path)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("edit target %q is not a regular file", path)
	}

	file, err := parentRoot.Open(base)
	if err != nil {
		return nil, false, fmt.Errorf("open edit target: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, false, fmt.Errorf("stat opened edit target: %w", err)
	}
	if !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, false, fmt.Errorf("edit target %q changed during validation", path)
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxEditFileBytes+1))
	closeErr := file.Close()
	if err != nil {
		return nil, false, fmt.Errorf("read edit target: %w", err)
	}
	if closeErr != nil {
		return nil, false, fmt.Errorf("close edit target: %w", closeErr)
	}
	if len(contents) > maxEditFileBytes {
		return nil, false, fmt.Errorf("edit target exceeds %d MiB", maxEditFileBytes/(1024*1024))
	}
	if err := ctx.Err(); err != nil {
		return nil, false, fmt.Errorf("after reading edit target: %w", err)
	}
	return contents, true, nil
}

func atomicWrite(ctx context.Context, workspaceRoot *workspace.Workspace, path string, contents []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before writing file: %w", err)
	}
	resolvedPath, err := workspaceRoot.Resolve(ctx, path)
	if err != nil {
		return err
	}
	return atomicWriteResolved(ctx, workspaceRoot, resolvedPath, contents)
}

// atomicWriteResolved installs contents at an already policy-checked absolute
// workspace path. The parent directory is reopened through the workspace's
// pinned no-symlink traversal before the temporary file is created, keeping
// the temp write and final rename on the validated directory handle.
func atomicWriteResolved(ctx context.Context, workspaceRoot *workspace.Workspace, resolvedPath string, contents []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before writing file: %w", err)
	}
	parentRoot, base, err := workspaceRoot.OpenParentNoSymlinks(ctx, resolvedPath, true)
	if err != nil {
		return err
	}
	defer parentRoot.Close()

	mode := os.FileMode(0o644)
	if info, statErr := parentRoot.Lstat(base); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: write target %q", workspace.ErrSymlinkPath, resolvedPath)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("write target %q is not a regular file", resolvedPath)
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat write target: %w", statErr)
	}

	temporary, temporaryName, err := createRootTemp(parentRoot, ".proton-write-")
	if err != nil {
		return fmt.Errorf("create temporary write: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = parentRoot.Remove(temporaryName)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("set file mode: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return fmt.Errorf("write file contents: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync file contents: %w", err)
	}
	closeErr := temporary.Close()
	closed = true
	if closeErr != nil {
		return fmt.Errorf("close temporary write: %w", closeErr)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before installing file: %w", err)
	}
	if err := parentRoot.Rename(temporaryName, base); err != nil {
		return fmt.Errorf("install file: %w", err)
	}
	return nil
}

func createRootTemp(root *os.Root, prefix string) (*os.File, string, error) {
	for attempt := 0; attempt < 16; attempt++ {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", fmt.Errorf("generate temporary name: %w", err)
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
	return nil, "", fmt.Errorf("could not allocate a unique temporary file")
}
