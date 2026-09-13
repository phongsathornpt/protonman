package memoryfs

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

func (s *FileStore) scopeDir(scope memory.Scope, workspaceKey string) (string, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	switch scope {
	case memory.ScopeGlobal:
		if workspaceKey != "" {
			return "", fmt.Errorf("global memory scope cannot bind workspace_key")
		}
		return filepath.Join(s.root, "v1", "global"), nil
	case memory.ScopeWorkspace:
		if !safeSegment(workspaceKey) {
			return "", fmt.Errorf("invalid memory workspace key %q", workspaceKey)
		}
		return filepath.Join(s.root, "v1", "workspaces", workspaceKey), nil
	default:
		return "", fmt.Errorf("unsupported memory scope %q", scope)
	}
}

func safeSegment(value string) bool {
	if value == "" || value == "." || value == ".." || filepath.Base(value) != value {
		return false
	}
	for _, r := range value {
		if r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func validateEntries(scope memory.Scope, workspaceKey string, entries []memory.Entry) ([]memory.Entry, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	out := append([]memory.Entry(nil), entries...)
	for i := range out {
		if err := out[i].Validate(); err != nil {
			return nil, err
		}
		if out[i].Scope != scope {
			return nil, fmt.Errorf("memory entry %q scope %q does not match index scope %q", out[i].ID, out[i].Scope, scope)
		}
		if scope == memory.ScopeWorkspace && out[i].WorkspaceKey != workspaceKey {
			return nil, fmt.Errorf("memory entry %q workspace %q does not match index workspace %q", out[i].ID, out[i].WorkspaceKey, workspaceKey)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
