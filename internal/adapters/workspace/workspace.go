// Package workspace provides the host-local filesystem boundary for Proton.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrOutsideWorkspace indicates that a path escapes the configured root,
// including through a symlink.
var ErrOutsideWorkspace = errors.New("path is outside workspace")

// ErrProtectedPath indicates that a configured protected path was targeted.
var ErrProtectedPath = errors.New("path is protected")

// ErrInvalidWorkspace indicates that a workspace root cannot be used safely.
var ErrInvalidWorkspace = errors.New("invalid workspace")

type protectedPath struct {
	absolute string
	pattern  string
	glob     bool
}

// Workspace is the shared path policy and root for local file tools.
type Workspace struct {
	root      string
	protected []protectedPath
}

// New validates a workspace root and compiles protected path entries.
func New(root string, protectedPaths []string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: root is required", ErrInvalidWorkspace)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve root: %v", ErrInvalidWorkspace, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve root symlinks: %v", ErrInvalidWorkspace, err)
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: stat root: %v", ErrInvalidWorkspace, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: root %q is not a directory", ErrInvalidWorkspace, resolvedRoot)
	}

	compiledProtected := make([]protectedPath, 0, len(protectedPaths))
	for _, rawPath := range protectedPaths {
		entry, err := compileProtectedPath(resolvedRoot, rawPath)
		if err != nil {
			return nil, err
		}
		compiledProtected = append(compiledProtected, entry)
	}
	return &Workspace{
		root:      filepath.Clean(resolvedRoot),
		protected: compiledProtected,
	}, nil
}

// Root returns the canonical workspace root.
func (w *Workspace) Root() string {
	return w.root
}

// Resolve converts a model-provided path to a checked absolute path.
func (w *Workspace) Resolve(ctx context.Context, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("%w: path is required", ErrOutsideWorkspace)
	}
	path := input
	if !filepath.IsAbs(path) {
		path = filepath.Join(w.root, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", input, err)
	}
	path = filepath.Clean(path)
	if err := w.checkAbsolute(ctx, path); err != nil {
		return "", err
	}
	return path, nil
}

// CheckAbsolute validates an absolute path discovered during a workspace
// walk, such as a grep candidate or directory entry.
func (w *Workspace) CheckAbsolute(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: path must be absolute", ErrOutsideWorkspace)
	}
	return w.checkAbsolute(ctx, filepath.Clean(path))
}

// IsProtected reports whether an absolute workspace path is protected.
func (w *Workspace) IsProtected(path string) bool {
	return w.isProtected(filepath.Clean(path))
}

func (w *Workspace) checkAbsolute(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("check workspace path: %w", err)
	}
	if !isWithin(w.root, path) {
		return fmt.Errorf("%w: %q", ErrOutsideWorkspace, path)
	}
	if w.isProtected(path) {
		return fmt.Errorf("%w: %q", ErrProtectedPath, path)
	}
	if err := w.checkSymlinkBoundary(path); err != nil {
		return err
	}
	return nil
}

func (w *Workspace) checkSymlinkBoundary(path string) error {
	ancestor, err := nearestExistingAncestor(path)
	if err != nil {
		return fmt.Errorf("resolve path ancestor: %w", err)
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return fmt.Errorf("resolve path symlinks: %w", err)
	}
	if !isWithin(w.root, resolvedAncestor) {
		return fmt.Errorf("%w: symlink target %q", ErrOutsideWorkspace, resolvedAncestor)
	}
	if w.isProtected(resolvedAncestor) {
		return fmt.Errorf("%w: symlink target %q", ErrProtectedPath, resolvedAncestor)
	}
	return nil
}

func (w *Workspace) isProtected(path string) bool {
	for _, entry := range w.protected {
		if entry.glob {
			relative, err := filepath.Rel(w.root, path)
			if err == nil && globMatch(entry.pattern, filepath.ToSlash(relative)) {
				return true
			}
			continue
		}
		if isWithin(entry.absolute, path) {
			return true
		}
	}
	return false
}

func compileProtectedPath(root string, rawPath string) (protectedPath, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return protectedPath{}, fmt.Errorf("%w: empty protected path", ErrInvalidWorkspace)
	}
	glob := strings.ContainsAny(trimmed, "*?[")
	if glob {
		pattern := trimmed
		if filepath.IsAbs(pattern) {
			pattern = filepath.ToSlash(filepath.Clean(pattern))
			if !strings.HasPrefix(pattern, filepath.ToSlash(root)+"/") && pattern != filepath.ToSlash(root) {
				return protectedPath{}, fmt.Errorf("%w: protected glob %q is outside workspace", ErrInvalidWorkspace, rawPath)
			}
			pattern = strings.TrimPrefix(pattern, filepath.ToSlash(root)+"/")
		}
		return protectedPath{pattern: filepath.ToSlash(filepath.Clean(pattern)), glob: true}, nil
	}

	absolute := trimmed
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(root, absolute)
	}
	absolute, err := filepath.Abs(absolute)
	if err != nil {
		return protectedPath{}, fmt.Errorf("%w: protected path %q: %v", ErrInvalidWorkspace, rawPath, err)
	}
	absolute = filepath.Clean(absolute)
	if !isWithin(root, absolute) {
		return protectedPath{}, fmt.Errorf("%w: protected path %q is outside workspace", ErrInvalidWorkspace, rawPath)
	}
	return protectedPath{absolute: absolute}, nil
}

func nearestExistingAncestor(path string) (string, error) {
	candidate := filepath.Clean(path)
	for {
		_, err := os.Lstat(candidate)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		candidate = parent
	}
}

func isWithin(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func globMatch(pattern string, value string) bool {
	p := []rune(pattern)
	v := []rune(value)
	previous := make([]bool, len(v)+1)
	previous[0] = true
	for _, patternRune := range p {
		current := make([]bool, len(v)+1)
		switch patternRune {
		case '*':
			current[0] = previous[0]
			for valueIndex := 1; valueIndex <= len(v); valueIndex++ {
				current[valueIndex] = current[valueIndex-1] || previous[valueIndex]
			}
		case '?':
			for valueIndex := 1; valueIndex <= len(v); valueIndex++ {
				current[valueIndex] = previous[valueIndex-1]
			}
		default:
			for valueIndex, valueRune := range v {
				if valueRune == patternRune {
					current[valueIndex+1] = previous[valueIndex]
				}
			}
		}
		previous = current
	}
	return previous[len(v)]
}
