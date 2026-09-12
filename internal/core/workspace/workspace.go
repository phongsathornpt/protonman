// Package workspace provides the host-local filesystem boundary for Protonman.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/base/glob"
	"github.com/phongsathornpt/protonman/internal/base/pathutil"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// ErrOutsideWorkspace indicates that a path escapes the configured root,
// including through a symlink.
var ErrOutsideWorkspace = errors.New("path is outside workspace")

// ErrProtectedPath indicates that a configured protected path was targeted.
var ErrProtectedPath = errors.New("path is protected")

// ErrInternalPath indicates that model-facing tools targeted Protonman-owned runtime state.
var ErrInternalPath = errors.New("path is reserved Protonman internal state")

// ErrInvalidWorkspace indicates that a workspace root cannot be used safely.
var ErrInvalidWorkspace = errors.New("invalid workspace")

type protectedPath struct {
	absolute string
	pattern  string
	glob     bool
}

// Workspace is the shared path policy and root for local file tools.
type Workspace struct {
	mu            sync.RWMutex
	root          string
	rawRoot       string
	protected     []protectedPath
	readRoots     []string
	internalRoots []string
	mutationGate  chan struct{}
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
		root:          filepath.Clean(resolvedRoot),
		rawRoot:       filepath.Clean(absoluteRoot),
		protected:     compiledProtected,
		readRoots:     make([]string, 0),
		internalRoots: make([]string, 0),
		mutationGate:  make(chan struct{}, 1),
	}, nil
}

// Root returns the canonical workspace root.
func (w *Workspace) Root() string {
	return w.root
}

// AddReadRoot authorizes an additional external directory for read-only tools
// (e.g. an activated skill directory).
func (w *Workspace) AddReadRoot(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("read root must be an existing directory")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	cleanAbs := filepath.Clean(abs)
	cleanResolved := filepath.Clean(resolved)
	w.addReadRootLocked(cleanAbs)
	if cleanResolved != cleanAbs {
		w.addReadRootLocked(cleanResolved)
	}
	return nil
}

func (w *Workspace) addReadRootLocked(dir string) {
	for _, r := range w.readRoots {
		if r == dir {
			return
		}
	}
	w.readRoots = append(w.readRoots, dir)
}

// ReserveInternalPath marks Protonman-owned state as inaccessible to model-facing workspace tools.
func (w *Workspace) ReserveInternalPath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve internal workspace path: %w", err)
	}
	absolute = filepath.Clean(absolute)
	canonical, err := pathutil.Canonical(absolute)
	if err != nil {
		return fmt.Errorf("resolve internal workspace path: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, candidate := range []string{absolute, canonical} {
		found := false
		for _, existing := range w.internalRoots {
			if existing == candidate {
				found = true
				break
			}
		}
		if !found {
			w.internalRoots = append(w.internalRoots, candidate)
		}
	}
	return nil
}

// IsInternalPath reports whether path belongs to a reserved Protonman internal subtree.
func (w *Workspace) IsInternalPath(path string) (bool, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	absolute = filepath.Clean(absolute)
	canonical, err := pathutil.Canonical(absolute)
	if err != nil {
		return false, err
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, root := range w.internalRoots {
		if isWithin(root, absolute) || isWithin(root, canonical) {
			return true, nil
		}
	}
	return false, nil
}

func (w *Workspace) isWithinPrimary(path string) bool {
	return isWithin(w.root, path) || (w.rawRoot != "" && isWithin(w.rawRoot, path))
}

// Resolve converts a model-provided path to a checked absolute path.
func (w *Workspace) Resolve(ctx context.Context, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if strings.TrimSpace(input) == "" {
		return "", newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			"path is required",
			ErrOutsideWorkspace,
		)
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
		return newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			"path must be absolute",
			ErrOutsideWorkspace,
		)
	}
	return w.checkAbsolute(ctx, filepath.Clean(path))
}

// ResolveRead converts a model-provided path to a checked absolute path,
// permitting paths within the primary workspace root or any authorized read roots.
func (w *Workspace) ResolveRead(ctx context.Context, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if strings.TrimSpace(input) == "" {
		return "", newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			"path is required",
			ErrOutsideWorkspace,
		)
	}
	path := input
	if !filepath.IsAbs(path) {
		candidate := filepath.Join(w.root, path)
		if _, err := os.Stat(candidate); err != nil {
			w.mu.RLock()
			for _, rr := range w.readRoots {
				rcand := filepath.Join(rr, path)
				if _, rerr := os.Stat(rcand); rerr == nil {
					candidate = rcand
					break
				}
			}
			w.mu.RUnlock()
		}
		path = candidate
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", input, err)
	}
	path = filepath.Clean(path)
	if err := w.CheckAbsoluteRead(ctx, path); err != nil {
		return "", err
	}
	return path, nil
}

