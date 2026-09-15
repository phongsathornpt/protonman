package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AddRoot adds an additional full-access workspace root. Unlike AddReadRoot,
// paths below this root are eligible for both read-only and mutating tools.
// Relative model paths still resolve from the primary Root().
func (w *Workspace) AddRoot(dir string) error {
	if w == nil {
		return fmt.Errorf("%w: workspace is required", ErrInvalidWorkspace)
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("%w: additional root is required", ErrInvalidWorkspace)
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%w: additional root %q must be absolute", ErrInvalidWorkspace, dir)
	}
	absolute := filepath.Clean(dir)
	info, err := os.Stat(absolute)
	if err != nil {
		return fmt.Errorf("%w: stat additional root %q: %v", ErrInvalidWorkspace, dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: additional root %q is not a directory", ErrInvalidWorkspace, dir)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("%w: resolve additional root %q: %v", ErrInvalidWorkspace, dir, err)
	}
	canonical = filepath.Clean(canonical)

	w.mu.Lock()
	defer w.mu.Unlock()
	for _, candidate := range []string{absolute, canonical} {
		found := false
		for _, existing := range w.roots {
			if existing == candidate {
				found = true
				break
			}
		}
		if !found {
			w.roots = append(w.roots, candidate)
		}
	}
	return nil
}

// Roots returns the canonical effective workspace roots. The primary root is
// always first; additional roots follow in registration order.
func (w *Workspace) Roots() []string {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]string, 0, 1+len(w.roots))
	out = append(out, w.root)
	for _, root := range w.roots {
		if root == w.root || root == w.rawRoot {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing == root {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, root)
		}
	}
	return out
}

func (w *Workspace) additionalRootFor(path string) (string, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, root := range w.roots {
		if isWithin(root, path) {
			return root, true
		}
	}
	return "", false
}
