package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/projectTHORN/proton/internal/adapters/workspace"
)

const maxEditFileBytes = 16 * 1024 * 1024

func readEditFile(ctx context.Context, workspaceRoot *workspace.Workspace, path string) ([]byte, bool, error) {
	if err := workspaceRoot.CheckAbsolute(ctx, path); err != nil {
		return nil, false, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("open edit target: %w", err)
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
	if err := workspaceRoot.CheckAbsolute(ctx, path); err != nil {
		return nil, false, err
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
	parent := filepath.Dir(resolvedPath)
	if err := workspaceRoot.CheckAbsolute(ctx, parent); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	if err := workspaceRoot.CheckAbsolute(ctx, parent); err != nil {
		return err
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(resolvedPath); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat write target: %w", statErr)
	}

	temporary, err := os.CreateTemp(parent, ".proton-write-*")
	if err != nil {
		return fmt.Errorf("create temporary write: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
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
	if err := os.Rename(temporaryPath, resolvedPath); err != nil {
		return fmt.Errorf("install file: %w", err)
	}
	return nil
}