// ResolveExistingRead resolves a read target and requires the final path to exist.
// Read-only tools should prefer this over ResolveRead when a missing target is
// a model/input error rather than a valid prospective path.
func (w *Workspace) ResolveExistingRead(ctx context.Context, input string) (string, error) {
	path, err := w.ResolveRead(ctx, input)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", tool.WrapToolError(tool.ErrorCodeNotFound, fmt.Sprintf("not found: %q", input), err)
		}
		if errors.Is(err, os.ErrPermission) {
			return "", tool.WrapToolError(tool.ErrorCodePermissionDenied, fmt.Sprintf("cannot access path: %q", input), err)
		}
		return "", fmt.Errorf("stat read path %q: %w", input, err)
	}
	return path, nil
}

// NearestExistingReadAncestor returns the closest existing readable ancestor of input.
// It preserves the caller's path form so recovery tools can reuse it directly.
func (w *Workspace) NearestExistingReadAncestor(ctx context.Context, input string) (string, error) {
	candidate := filepath.Clean(strings.TrimSpace(input))
	if candidate == "" {
		candidate = "."
	}
	for {
		if _, err := w.ResolveExistingRead(ctx, candidate); err == nil {
			return candidate, nil
		} else if failure := tool.FailureFromError(err); failure == nil || failure.Code != tool.ErrorCodeNotFound {
			return "", err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return ".", nil
		}
		candidate = parent
	}
}

// CheckAbsoluteRead validates an absolute path discovered or requested for read operations.
// It allows paths inside the workspace root or inside any authorized read roots,
// while checking for protected paths and symlink boundary escapes.
func (w *Workspace) CheckAbsoluteRead(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("check workspace read path: %w", err)
	}
	if !filepath.IsAbs(path) {
		return newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			"path must be absolute",
			ErrOutsideWorkspace,
		)
	}
	clean := filepath.Clean(path)
	if w.isWithinPrimary(clean) {
		return w.checkAbsolute(ctx, clean)
	}
	w.mu.RLock()
	readRoots := append([]string(nil), w.readRoots...)
	w.mu.RUnlock()
	for _, rr := range readRoots {
		if isWithin(rr, clean) {
			if w.isProtected(clean) {
				return newBoundaryError(
					tool.ErrorCodeProtectedPath,
					fmt.Sprintf("path is protected: %q", clean),
					ErrProtectedPath,
				)
			}
			return w.checkSymlinkBoundaryWithRoot(clean, rr)
		}
	}
	return newBoundaryError(
		tool.ErrorCodeOutsideWorkspace,
		fmt.Sprintf("path is outside workspace: %q", clean),
		ErrOutsideWorkspace,
	)
}

// RelRead returns the path relative to the workspace root if it is within it,
// or relative to the matching read root if within a read root, or path unchanged.
func (w *Workspace) RelRead(path string) (string, error) {
	clean := filepath.Clean(path)
	if isWithin(w.root, clean) {
		return filepath.Rel(w.root, clean)
	}
	if w.rawRoot != "" && isWithin(w.rawRoot, clean) {
		return filepath.Rel(w.rawRoot, clean)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, rr := range w.readRoots {
		if isWithin(rr, clean) {
			return filepath.Rel(rr, clean)
		}
	}
	return filepath.Rel(w.root, clean)
}

// IsProtected reports whether an absolute workspace path is protected.
func (w *Workspace) IsProtected(path string) bool {
	if len(w.protected) == 0 {
		return false
	}
	return w.isProtected(path)
}

func (w *Workspace) checkAbsolute(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("check workspace path: %w", err)
	}
	clean := filepath.Clean(path)
	if !w.isWithinPrimary(clean) {
		return newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			fmt.Sprintf("path is outside workspace: %q", path),
			ErrOutsideWorkspace,
		)
	}
	if internal, err := w.IsInternalPath(path); err != nil {
		return fmt.Errorf("check internal workspace path: %w", err)
	} else if internal {
		return newBoundaryError(
			tool.ErrorCodeInternalPath,
			fmt.Sprintf("path is reserved Protonman internal state: %q", path),
			ErrInternalPath,
		)
	}
	if w.isProtected(path) {
		return newBoundaryError(
			tool.ErrorCodeProtectedPath,
			fmt.Sprintf("path is protected: %q", path),
			ErrProtectedPath,
		)
	}
	if err := w.checkSymlinkBoundary(path); err != nil {
		return err
	}
	return nil
}

