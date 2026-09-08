// Package pathutil provides canonical filesystem identity checks.
package pathutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Canonical resolves a path through its nearest existing ancestor and then
// reapplies any missing suffix. This preserves identity for paths whose leaf
// does not exist yet while still resolving symlinked ancestors.
func Canonical(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	absolute = filepath.Clean(absolute)

	ancestor := absolute
	missing := make([]string, 0, 4)
	for {
		_, statErr := os.Lstat(ancestor)
		if statErr == nil {
			resolved, resolveErr := filepath.EvalSymlinks(ancestor)
			if resolveErr != nil {
				return "", fmt.Errorf("resolve path symlinks: %w", resolveErr)
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}

		if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect path %q: %w", ancestor, statErr)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		missing = append(missing, filepath.Base(ancestor))
		ancestor = parent
	}
}

// Same reports whether two paths resolve to the same filesystem identity.
func Same(left, right string) (bool, error) {
	canonicalLeft, err := Canonical(left)
	if err != nil {
		return false, err
	}
	canonicalRight, err := Canonical(right)
	if err != nil {
		return false, err
	}
	return canonicalLeft == canonicalRight, nil
}

// Within reports whether path resolves to root or one of its descendants.
func Within(path, root string) (bool, error) {
	canonicalPath, err := Canonical(path)
	if err != nil {
		return false, err
	}
	canonicalRoot, err := Canonical(root)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil {
		return false, err
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}
