package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
)

// ErrSymlinkPath indicates that a mutation-sensitive path contains a symlink
// component. File mutations intentionally reject symlink parents so the
// directory validated by policy is the same directory later used for I/O.
var ErrSymlinkPath = errors.New("symlink path component is not allowed")

// OpenParentNoSymlinks opens and pins the parent directory of an absolute,
// policy-checked workspace file. Each path component is opened relative to the
// previously pinned directory and verified with SameFile, closing the
// check-then-use window between inspection and descent. When create is true,
// missing parent directories are created one component at a time.
//
// The caller owns the returned Root and must close it.
func (w *Workspace) OpenParentNoSymlinks(ctx context.Context, path string, create bool) (*os.Root, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("before opening workspace parent: %w", err)
	}
	if !filepath.IsAbs(path) {
		return nil, "", newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			"path must be absolute",
			ErrOutsideWorkspace,
		)
	}
	path = filepath.Clean(path)
	if err := w.CheckAbsolute(ctx, path); err != nil {
		return nil, "", err
	}
	relative, err := filepath.Rel(w.root, path)
	if err != nil {
		return nil, "", fmt.Errorf("relative workspace path: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, "", newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			fmt.Sprintf("path is outside workspace: %q", path),
			ErrOutsideWorkspace,
		)
	}
	base := filepath.Base(relative)
	parent := filepath.Dir(relative)

	current, err := os.OpenRoot(w.root)
	if err != nil {
		return nil, "", fmt.Errorf("open workspace root: %w", err)
	}
	if parent == "." {
		return current, base, nil
	}

	for component := range strings.SplitSeq(parent, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		if component == ".." {
			_ = current.Close()
			return nil, "", newBoundaryError(
				tool.ErrorCodeOutsideWorkspace,
				fmt.Sprintf("path is outside workspace: %q", path),
				ErrOutsideWorkspace,
			)
		}
		if err := ctx.Err(); err != nil {
			_ = current.Close()
			return nil, "", fmt.Errorf("while opening workspace parent: %w", err)
		}

		info, err := current.Lstat(component)
		if errors.Is(err, os.ErrNotExist) && create {
			if mkdirErr := current.Mkdir(component, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				_ = current.Close()
				return nil, "", fmt.Errorf("create workspace directory %q: %w", component, mkdirErr)
			}
			info, err = current.Lstat(component)
		}
		if err != nil {
			_ = current.Close()
			return nil, "", fmt.Errorf("inspect workspace directory %q: %w", component, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			_ = current.Close()
			return nil, "", fmt.Errorf("%w: %q", ErrSymlinkPath, component)
		}
		if !info.IsDir() {
			_ = current.Close()
			return nil, "", fmt.Errorf("workspace path component %q is not a directory", component)
		}

		next, err := current.OpenRoot(component)
		if err != nil {
			_ = current.Close()
			return nil, "", fmt.Errorf("open workspace directory %q: %w", component, err)
		}
		openedInfo, err := next.Stat(".")
		if err != nil {
			_ = next.Close()
			_ = current.Close()
			return nil, "", fmt.Errorf("stat opened workspace directory %q: %w", component, err)
		}
		if !os.SameFile(info, openedInfo) {
			_ = next.Close()
			_ = current.Close()
			return nil, "", fmt.Errorf("workspace path component %q changed during validation", component)
		}
		_ = current.Close()
		current = next
	}
	return current, base, nil
}