func (w *Workspace) isWithinAnyReadRoot(path string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, rr := range w.readRoots {
		if isWithin(rr, path) {
			return true
		}
	}
	return false
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
	if !w.isWithinPrimary(resolvedAncestor) {
		return newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			fmt.Sprintf("symlink target is outside workspace: %q", resolvedAncestor),
			ErrOutsideWorkspace,
		)
	}
	if internal, err := w.IsInternalPath(resolvedAncestor); err != nil {
		return fmt.Errorf("check internal symlink target: %w", err)
	} else if internal {
		return newBoundaryError(
			tool.ErrorCodeInternalPath,
			fmt.Sprintf("symlink target is reserved Protonman internal state: %q", resolvedAncestor),
			ErrInternalPath,
		)
	}
	if w.isProtected(resolvedAncestor) {
		return newBoundaryError(
			tool.ErrorCodeProtectedPath,
			fmt.Sprintf("symlink target is protected: %q", resolvedAncestor),
			ErrProtectedPath,
		)
	}
	return nil
}

func (w *Workspace) checkSymlinkBoundaryWithRoot(path string, root string) error {
	ancestor, err := nearestExistingAncestor(path)
	if err != nil {
		return fmt.Errorf("resolve path ancestor: %w", err)
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return fmt.Errorf("resolve path symlinks: %w", err)
	}
	if !isWithin(root, resolvedAncestor) && !w.isWithinPrimary(resolvedAncestor) && !w.isWithinAnyReadRoot(resolvedAncestor) {
		return newBoundaryError(
			tool.ErrorCodeOutsideWorkspace,
			fmt.Sprintf("symlink target is outside workspace: %q", resolvedAncestor),
			ErrOutsideWorkspace,
		)
	}
	if internal, err := w.IsInternalPath(resolvedAncestor); err != nil {
		return fmt.Errorf("check internal symlink target: %w", err)
	} else if internal {
		return newBoundaryError(
			tool.ErrorCodeInternalPath,
			fmt.Sprintf("symlink target is reserved Protonman internal state: %q", resolvedAncestor),
			ErrInternalPath,
		)
	}
	if w.isProtected(resolvedAncestor) {
		return newBoundaryError(
			tool.ErrorCodeProtectedPath,
			fmt.Sprintf("symlink target is protected: %q", resolvedAncestor),
			ErrProtectedPath,
		)
	}
	return nil
}

type boundaryError struct {
	code    tool.ErrorCode
	message string
	cause   error
}

func newBoundaryError(code tool.ErrorCode, message string, cause error) error {
	return boundaryError{
		code:    code,
		message: message,
		cause:   cause,
	}
}

func (e boundaryError) Error() string {
	return e.message
}

func (e boundaryError) Unwrap() error {
	return e.cause
}

func (e boundaryError) FailureCode() tool.ErrorCode {
	return e.code
}

func (w *Workspace) isProtected(path string) bool {
	clean := filepath.Clean(path)
	for _, entry := range w.protected {
		if entry.glob {
			relative, err := filepath.Rel(w.root, clean)
			if err != nil && w.rawRoot != "" {
				relative, err = filepath.Rel(w.rawRoot, clean)
			}
			if err == nil {
				relative = filepath.ToSlash(relative)
				if glob.Match(entry.pattern, relative) {
					return true
				}
				// In glob syntax, a leading **/ means zero or more directory
				// components. The generic matcher already handles one or more;
				// this explicit zero-directory case protects root-level matches
				// such as server.pem for **/*.pem.
				if strings.HasPrefix(entry.pattern, "**/") && glob.Match(strings.TrimPrefix(entry.pattern, "**/"), relative) {
					return true
				}
			}
			continue
		}
		if isWithin(entry.absolute, clean) {
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

// AcquireMutation serializes workspace mutations while allowing read-only work to remain concurrent.
func (w *Workspace) AcquireMutation(ctx context.Context) (func(), error) {
	if w == nil {
		return func() {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case w.mutationGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var once sync.Once
	return func() {
		once.Do(func() { <-w.mutationGate })
	}, nil
}
