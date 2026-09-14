//go:build desktop

package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const workspacePathsPreferencesKey = "workspace.paths.v1"

func (a *application) activeWorkspacePath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, session := range a.state.Sessions {
		if session.ID == a.state.ActiveSessionID {
			return validWorkspacePath(session.Workspace)
		}
	}
	return ""
}

func (a *application) resolveWorkspacePath(workspaceKey, reportedPath string) string {
	reportedPath = validWorkspacePath(reportedPath)
	workspaceKey = strings.TrimSpace(workspaceKey)
	if reportedPath != "" {
		if workspaceKey != "" {
			_ = a.rememberWorkspacePath(workspaceKey, reportedPath)
		}
		return reportedPath
	}
	if workspaceKey == "" || a.preferences == nil {
		return ""
	}
	paths := workspacePathMap(a.preferences.String(workspacePathsPreferencesKey))
	return validWorkspacePath(paths[workspaceKey])
}

func (a *application) rememberWorkspacePath(workspaceKey, path string) error {
	workspaceKey = strings.TrimSpace(workspaceKey)
	path = validWorkspacePath(path)
	if workspaceKey == "" || path == "" {
		return nil
	}
	if a.preferences == nil {
		return fmt.Errorf("desktop preferences are unavailable")
	}
	paths := workspacePathMap(a.preferences.String(workspacePathsPreferencesKey))
	if paths[workspaceKey] == path {
		return nil
	}
	paths[workspaceKey] = path
	payload, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	a.preferences.SetString(workspacePathsPreferencesKey, string(payload))
	return nil
}

func workspacePathMap(raw string) map[string]string {
	paths := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return paths
	}
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return make(map[string]string)
	}
	for key, path := range paths {
		clean := validWorkspacePath(path)
		if clean == "" {
			delete(paths, key)
			continue
		}
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			delete(paths, key)
			continue
		}
		paths[trimmedKey] = clean
		if trimmedKey != key {
			delete(paths, key)
		}
	}
	return paths
}

func validWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return path
}
