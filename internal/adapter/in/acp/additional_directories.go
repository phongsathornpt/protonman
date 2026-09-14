package acp

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func normalizeSessionDirectories(cwd string, directories []string) (string, []string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd != "" {
		if !filepath.IsAbs(cwd) {
			return "", nil, fmt.Errorf("cwd %q must be absolute", cwd)
		}
		cwd = filepath.Clean(cwd)
	}
	if len(directories) > 0 && cwd == "" {
		return "", nil, fmt.Errorf("cwd is required when additionalDirectories are provided")
	}

	out := make([]string, 0, len(directories))
	seen := make(map[string]struct{}, len(directories))
	for i, raw := range directories {
		dir := strings.TrimSpace(raw)
		if dir == "" {
			return "", nil, fmt.Errorf("additionalDirectories[%d] must be non-empty", i)
		}
		if !filepath.IsAbs(dir) {
			return "", nil, fmt.Errorf("additionalDirectories[%d] %q must be absolute", i, dir)
		}
		dir = filepath.Clean(dir)
		if dir == cwd {
			continue
		}
		if _, duplicate := seen[dir]; duplicate {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return cwd, out, nil
}

func cloneDirectories(in []string) []string {
	return append([]string(nil), in...)
}

func sameDirectories(left, right []string) bool {
	return slices.Equal(left, right)
}

func sessionIsActive(sess *Session) bool {
	if sess == nil {
		return false
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sess.active
}
